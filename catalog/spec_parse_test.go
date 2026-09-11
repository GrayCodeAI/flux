package catalog

import (
	"encoding/json"
	"testing"
	"time"
)

// --- ParseCatalog tests ---

func TestParseCatalog_V1Format(t *testing.T) {
	c := SeedCatalog()
	_ = c
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	// Debug: show offerings count
	if len(c.Offerings) == 0 {
		t.Log("DEBUG: SeedCatalog has 0 offerings")
	} else {
		t.Logf("DEBUG: SeedCatalog has %d offerings", len(c.Offerings))
	}
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCatalog(data)
	if err != nil {
		t.Fatalf("ParseCatalog failed: %v", err)
	}
	if parsed.SchemaVersion != CatalogSchemaVersion {
		t.Fatalf("schema_version = %q", parsed.SchemaVersion)
	}
	if len(parsed.Models) == 0 {
		t.Fatal("expected models in parsed catalog")
	}
}

func TestParseCatalog_LegacyFormatRejected(t *testing.T) {
	legacy := testModelCatalog()
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseCatalog(data)
	if err == nil {
		t.Fatal("expected error for legacy ModelCatalog format")
	}
}

func TestParseCatalog_InvalidJSON(t *testing.T) {
	_, err := ParseCatalog([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseCatalog_StrictV1RejectsUnknownFields(t *testing.T) {
	data := []byte(`{
		"schema_version": "model-catalog/v1",
		"unknown_field": true,
		"generated_at": "2026-01-01T00:00:00Z",
		"stale_after": "2026-02-01T00:00:00Z",
		"providers": {"p": {"id": "p", "name": "P"}},
		"api_protocols": {"a": {"id": "a", "name": "A"}},
		"deployments": {},
		"models": {},
		"offerings": []
	}`)
	_, err := ParseCatalog(data)
	if err == nil {
		t.Fatal("expected error for unknown fields in v1 format")
	}
}

// --- ValidateCatalog tests ---

func TestValidateCatalog_NilCatalog(t *testing.T) {
	if err := ValidateCatalog(nil); err == nil {
		t.Fatal("expected error for nil catalog")
	}
}

func TestValidateCatalog_BadSchemaVersion(t *testing.T) {
	c := SeedCatalog()
	c.SchemaVersion = "wrong"
	if err := ValidateCatalog(&c); err == nil {
		t.Fatal("expected error for wrong schema_version")
	}
}

func TestValidateCatalog_MissingGeneratedAt(t *testing.T) {
	c := SeedCatalog()
	c.GeneratedAt = time.Time{}
	if err := ValidateCatalog(&c); err == nil {
		t.Fatal("expected error for zero generated_at")
	}
}

func TestValidateCatalog_StaleAfterBeforeGeneratedAt(t *testing.T) {
	c := SeedCatalog()
	c.GeneratedAt = time.Now().UTC()
	c.StaleAfter = c.GeneratedAt.Add(-time.Hour)
	if err := ValidateCatalog(&c); err == nil {
		t.Fatal("expected error when stale_after < generated_at")
	}
}

func TestValidateCatalog_BadProviderIDMismatch(t *testing.T) {
	c := SeedCatalog()
	c.Providers["bad"] = Provider{ID: "mismatch", Name: "Bad"}
	if err := ValidateCatalog(&c); err == nil {
		t.Fatal("expected error for provider ID mismatch")
	}
}

func TestValidateCatalog_BadDeploymentProviderRef(t *testing.T) {
	c := SeedCatalog()
	c.Deployments["bad-dep"] = Deployment{
		ID:                  "bad-dep",
		Name:                "Bad",
		ProviderID:          "nonexistent",
		APIProtocolID:       "openai-chat-completions",
		AdapterConstructor:  "openai",
		NativeModelIDSource: NativeModelIDCatalogKnown,
	}
	if err := ValidateCatalog(&c); err == nil {
		t.Fatal("expected error for bad provider reference in deployment")
	}
}

func TestValidateCatalog_BadDeploymentProtocolRef(t *testing.T) {
	c := SeedCatalog()
	c.Deployments["bad-dep"] = Deployment{
		ID:                  "bad-dep",
		Name:                "Bad",
		ProviderID:          "openai",
		APIProtocolID:       "nonexistent-protocol",
		AdapterConstructor:  "openai",
		NativeModelIDSource: NativeModelIDCatalogKnown,
	}
	if err := ValidateCatalog(&c); err == nil {
		t.Fatal("expected error for bad api_protocol reference in deployment")
	}
}

func TestValidateCatalog_BadModelProviderRef(t *testing.T) {
	c := SeedCatalog()
	c.Models["bad/model"] = Model{
		ID:         "bad/model",
		ProviderID: "nonexistent",
		Name:       "Bad Model",
	}
	if err := ValidateCatalog(&c); err == nil {
		t.Fatal("expected error for bad provider reference in model")
	}
}

func TestValidateCatalog_DuplicateOffering(t *testing.T) {
	c := SeedCatalog()
	c.Offerings = append(c.Offerings, c.Offerings[0])
	if err := ValidateCatalog(&c); err == nil {
		t.Fatal("expected error for duplicate offering")
	}
}

func TestValidateCatalog_OfferingBadModelRef(t *testing.T) {
	c := SeedCatalog()
	c.Offerings = append(c.Offerings, ModelOffering{
		ID:               "anthropic-direct:fake",
		CanonicalModelID: "nonexistent/model",
		DeploymentID:     "anthropic-direct",
		NativeModelID:    "fake",
		Pricing:          Pricing{Status: PricingUnknown},
	})
	if err := ValidateCatalog(&c); err == nil {
		t.Fatal("expected error for offering referencing unknown model")
	}
}

func TestValidateCatalog_InvalidPricingStatus(t *testing.T) {
	c := SeedCatalog()
	c.Offerings[0].Pricing = Pricing{Status: "invalid_status"}
	if err := ValidateCatalog(&c); err == nil {
		t.Fatal("expected error for invalid pricing status")
	}
}

func TestValidateCatalog_InvalidCapabilityState(t *testing.T) {
	c := SeedCatalog()
	c.Offerings[0].Capabilities = CapabilitySet{
		FunctionCalling: "invalid_state",
	}
	if err := ValidateCatalog(&c); err == nil {
		t.Fatal("expected error for invalid capability state")
	}
}

func TestValidateCatalog_ValidBootstrapCatalog(t *testing.T) {
	c := BootstrapCatalog()
	if err := ValidateCatalog(&c); err != nil {
		t.Fatalf("bootstrap catalog should validate: %v", err)
	}
}

// --- SplitOfferingID tests ---

func TestSplitOfferingID_Valid(t *testing.T) {
	dep, native, ok := SplitOfferingID("anthropic-direct:claude-sonnet-4-6")
	if !ok || dep != "anthropic-direct" || native != "claude-sonnet-4-6" {
		t.Fatalf("got (%q, %q, %v)", dep, native, ok)
	}
}

func TestSplitOfferingID_Invalid(t *testing.T) {
	tests := []string{"", "nocolon", ":empty-left", "empty-right:"}
	for _, id := range tests {
		_, _, ok := SplitOfferingID(id)
		if ok {
			t.Errorf("SplitOfferingID(%q) should return ok=false", id)
		}
	}
}

// --- looksCanonicalModelID tests ---

func TestLooksCanonicalModelID(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"anthropic/claude-sonnet-4-6", true},
		{"openai/gpt-4o", true},
		{"google/gemini-2.0-flash", true},
		{"no-slash", false},
		{"/empty-owner", false},
		{"empty-model/", false},
		{"has space/model", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := looksCanonicalModelID(tt.input); got != tt.want {
			t.Errorf("looksCanonicalModelID(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// --- validNativeModelIDSource tests ---

func TestValidNativeModelIDSource(t *testing.T) {
	valid := []NativeModelIDSource{
		NativeModelIDCatalogKnown, NativeModelIDDiscovered,
		NativeModelIDUserConfigured, NativeModelIDCatalogOrUser,
	}
	for _, s := range valid {
		if !validNativeModelIDSource(s) {
			t.Errorf("validNativeModelIDSource(%q) should be true", s)
		}
	}
	if validNativeModelIDSource("invalid") {
		t.Error("validNativeModelIDSource('invalid') should be false")
	}
}

// --- canonicalProviderID tests ---

func TestCanonicalProviderID(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"anthropic", "anthropic"},
		{"openai", "openai"},
		{"gemini", "google"},
		{"grok", "xai"},
		{"zai_payg", "zai_payg"},
		{"zai_coding", "zai_coding"},
		{"ollama", "ollama"},
		{"xiaomi-mimo", "xiaomi_mimo_payg"},
		{"xiaomi-mimo-payg", "xiaomi_mimo_payg"},
		{"xiaomi-mimo-token-plan", "xiaomi_mimo_token_plan"},
	}
	for _, tt := range tests {
		got := CanonicalProviderID(tt.input)
		if got != tt.want {
			t.Errorf("CanonicalProviderID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- sanitizePricing tests ---

func TestSanitizePricing_DropsNegativeRates(t *testing.T) {
	p := Pricing{
		Status:     PricingKnown,
		Currency:   "USD",
		RatesPer1M: map[string]float64{"input_tokens": 3, "output_tokens": -1},
	}
	cleaned := sanitizePricing(p)
	if _, ok := cleaned.RatesPer1M["output_tokens"]; ok {
		t.Error("negative rate should be dropped")
	}
	if cleaned.RatesPer1M["input_tokens"] != 3 {
		t.Error("positive rate should be kept")
	}
}

func TestSanitizePricing_AllNegativeBecomesUnknown(t *testing.T) {
	p := Pricing{
		Status:     PricingKnown,
		Currency:   "USD",
		RatesPer1M: map[string]float64{"input_tokens": -1, "output_tokens": -2},
	}
	cleaned := sanitizePricing(p)
	if cleaned.Status != PricingUnknown {
		t.Fatalf("status = %q, want unknown", cleaned.Status)
	}
	if cleaned.RatesPer1M != nil {
		t.Fatal("all-negative rates should result in nil RatesPer1M")
	}
}

func TestSanitizePricing_EmptyRatesMapReturnsUnchanged(t *testing.T) {
	p := Pricing{
		Status:     PricingKnown,
		Currency:   "USD",
		RatesPer1M: map[string]float64{},
	}
	cleaned := sanitizePricing(p)
	// Empty rates map returns early (len == 0) without modifying status
	if cleaned.Status != PricingKnown {
		t.Fatalf("status = %q, want known (empty map returns unchanged)", cleaned.Status)
	}
}

// --- uniqueNonEmpty tests ---

func TestUniqueNonEmpty(t *testing.T) {
	got := uniqueNonEmpty("a", "", "b", "a", "  ", "c")
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, v := range got {
		if v != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, v, want[i])
		}
	}
}

// --- CompileCatalog tests ---

func TestCompileCatalog_BuildsIndexes(t *testing.T) {
	c := SeedCatalog()
	compiled, err := CompileCatalog(&c)
	if err != nil {
		t.Fatalf("CompileCatalog: %v", err)
	}
	if len(compiled.OfferingsByID) != len(c.Offerings) {
		t.Fatalf("OfferingsByID len = %d, want %d", len(compiled.OfferingsByID), len(c.Offerings))
	}
	for _, o := range c.Offerings {
		if _, ok := compiled.OfferingsByID[o.ID]; !ok {
			t.Errorf("missing offering %q in OfferingsByID", o.ID)
		}
	}
	if len(compiled.OfferingsByCanonicalModel) == 0 {
		t.Fatal("OfferingsByCanonicalModel should not be empty")
	}
	if len(compiled.OfferingsByDeployment) == 0 {
		t.Fatal("OfferingsByDeployment should not be empty")
	}
}

func TestCompileCatalog_AppliesEnvFallbacks(t *testing.T) {
	c := SeedCatalog()
	// Remove env fallbacks to test that compile adds them
	for id, dep := range c.Deployments {
		dep.EnvFallbacks = nil
		c.Deployments[id] = dep
	}
	compiled, err := CompileCatalog(&c)
	if err != nil {
		t.Fatalf("CompileCatalog: %v", err)
	}
	anthDep := compiled.DeploymentsByID["anthropic-direct"]
	if len(anthDep.EnvFallbacks) == 0 {
		t.Error("CompileCatalog should apply env fallbacks")
	}
}

func TestCompileCatalog_InvalidCatalogFails(t *testing.T) {
	c := Catalog{} // missing required fields
	_, err := CompileCatalog(&c)
	if err == nil {
		t.Fatal("expected error for invalid catalog")
	}
}

// --- CanonicalModelForAliasOrID tests ---

func TestCanonicalModelForAliasOrID_ByDirectID(t *testing.T) {
	c := SeedCatalog()
	compiled, _ := CompileCatalog(&c)
	canonical, ok := compiled.CanonicalModelForAliasOrID("anthropic/claude-sonnet-4-6")
	if !ok || canonical != "anthropic/claude-sonnet-4-6" {
		t.Fatalf("got (%q, %v)", canonical, ok)
	}
}

func TestCanonicalModelForAliasOrID_ByAlias(t *testing.T) {
	c := SeedCatalog()
	compiled, _ := CompileCatalog(&c)
	canonical, ok := compiled.CanonicalModelForAliasOrID("claude-sonnet-4-6")
	if !ok || canonical != "anthropic/claude-sonnet-4-6" {
		t.Fatalf("got (%q, %v)", canonical, ok)
	}
}

func TestCanonicalModelForAliasOrID_NotFound(t *testing.T) {
	c := SeedCatalog()
	compiled, _ := CompileCatalog(&c)
	_, ok := compiled.CanonicalModelForAliasOrID("nonexistent-model")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestCanonicalModelForAliasOrID_NilCompiled(t *testing.T) {
	var c *CompiledCatalog
	_, ok := c.CanonicalModelForAliasOrID("anything")
	if ok {
		t.Fatal("nil compiled should return false")
	}
}

// --- ResolveModel tests ---

func TestResolveModel_ByDirectID(t *testing.T) {
	c := SeedCatalog()
	compiled, _ := CompileCatalog(&c)
	got := ResolveModel(compiled, "anthropic/claude-sonnet-4-6")
	if got != "anthropic/claude-sonnet-4-6" {
		t.Fatalf("got %q, want %q", got, "anthropic/claude-sonnet-4-6")
	}
}

func TestResolveModel_ByAlias(t *testing.T) {
	c := SeedCatalog()
	compiled, _ := CompileCatalog(&c)
	got := ResolveModel(compiled, "claude-sonnet-4-6")
	if got != "anthropic/claude-sonnet-4-6" {
		t.Fatalf("got %q, want %q", got, "anthropic/claude-sonnet-4-6")
	}
}

func TestResolveModel_NotFound(t *testing.T) {
	c := SeedCatalog()
	compiled, _ := CompileCatalog(&c)
	got := ResolveModel(compiled, "nonexistent-model")
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestResolveModel_NilCompiledNativeID(t *testing.T) {
	got := ResolveModel(nil, "openai/gpt-4o")
	if got != "openai/gpt-4o" {
		t.Fatalf("nil catalog with native ID: got %q, want %q", got, "openai/gpt-4o")
	}
}

func TestResolveModel_NilCompiledAlias(t *testing.T) {
	got := ResolveModel(nil, "gpt-4o")
	if got != "" {
		t.Fatalf("nil catalog with alias: got %q, want empty", got)
	}
}

func TestResolveModel_EmptyString(t *testing.T) {
	c := SeedCatalog()
	compiled, _ := CompileCatalog(&c)
	got := ResolveModel(compiled, "")
	if got != "" {
		t.Fatalf("empty input: got %q, want empty", got)
	}
}

func TestResolveModel_TrimsWhitespace(t *testing.T) {
	c := SeedCatalog()
	compiled, _ := CompileCatalog(&c)
	got := ResolveModel(compiled, "  claude-sonnet-4-6  ")
	if got != "anthropic/claude-sonnet-4-6" {
		t.Fatalf("whitespace trim: got %q, want %q", got, "anthropic/claude-sonnet-4-6")
	}
}

// --- OfferingForDeployment tests ---

func TestOfferingForDeployment_Found(t *testing.T) {
	c := SeedCatalog()
	compiled, _ := CompileCatalog(&c)
	offering, ok := compiled.OfferingForDeployment("anthropic/claude-sonnet-4-6", "anthropic-direct")
	if !ok {
		t.Fatal("expected offering")
	}
	if offering.NativeModelID != "claude-sonnet-4-6" {
		t.Fatalf("native = %q", offering.NativeModelID)
	}
}

func TestOfferingForDeployment_NotFound(t *testing.T) {
	c := SeedCatalog()
	compiled, _ := CompileCatalog(&c)
	_, ok := compiled.OfferingForDeployment("anthropic/claude-sonnet-4-6", "nonexistent")
	if ok {
		t.Fatal("expected not found")
	}
}

// --- ResolvedRemoteCatalogURL tests ---

func TestResolvedRemoteCatalogURL_Explicit(t *testing.T) {
	got := ResolvedRemoteCatalogURL("https://custom.example.com/catalog.json")
	if got != "https://custom.example.com/catalog.json" {
		t.Fatalf("got %q", got)
	}
}

func TestResolvedRemoteCatalogURL_Default(t *testing.T) {
	// Clear env var if set
	t.Setenv("EYRIE_MODEL_CATALOG_URL", "")
	got := ResolvedRemoteCatalogURL("")
	if got != SeedCatalogURL {
		t.Fatalf("got %q, want %q", got, SeedCatalogURL)
	}
}

func TestResolvedRemoteCatalogURL_EnvOverride(t *testing.T) {
	t.Setenv("EYRIE_MODEL_CATALOG_URL", "https://env.example.com/catalog.json")
	got := ResolvedRemoteCatalogURL("")
	if got != "https://env.example.com/catalog.json" {
		t.Fatalf("got %q", got)
	}
}
