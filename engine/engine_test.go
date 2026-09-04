package engine

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/graycode-router/catalog"
	"github.com/GrayCodeAI/graycode-router/client"
	"github.com/GrayCodeAI/graycode-router/config"
	"github.com/GrayCodeAI/graycode-router/credentials"
	"github.com/GrayCodeAI/graycode-router/llm"
)

func TestNewUsesInjectedCredentialStore(t *testing.T) {
	store := &credentials.MapStore{}
	eng, err := New(Options{SecretStore: store})
	if err != nil {
		t.Fatal(err)
	}
	if eng.secretStore != store {
		t.Fatal("engine did not retain injected credential store")
	}
}

func TestContractVersionAndRemoteCatalogIsolation(t *testing.T) {
	if ContractVersion != "2" {
		t.Fatalf("ContractVersion = %q, want 2", ContractVersion)
	}
	t.Setenv("GRAYCODE_ROUTER_MODEL_CATALOG_URL", "https://ambient.invalid/catalog.json")
	eng, err := New(Options{SecretStore: &credentials.MapStore{}, StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if eng.remoteCatalogURL != catalog.SeedCatalogURL {
		t.Fatalf("ambient remote catalog leaked into Engine: %q", eng.remoteCatalogURL)
	}
	explicit, err := New(Options{
		SecretStore: &credentials.MapStore{}, StateDir: t.TempDir(),
		RemoteCatalogURL: "https://host.example.test/catalog.json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if explicit.remoteCatalogURL != "https://host.example.test/catalog.json" {
		t.Fatalf("explicit remote catalog = %q", explicit.remoteCatalogURL)
	}
}

func TestIsCatalogCacheRequired(t *testing.T) {
	wrapped := &Error{Code: ErrorCatalogUnavailable, Cause: catalog.ErrCatalogCacheRequired}
	if !IsCatalogCacheRequired(wrapped) {
		t.Fatal("wrapped catalog cache requirement was not recognized")
	}
	if IsCatalogCacheRequired(errors.New("authentication failed")) {
		t.Fatal("unrelated error reported as catalog cache requirement")
	}
}

func TestNewDerivesHostNeutralPathsFromStateDir(t *testing.T) {
	dir := t.TempDir()
	eng, err := New(Options{SecretStore: &credentials.MapStore{}, StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if eng.catalogPath != filepath.Join(dir, "model_catalog.json") {
		t.Fatalf("catalog path = %q", eng.catalogPath)
	}
	if eng.providerConfigPath != filepath.Join(dir, "provider.json") {
		t.Fatalf("provider path = %q", eng.providerConfigPath)
	}
}

func TestCredentialStatusAndRemoveUseInjectedStore(t *testing.T) {
	ctx := context.Background()
	store := &credentials.MapStore{}
	if err := store.Set(ctx, credentials.AccountForEnv("OPENAI_API_KEY"), "sk-live-value-1234567890"); err != nil {
		t.Fatal(err)
	}
	eng, err := New(Options{SecretStore: store})
	if err != nil {
		t.Fatal(err)
	}
	status, err := eng.CredentialStatus(ctx, "openai")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Configured || status.EnvVar != "OPENAI_API_KEY" {
		t.Fatalf("unexpected status: %+v", status)
	}
	if err := eng.RemoveCredential(ctx, "openai"); err != nil {
		t.Fatal(err)
	}
	status, err = eng.CredentialStatus(ctx, "openai")
	if err != nil {
		t.Fatal(err)
	}
	if status.Configured {
		t.Fatalf("credential still configured: %+v", status)
	}
}

func TestValidateGenerateRequest(t *testing.T) {
	tests := []struct {
		name string
		req  GenerateRequest
	}{
		{name: "no messages", req: GenerateRequest{}},
		{name: "missing role", req: GenerateRequest{Messages: []Message{{Content: "hello"}}}},
		{name: "negative output", req: GenerateRequest{Messages: []Message{{Role: "user"}}, Limits: Limits{MaxOutputTokens: -1}}},
		{name: "negative continuations", req: GenerateRequest{Messages: []Message{{Role: "user"}}, Limits: Limits{MaxContinuations: -1}}},
		{name: "negative context", req: GenerateRequest{Messages: []Message{{Role: "user"}}, Requirements: Requirements{MinimumContext: -1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGenerateRequest(tt.req)
			if !IsCode(err, ErrorInvalidRequest) {
				t.Fatalf("error = %v, want invalid request", err)
			}
		})
	}
}

func TestModelPriceKnownContract(t *testing.T) {
	if !modelPriceKnown("openrouter/model:free", "", 0, 0, 0) {
		t.Fatal("explicitly free model should have known zero price")
	}
	if !modelPriceKnown("model", "Model", 0, 0, 128000) {
		t.Fatal("catalog model with context metadata should have known price")
	}
	if modelPriceKnown("model", "Model", 0, 0, 0) {
		t.Fatal("bare live model row should keep price unknown")
	}
}

func TestMessageConversionPreservesToolsAndMultimodalParts(t *testing.T) {
	messages := toClientMessages([]Message{{
		Role: "user", Content: "inspect", ContentParts: []ContentPart{{Type: "image_url", ImageURL: &llm.ImageURLPart{URL: "https://example.test/image.png", Detail: "high"}}},
		ToolUse:     []ToolCall{{ID: "call-1", Name: "read", Arguments: map[string]interface{}{"path": "a.go"}}},
		ToolResults: []ToolResult{{ToolUseID: "call-1", Content: "ok"}},
	}})
	if len(messages) != 1 || len(messages[0].ContentParts) != 1 || messages[0].ContentParts[0].ImageURL == nil {
		t.Fatalf("multimodal conversion lost data: %+v", messages)
	}
	if len(messages[0].ToolUse) != 1 || len(messages[0].ToolResults) != 1 {
		t.Fatalf("tool conversion lost data: %+v", messages[0])
	}
}

func TestNormalizedStreamContract(t *testing.T) {
	sourceEvents := make(chan client.GraycodeRouterStreamEvent, 3)
	sourceEvents <- client.GraycodeRouterStreamEvent{Type: "content", Content: "hello"}
	sourceEvents <- client.GraycodeRouterStreamEvent{Type: "tool_call", ToolCall: &client.ToolCall{ID: "1", Name: "read"}}
	sourceEvents <- client.GraycodeRouterStreamEvent{Type: "done", StopReason: "end_turn", Usage: &client.GraycodeRouterUsage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5}}
	close(sourceEvents)

	ctx, cancel := context.WithCancel(context.Background())
	stream := newStream(ctx, cancel, client.NewStreamResult(sourceEvents, nil), Route{Provider: "mock", Model: "mock/model"})
	defer stream.Close()

	var events []Event
	for stream.Next() {
		events = append(events, stream.Event())
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Fatalf("events = %d, want 4: %+v", len(events), events)
	}
	if events[0].Type != EventRouteSelected || events[1].Type != EventContentDelta || events[2].Type != EventToolCallDone || events[3].Type != EventDone {
		t.Fatalf("unexpected event sequence: %+v", events)
	}
	if events[3].Usage == nil || events[3].Usage.TotalTokens != 5 {
		t.Fatalf("usage not normalized: %+v", events[3])
	}
}

// Regression (audit E4): client/core emits end-of-stream health diagnostics
// (e.g. reasoning-only responses) as error-type events marked non-fatal via
// the Warning field, followed by the terminal done. The engine must forward
// them as warning events and still deliver the done/usage event without
// setting Err().
func TestStreamDiagnosticErrorEventIsNonFatal(t *testing.T) {
	sourceEvents := make(chan client.GraycodeRouterStreamEvent, 3)
	sourceEvents <- client.GraycodeRouterStreamEvent{Type: "content", Content: "answer"}
	sourceEvents <- client.GraycodeRouterStreamEvent{Type: "error", Error: "model produced reasoning tokens but no answer", Warning: "model produced reasoning tokens but no answer"}
	sourceEvents <- client.GraycodeRouterStreamEvent{Type: "done", StopReason: "stop", Usage: &client.GraycodeRouterUsage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3}}
	close(sourceEvents)

	ctx, cancel := context.WithCancel(context.Background())
	stream := newStream(ctx, cancel, client.NewStreamResult(sourceEvents, nil), Route{Provider: "mock", Model: "mock/model"})
	defer stream.Close()

	var events []Event
	for stream.Next() {
		events = append(events, stream.Event())
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("diagnostic must not set Err(): %v", err)
	}
	if len(events) != 4 {
		t.Fatalf("events = %d, want 4: %+v", len(events), events)
	}
	if events[1].Type != EventContentDelta || events[1].Content != "answer" {
		t.Fatalf("content delta lost: %+v", events[1])
	}
	if events[2].Type != EventWarning || events[2].Warning == "" {
		t.Fatalf("diagnostic should surface as a warning event: %+v", events[2])
	}
	if events[3].Type != EventDone || events[3].Usage == nil || events[3].Usage.TotalTokens != 3 {
		t.Fatalf("terminal done/usage lost: %+v", events[3])
	}
}

// Genuinely fatal error events (no Warning marker) keep the previous
// behavior: the stream terminates and Err() carries the classified error.
func TestStreamFatalErrorEventStillTerminal(t *testing.T) {
	sourceEvents := make(chan client.GraycodeRouterStreamEvent, 2)
	sourceEvents <- client.GraycodeRouterStreamEvent{Type: "content", Content: "partial"}
	sourceEvents <- client.GraycodeRouterStreamEvent{Type: "error", Error: "connection reset"}
	close(sourceEvents)

	ctx, cancel := context.WithCancel(context.Background())
	stream := newStream(ctx, cancel, client.NewStreamResult(sourceEvents, nil), Route{Provider: "mock", Model: "mock/model"})
	defer stream.Close()

	var events []Event
	for stream.Next() {
		events = append(events, stream.Event())
	}
	if err := stream.Err(); err == nil {
		t.Fatal("fatal error event must set Err()")
	} else if !IsCode(err, ErrorProviderUnavailable) {
		t.Fatalf("error code = %v, want provider_unavailable", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 (route_selected + content_delta): %+v", len(events), events)
	}
}

func TestSnapshotPublishesCapabilities(t *testing.T) {
	compiled := &catalog.CompiledCatalog{
		ModelsByID: map[string]catalog.Model{
			"vendor/model": {ID: "vendor/model", ProviderID: "vendor", Name: "Model", ContextWindow: 100_000},
		},
		OfferingsByCanonicalModel: map[string][]catalog.ModelOffering{
			"vendor/model": {{CanonicalModelID: "vendor/model", Capabilities: catalog.CapabilitySet{
				FunctionCalling: catalog.CapabilitySupported,
				ImageInput:      catalog.CapabilitySupported,
			}}},
		},
	}
	snapshot := snapshotFromCompiled(compiled)
	if len(snapshot.Models) != 1 {
		t.Fatalf("models = %d, want 1", len(snapshot.Models))
	}
	if got := snapshot.Models[0].Capabilities; len(got) != 2 || got[0] != "tools" || got[1] != "vision" {
		t.Fatalf("capabilities = %v", got)
	}
}

func TestSelectCompatibleModelUsesCapabilitiesAndIntent(t *testing.T) {
	compiled := &catalog.CompiledCatalog{
		ModelsByID: map[string]catalog.Model{
			"vendor/cheap": {ID: "vendor/cheap", ProviderID: "vendor", Name: "Cheap", ContextWindow: 128_000},
			"vendor/rich":  {ID: "vendor/rich", ProviderID: "vendor", Name: "Rich", ContextWindow: 200_000},
			"vendor/text":  {ID: "vendor/text", ProviderID: "vendor", Name: "Text", ContextWindow: 300_000},
		},
		OfferingsByCanonicalModel: map[string][]catalog.ModelOffering{
			"vendor/cheap": {{CanonicalModelID: "vendor/cheap", Capabilities: catalog.CapabilitySet{FunctionCalling: catalog.CapabilitySupported}, Pricing: catalog.Pricing{RatesPer1M: map[string]float64{"input_tokens": 1, "output_tokens": 2}}}},
			"vendor/rich":  {{CanonicalModelID: "vendor/rich", Capabilities: catalog.CapabilitySet{FunctionCalling: catalog.CapabilitySupported}, Pricing: catalog.Pricing{RatesPer1M: map[string]float64{"input_tokens": 5, "output_tokens": 10}}}},
			"vendor/text":  {{CanonicalModelID: "vendor/text", Capabilities: catalog.CapabilitySet{}, Pricing: catalog.Pricing{RatesPer1M: map[string]float64{"input_tokens": 0.1, "output_tokens": 0.1}}}},
		},
	}

	model, provider := selectCompatibleModel(compiled, SelectionRequest{
		Requirements: Requirements{Tools: true, MinimumContext: 100_000},
		Preference:   Preference{Intent: llm.IntentEconomical},
	})
	if model != "vendor/cheap" || provider != "vendor" {
		t.Fatalf("selected %q via %q, want vendor/cheap via vendor", model, provider)
	}

	model, _ = selectCompatibleModel(compiled, SelectionRequest{
		Requirements: Requirements{Tools: true, MinimumContext: 150_000},
		Preference:   Preference{Intent: llm.IntentReasoning},
	})
	if model != "vendor/rich" {
		t.Fatalf("selected %q, want vendor/rich", model)
	}
}

func TestTypedErrorUnwrapAndCode(t *testing.T) {
	cause := errors.New("provider down")
	err := classify("stream", Route{Provider: "mock", Model: "m"}, cause)
	if !errors.Is(err, cause) {
		t.Fatal("typed error does not unwrap cause")
	}
	var typed *Error
	if !errors.As(err, &typed) || typed.Operation != "stream" {
		t.Fatalf("unexpected typed error: %#v", err)
	}
}

func TestSelectionUsesEngineStateDir(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "model_catalog.json")
	bootstrap := catalog.SeedCatalog()
	if err := catalog.WriteCatalogCache(cachePath, &bootstrap); err != nil {
		t.Fatal(err)
	}
	eng, err := New(Options{SecretStore: &credentials.MapStore{}, StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var modelID string
	for id := range bootstrap.Models {
		modelID = id
		break
	}
	if modelID == "" {
		t.Fatal("bootstrap catalog has no model")
	}
	if err := eng.SetSelection(context.Background(), "", modelID); err != nil {
		t.Fatal(err)
	}
	active := eng.ActiveSelection(context.Background())
	if active.Model != modelID || active.Provider == "" {
		t.Fatalf("active selection = %+v", active)
	}
	saved := config.LoadProviderConfig(eng.providerConfigPath)
	if saved == nil || saved.ActiveModel != modelID {
		t.Fatalf("selection not saved to engine path: %+v", saved)
	}
	if err := eng.ClearSelection(context.Background()); err != nil {
		t.Fatal(err)
	}
	saved = config.LoadProviderConfig(eng.providerConfigPath)
	if saved == nil || saved.ActiveModel != "" || saved.ActiveProvider != "" {
		t.Fatalf("selection not cleared: %+v", saved)
	}
}

func TestListModelsUsesGatewayOfferings(t *testing.T) {
	dir := t.TempDir()
	seed := catalog.SeedCatalog()
	compiled, err := catalog.CompileCatalog(&seed)
	if err != nil {
		t.Fatal(err)
	}
	providerID := ""
	var expected []catalog.ModelCatalogEntry
	for _, candidate := range []string{"openrouter", "openai", "anthropic", "gemini"} {
		if entries := catalog.ModelEntriesForProvider(compiled, candidate); len(entries) > 0 {
			providerID, expected = candidate, entries
			break
		}
	}
	if providerID == "" {
		t.Skip("seed catalog has no gateway offerings")
	}
	if err := catalog.WriteCatalogCache(filepath.Join(dir, "model_catalog.json"), &seed); err != nil {
		t.Fatal(err)
	}
	eng, err := New(Options{StateDir: dir, SecretStore: &credentials.MapStore{}})
	if err != nil {
		t.Fatal(err)
	}
	models, err := eng.ListModels(context.Background(), providerID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != len(expected) || len(models) == 0 {
		t.Fatalf("models = %d, want %d", len(models), len(expected))
	}
	if models[0].ProviderID != providerID || models[0].ID != expected[0].ID {
		t.Fatalf("gateway model mismatch: got %+v want %+v", models[0], expected[0])
	}
}

func TestModelPolicyQueriesUseEngineCatalog(t *testing.T) {
	dir := t.TempDir()
	seed := catalog.SeedCatalog()
	if err := catalog.WriteCatalogCache(filepath.Join(dir, "model_catalog.json"), &seed); err != nil {
		t.Fatal(err)
	}
	eng, err := New(Options{SecretStore: &credentials.MapStore{}, StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	names := eng.ModelNames(ctx)
	providers, err := eng.ModelProviders(ctx)
	if err != nil || len(names) == 0 || len(providers) == 0 {
		t.Fatalf("policy catalog unavailable: names=%d providers=%v err=%v", len(names), providers, err)
	}
	modelID := firstCatalogModelID(seed)
	if model, ok, err := eng.ModelInfo(ctx, modelID); err != nil || !ok || model.ID == "" {
		t.Fatalf("ModelInfo(%q) = %+v, %v, %v", modelID, model, ok, err)
	}
	if got := eng.ModelClassOf(ctx, modelID); got == "" {
		t.Fatal("empty model class")
	}
}
