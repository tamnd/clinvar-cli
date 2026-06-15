package clinvar

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testServer(t *testing.T, mux *http.ServeMux) (*httptest.Server, *Client) {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 0
	return srv, NewClient(cfg)
}

func TestSearch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/esearch.fcgi", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("db") != "clinvar" {
			http.Error(w, "wrong db", 400)
			return
		}
		json.NewEncoder(w).Encode(wireSearch{
			ESearchResult: struct {
				Count  string   `json:"count"`
				IDList []string `json:"idlist"`
			}{
				Count:  "2847",
				IDList: []string{"10", "11", "12"},
			},
		})
	})
	_, client := testServer(t, mux)
	ids, total, err := client.Search(context.Background(), "BRCA1", 3, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2847 {
		t.Errorf("total = %d, want 2847", total)
	}
	if len(ids) != 3 {
		t.Errorf("len = %d, want 3", len(ids))
	}
	if ids[0] != "10" {
		t.Errorf("ids[0] = %q, want 10", ids[0])
	}
}

func TestFetchVariants(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/esummary.fcgi", func(w http.ResponseWriter, r *http.Request) {
		v := wireVariant{
			UID:       "10",
			Accession: "VCV000000010",
			Title:     "BRCA2 c.884C>T",
			GeneSort:  "BRCA2",
			ChrSort:   "13",
			ClinSig: struct {
				Description  string `json:"description"`
				ReviewStatus string `json:"review_status"`
				LastEval     string `json:"last_evaluated"`
			}{
				Description:  "Pathogenic",
				ReviewStatus: "criteria provided",
				LastEval:     "2023-04-01",
			},
			VarSet: []struct {
				MeasureID   string `json:"measure_id"`
				MeasureName string `json:"measure_name"`
				MeasureType string `json:"measure_type"`
				Chr         string `json:"chr"`
				Location    string `json:"location_value"`
				CdnaChange  string `json:"cdna_change"`
				ProtChange  string `json:"protein_change"`
			}{
				{MeasureType: "single nucleotide variant", CdnaChange: "c.884C>T", ProtChange: "p.Pro295Leu"},
			},
		}
		vBytes, _ := json.Marshal(v)
		result := map[string]json.RawMessage{
			"uids": json.RawMessage(`["10"]`),
			"10":   vBytes,
		}
		json.NewEncoder(w).Encode(map[string]any{"result": result})
	})
	_, client := testServer(t, mux)
	variants, err := client.FetchVariants(context.Background(), []string{"10"})
	if err != nil {
		t.Fatal(err)
	}
	if len(variants) != 1 {
		t.Fatalf("len = %d, want 1", len(variants))
	}
	v := variants[0]
	if v.ID != "10" {
		t.Errorf("ID = %q, want 10", v.ID)
	}
	if v.Gene != "BRCA2" {
		t.Errorf("Gene = %q, want BRCA2", v.Gene)
	}
	if v.Significance != "Pathogenic" {
		t.Errorf("Significance = %q, want Pathogenic", v.Significance)
	}
	if v.MeasureType != "single nucleotide variant" {
		t.Errorf("MeasureType = %q, want single nucleotide variant", v.MeasureType)
	}
}

func TestGetVariant(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/esummary.fcgi", func(w http.ResponseWriter, r *http.Request) {
		v := wireVariant{UID: "42", Accession: "VCV000000042", Title: "TP53 variant"}
		vBytes, _ := json.Marshal(v)
		result := map[string]json.RawMessage{
			"uids": json.RawMessage(`["42"]`),
			"42":   vBytes,
		}
		json.NewEncoder(w).Encode(map[string]any{"result": result})
	})
	_, client := testServer(t, mux)
	v, err := client.GetVariant(context.Background(), "42")
	if err != nil {
		t.Fatal(err)
	}
	if v.ID != "42" {
		t.Errorf("ID = %q, want 42", v.ID)
	}
}

func TestFetchVariantsEmpty(t *testing.T) {
	_, client := testServer(t, http.NewServeMux())
	variants, err := client.FetchVariants(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if variants != nil {
		t.Error("expected nil variants for empty input")
	}
}
