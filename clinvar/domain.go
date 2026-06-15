package clinvar

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

func init() { kit.Register(Domain{}) }

// Domain is the ClinVar driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against,
// and the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "clinvar",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "clinvar",
			Short:  "A command line for NCBI ClinVar genetic variants.",
			Long: `A command line for NCBI ClinVar.

clinvar reads genetic variant records from the NCBI ClinVar database,
which archives reports of relationships between human variants and phenotypes.
No API key required. Optional CLINVAR_API_KEY env var for higher rate limits.
746,000+ variants indexed.`,
			Site: "https://www.ncbi.nlm.nih.gov/clinvar/",
			Repo: "https://github.com/tamnd/clinvar-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{Name: "search", Group: "read", List: true,
		Summary: "Search ClinVar variants by gene, condition, or keyword (--limit, --start)",
		Args:    []kit.Arg{{Name: "query", Help: "search query (e.g. BRCA1, cancer, pathogenic)"}}}, searchVariants)

	kit.Handle(app, kit.OpMeta{Name: "variant", Group: "read", Single: true,
		Summary: "Get a single variant by ClinVar numeric ID", URIType: "variant", Resolver: true,
		Args: []kit.Arg{{Name: "id", Help: "ClinVar numeric variant ID"}}}, getVariant)

	kit.Handle(app, kit.OpMeta{Name: "gene", Group: "read", List: true,
		Summary: "List variants for a gene symbol (--limit, --start)",
		Args:    []kit.Arg{{Name: "gene", Help: "gene symbol (e.g. BRCA1, TP53)"}}}, geneVariants)
}

// newClient builds the ClinVar client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := DefaultConfig()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.Timeout = cfg.Timeout
	}
	// Read optional API key from the prefixed environment variable.
	if key, ok := cfg.Env("API_KEY"); ok && key != "" {
		c.APIKey = key
	}
	return NewClient(c), nil
}

// --- inputs ---

type searchInput struct {
	Query  string  `kit:"arg"          help:"search query"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Start  int     `kit:"flag"         help:"offset for pagination"`
	Client *Client `kit:"inject"`
}

type variantInput struct {
	ID     string  `kit:"arg"    help:"ClinVar numeric ID"`
	Client *Client `kit:"inject"`
}

type geneInput struct {
	Gene   string  `kit:"arg"          help:"gene symbol"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Start  int     `kit:"flag"         help:"offset for pagination"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func searchVariants(ctx context.Context, in searchInput, emit func(*Variant) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	variants, _, err := in.Client.SearchAndFetch(ctx, in.Query, limit, in.Start)
	if err != nil {
		return err
	}
	for _, v := range variants {
		if err := emit(v); err != nil {
			return err
		}
	}
	return nil
}

func getVariant(ctx context.Context, in variantInput, emit func(*Variant) error) error {
	v, err := in.Client.GetVariant(ctx, in.ID)
	if err != nil {
		return err
	}
	return emit(v)
}

func geneVariants(ctx context.Context, in geneInput, emit func(*Variant) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	// ClinVar gene field qualifier: "BRCA1[gene]"
	query := fmt.Sprintf("%s[gene]", in.Gene)
	variants, _, err := in.Client.SearchAndFetch(ctx, query, limit, in.Start)
	if err != nil {
		return err
	}
	for _, v := range variants {
		if err := emit(v); err != nil {
			return err
		}
	}
	return nil
}

// Classify turns any accepted input into the canonical (type, id).
// ClinVar numeric IDs are all digits.
func (Domain) Classify(input string) (string, string, error) {
	s := strings.TrimSpace(input)
	if len(s) > 0 && allDigits(s) {
		return "variant", s, nil
	}
	return "", "", errs.Usage("clinvar IDs are numeric, got %q", input)
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(t, id string) (string, error) {
	switch t {
	case "variant":
		return fmt.Sprintf("https://www.ncbi.nlm.nih.gov/clinvar/variation/%s/", id), nil
	default:
		return "", errs.Usage("clinvar has no resource type %q", t)
	}
}

// allDigits reports whether s is a non-empty string of ASCII digits.
func allDigits(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
