// Package clinvar is the library behind the clinvar command line:
// the HTTP client, request shaping, and the typed data models for NCBI ClinVar.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public API throws under load.
package clinvar

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Host is the eUtils hostname this client talks to, and the host the URI
// driver in domain.go claims.
const Host = "eutils.ncbi.nlm.nih.gov"

const baseURL = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils"

// Config holds the runtime settings for the ClinVar client.
type Config struct {
	BaseURL   string
	APIKey    string
	Rate      time.Duration
	Retries   int
	Timeout   time.Duration
	UserAgent string
}

// DefaultConfig returns a Config with sensible defaults: 400ms rate limit,
// 3 retries, and a 30s timeout.
func DefaultConfig() Config {
	return Config{
		BaseURL:   baseURL,
		Rate:      400 * time.Millisecond,
		Retries:   3,
		Timeout:   30 * time.Second,
		UserAgent: "clinvar-cli/0.1.0 (github.com/tamnd/clinvar-cli)",
	}
}

// Client talks to NCBI eUtils for ClinVar records.
type Client struct {
	cfg  Config
	http *http.Client
	last time.Time
}

// NewClient returns a Client using the given Config.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

func (c *Client) wait() {
	if c.cfg.Rate > 0 {
		if since := time.Since(c.last); since < c.cfg.Rate {
			time.Sleep(c.cfg.Rate - since)
		}
	}
	c.last = time.Now()
}

func (c *Client) get(ctx context.Context, rawURL string, out any) error {
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			d := time.Duration(attempt) * 500 * time.Millisecond
			if d > 5*time.Second {
				d = 5 * time.Second
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d):
			}
		}
		c.wait()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", c.cfg.UserAgent)
		resp, err := c.http.Do(req)
		if err != nil {
			if attempt < c.cfg.Retries {
				continue
			}
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			if attempt < c.cfg.Retries {
				continue
			}
			return fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return fmt.Errorf("all retries exhausted")
}

func (c *Client) withAPIKey(rawURL string) string {
	if c.cfg.APIKey == "" {
		return rawURL
	}
	if strings.Contains(rawURL, "?") {
		return rawURL + "&api_key=" + c.cfg.APIKey
	}
	return rawURL + "?api_key=" + c.cfg.APIKey
}

// --- wire types (unexported) ---

type wireSearch struct {
	ESearchResult struct {
		Count  string   `json:"count"`
		IDList []string `json:"idlist"`
	} `json:"esearchresult"`
}

type wireSummary struct {
	Result map[string]json.RawMessage `json:"result"`
}

type wireVariant struct {
	UID       string `json:"uid"`
	ObjType   string `json:"obj_type"`
	Accession string `json:"accession"`
	Title     string `json:"title"`
	NumSubmit int    `json:"num_submitters"`
	GeneSort  string `json:"gene_sort"`
	ChrSort   string `json:"chr_sort"`
	ClinSig   struct {
		Description  string `json:"description"`
		ReviewStatus string `json:"review_status"`
		LastEval     string `json:"last_evaluated"`
	} `json:"clinical_significance"`
	VarSet []struct {
		MeasureID   string `json:"measure_id"`
		MeasureName string `json:"measure_name"`
		MeasureType string `json:"measure_type"`
		Chr         string `json:"chr"`
		Location    string `json:"location_value"`
		CdnaChange  string `json:"cdna_change"`
		ProtChange  string `json:"protein_change"`
	} `json:"variation_set"`
	Conditions []struct {
		Name string `json:"name"`
	} `json:"conditions"`
}

// --- public types ---

// Variant is a single ClinVar variant record.
type Variant struct {
	ID            string   `json:"id"                       kit:"id"`
	Accession     string   `json:"accession"`
	Title         string   `json:"title"`
	Gene          string   `json:"gene,omitempty"`
	Chromosome    string   `json:"chromosome,omitempty"`
	MeasureType   string   `json:"measure_type,omitempty"`
	CdnaChange    string   `json:"cdna_change,omitempty"`
	ProteinChange string   `json:"protein_change,omitempty"`
	Significance  string   `json:"clinical_significance,omitempty"`
	ReviewStatus  string   `json:"review_status,omitempty"`
	LastEvaluated string   `json:"last_evaluated,omitempty"`
	Conditions    []string `json:"conditions,omitempty"`
	NumSubmitters int      `json:"num_submitters,omitempty"`
}

func toVariant(w wireVariant) *Variant {
	conds := make([]string, 0, len(w.Conditions))
	for _, c := range w.Conditions {
		if c.Name != "" {
			conds = append(conds, c.Name)
		}
	}
	var mt, cdna, prot string
	if len(w.VarSet) > 0 {
		mt = w.VarSet[0].MeasureType
		cdna = w.VarSet[0].CdnaChange
		prot = w.VarSet[0].ProtChange
	}
	return &Variant{
		ID:            w.UID,
		Accession:     w.Accession,
		Title:         w.Title,
		Gene:          w.GeneSort,
		Chromosome:    w.ChrSort,
		MeasureType:   mt,
		CdnaChange:    cdna,
		ProteinChange: prot,
		Significance:  w.ClinSig.Description,
		ReviewStatus:  w.ClinSig.ReviewStatus,
		LastEvaluated: w.ClinSig.LastEval,
		Conditions:    conds,
		NumSubmitters: w.NumSubmit,
	}
}

// Search searches ClinVar for variants matching the query and returns IDs and
// the total hit count.
func (c *Client) Search(ctx context.Context, query string, limit, start int) ([]string, int, error) {
	u := fmt.Sprintf("%s/esearch.fcgi?db=clinvar&term=%s&retmax=%d&retstart=%d&retmode=json",
		c.cfg.BaseURL, url.QueryEscape(query), limit, start)
	u = c.withAPIKey(u)
	var w wireSearch
	if err := c.get(ctx, u, &w); err != nil {
		return nil, 0, err
	}
	count := 0
	fmt.Sscanf(w.ESearchResult.Count, "%d", &count)
	return w.ESearchResult.IDList, count, nil
}

// FetchVariants fetches variant details for the given IDs (up to ~500 per
// call; callers should batch if needed).
func (c *Client) FetchVariants(ctx context.Context, ids []string) ([]*Variant, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	u := fmt.Sprintf("%s/esummary.fcgi?db=clinvar&id=%s&retmode=json",
		c.cfg.BaseURL, strings.Join(ids, ","))
	u = c.withAPIKey(u)
	var w wireSummary
	if err := c.get(ctx, u, &w); err != nil {
		return nil, err
	}
	rawUIDs, ok := w.Result["uids"]
	if !ok {
		return nil, fmt.Errorf("no uids in esummary response")
	}
	var uids []string
	if err := json.Unmarshal(rawUIDs, &uids); err != nil {
		return nil, err
	}
	var variants []*Variant
	for _, uid := range uids {
		raw, ok := w.Result[uid]
		if !ok {
			continue
		}
		var wv wireVariant
		if err := json.Unmarshal(raw, &wv); err != nil {
			continue
		}
		variants = append(variants, toVariant(wv))
	}
	return variants, nil
}

// GetVariant fetches a single variant by its ClinVar numeric ID.
func (c *Client) GetVariant(ctx context.Context, id string) (*Variant, error) {
	variants, err := c.FetchVariants(ctx, []string{id})
	if err != nil {
		return nil, err
	}
	if len(variants) == 0 {
		return nil, fmt.Errorf("variant %s not found", id)
	}
	return variants[0], nil
}

// SearchAndFetch searches ClinVar and returns full Variant records.
func (c *Client) SearchAndFetch(ctx context.Context, query string, limit, start int) ([]*Variant, int, error) {
	ids, total, err := c.Search(ctx, query, limit, start)
	if err != nil {
		return nil, 0, err
	}
	if len(ids) == 0 {
		return nil, total, nil
	}
	variants, err := c.FetchVariants(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	return variants, total, nil
}
