package clinvar

import (
	"testing"
)

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "clinvar" {
		t.Errorf("Scheme = %q, want clinvar", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "clinvar" {
		t.Errorf("Identity.Binary = %q, want clinvar", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	typ, id, err := Domain{}.Classify("12345")
	if err != nil {
		t.Fatalf("Classify error: %v", err)
	}
	if typ != "variant" {
		t.Errorf("type = %q, want variant", typ)
	}
	if id != "12345" {
		t.Errorf("id = %q, want 12345", id)
	}
}

func TestClassifyInvalid(t *testing.T) {
	_, _, err := Domain{}.Classify("BRCA1")
	if err == nil {
		t.Fatal("expected error for non-numeric ID")
	}
}

func TestLocate(t *testing.T) {
	got, err := Domain{}.Locate("variant", "10")
	if err != nil {
		t.Fatalf("Locate error: %v", err)
	}
	if got != "https://www.ncbi.nlm.nih.gov/clinvar/variation/10/" {
		t.Errorf("Locate = %q", got)
	}
}
