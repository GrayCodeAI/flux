package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/GrayCodeAI/graycode-router/credentials"
)

func TestRegisterCustomGatewayValidatesHostMetadata(t *testing.T) {
	for _, test := range []struct {
		name    string
		gateway CustomGateway
		want    string
	}{
		{name: "missing id", gateway: CustomGateway{BaseURL: "https://example.test/v1"}, want: "ID is required"},
		{name: "missing base URL", gateway: CustomGateway{ID: "custom"}, want: "base URL is required"},
		{name: "invalid base URL", gateway: CustomGateway{ID: "custom", BaseURL: "file:///tmp/socket"}, want: "invalid baseURL"},
		{name: "secret in base URL", gateway: CustomGateway{ID: "custom", BaseURL: "https://user:secret@example.test/v1"}, want: "must not contain userinfo"},
		{name: "query in base URL", gateway: CustomGateway{ID: "custom", BaseURL: "https://example.test/v1?key=secret"}, want: "must not contain userinfo"},
		{name: "built-in collision", gateway: CustomGateway{ID: "openai", BaseURL: "https://example.test/v1"}, want: "collides with built-in gateway"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := RegisterCustomGateway(test.gateway)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("RegisterCustomGateway() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestRegisterCustomGatewayAcceptsSafeMetadata(t *testing.T) {
	registerCustomGatewayForTest(t, CustomGateway{
		ID:            "engine-contract-test",
		BaseURL:       "https://example.test/v1",
		CredentialEnv: "ENGINE_CONTRACT_TEST_API_KEY",
	})
}

func TestParseInlineToolCallsNormalizesHermesCall(t *testing.T) {
	clean, calls := ParseInlineToolCalls(`Before <tool_call>{"name":"read_file","arguments":{"path":"main.go"}}</tool_call> after`)
	if clean != "Before  after" {
		t.Fatalf("clean = %q, want preserved visible text", clean)
	}
	if len(calls) != 1 || calls[0].Name != "read_file" || calls[0].Arguments["path"] != "main.go" {
		t.Fatalf("calls = %#v, want normalized read_file call", calls)
	}
}

func TestParseInlineToolCallsLeavesPlainTextUntouched(t *testing.T) {
	const input = "ordinary assistant response"
	clean, calls := ParseInlineToolCalls(input)
	if clean != input || len(calls) != 0 {
		t.Fatalf("ParseInlineToolCalls() = %q, %#v", clean, calls)
	}
}

func TestCustomGatewayUnknownToolModelUsesInjectedStoreForGenerateAndStream(t *testing.T) {
	const (
		gatewayID = "custom-parity-gateway"
		modelID   = "vendor/unknown-tools-model"
		envKey    = "CUSTOM_PARITY_API_KEY"
	)
	t.Setenv(envKey, "global-secret-must-not-be-used")

	var (
		mu       sync.Mutex
		requests []map[string]interface{}
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q, want /chat/completions", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer injected-secret" {
			t.Errorf("Authorization = %q, want injected SecretStore value", auth)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		requests = append(requests, body)
		mu.Unlock()
		if body["model"] != modelID {
			t.Errorf("model = %#v, want %q", body["model"], modelID)
		}
		tools, ok := body["tools"].([]interface{})
		if !ok || len(tools) != 1 {
			t.Errorf("tools = %#v, want one tool", body["tools"])
		}
		if streaming, _ := body["stream"].(bool); streaming {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "data: {\"id\":\"custom-stream\",\"choices\":[{\"delta\":{\"content\":\"streamed\"},\"finish_reason\":null}]}\n\n")
			_, _ = fmt.Fprint(w, "data: {\"id\":\"custom-stream\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
			_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "custom-generate",
			"choices": []map[string]interface{}{{
				"message":       map[string]string{"role": "assistant", "content": "generated"},
				"finish_reason": "stop",
			}},
			"usage": map[string]int{"prompt_tokens": 2, "completion_tokens": 1, "total_tokens": 3},
		})
	}))
	defer server.Close()

	registerCustomGatewayForTest(t, CustomGateway{
		ID: gatewayID, BaseURL: server.URL, CredentialEnv: envKey, DefaultModel: modelID,
	})
	store := &credentials.MapStore{}
	if err := store.Set(context.Background(), credentials.AccountForEnv(envKey), "injected-secret"); err != nil {
		t.Fatal(err)
	}
	eng, err := New(Options{StateDir: t.TempDir(), SecretStore: store, UseRegisteredCustomGateways: true})
	if err != nil {
		t.Fatal(err)
	}

	route, err := eng.Resolve(context.Background(), SelectionRequest{
		Requirements: Requirements{Tools: true},
		Preference:   Preference{PreferredProvider: gatewayID, PreferredModelID: modelID},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if route.Provider != "custom_parity_gateway" || route.Model != modelID || route.DeploymentRouting {
		t.Fatalf("route = %+v", route)
	}

	request := GenerateRequest{
		Messages:     []Message{{Role: "user", Content: "inspect main.go"}},
		Tools:        []Tool{{Name: "read_file", Parameters: map[string]interface{}{"type": "object"}}},
		Requirements: Requirements{Tools: true},
		Preference:   Preference{PreferredProvider: gatewayID, PreferredModelID: modelID},
	}
	response, err := eng.Generate(context.Background(), request)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if response.Content != "generated" || response.Route.Provider != route.Provider {
		t.Fatalf("response = %+v", response)
	}

	request.Requirements.Streaming = true
	stream, err := eng.Stream(context.Background(), request)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer stream.Close()
	var content strings.Builder
	for stream.Next() {
		if event := stream.Event(); event.Type == EventContentDelta {
			content.WriteString(event.Content)
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream error = %v", err)
	}
	if content.String() != "streamed" {
		t.Fatalf("stream content = %q", content.String())
	}
	mu.Lock()
	requestCount := len(requests)
	mu.Unlock()
	if requestCount != 2 {
		t.Fatalf("transport requests = %d, want generate + stream", requestCount)
	}

	models, err := eng.ListModels(context.Background(), gatewayID, false)
	if err != nil || len(models) != 1 || models[0].ID != modelID || models[0].Source != "custom" {
		t.Fatalf("custom models = %+v, err = %v", models, err)
	}
}

func TestCustomGatewayDeclaredCapabilitiesAreEnforced(t *testing.T) {
	const gatewayID = "custom-capability-contract"
	registerCustomGatewayForTest(t, CustomGateway{
		ID: gatewayID, BaseURL: "https://example.test/v1", DefaultModel: "custom/model",
		Capabilities: &CustomGatewayCapabilities{Streaming: true, Tools: false},
	})
	eng, err := New(Options{StateDir: t.TempDir(), SecretStore: &credentials.MapStore{}, UseRegisteredCustomGateways: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = eng.Resolve(context.Background(), SelectionRequest{
		Requirements: Requirements{Tools: true},
		Preference:   Preference{PreferredProvider: gatewayID, PreferredModelID: "custom/model"},
	})
	if !IsCode(err, ErrorCapabilityMismatch) {
		t.Fatalf("Resolve() error = %v, want capability mismatch", err)
	}
}

func TestCustomGatewayRejectsPlaceholderFromInjectedStore(t *testing.T) {
	const (
		gatewayID = "custom-placeholder-contract"
		envKey    = "CUSTOM_PLACEHOLDER_API_KEY"
	)
	registerCustomGatewayForTest(t, CustomGateway{
		ID: gatewayID, BaseURL: "https://example.test/v1", CredentialEnv: envKey, DefaultModel: "custom/model",
	})
	store := &credentials.MapStore{}
	if err := store.Set(context.Background(), credentials.AccountForEnv(envKey), "your-api-key-here"); err != nil {
		t.Fatal(err)
	}
	eng, err := New(Options{StateDir: t.TempDir(), SecretStore: store, UseRegisteredCustomGateways: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = eng.Generate(context.Background(), GenerateRequest{
		Messages:   []Message{{Role: "user", Content: "hello"}},
		Preference: Preference{PreferredProvider: gatewayID, PreferredModelID: "custom/model"},
	})
	if !IsCode(err, ErrorCredentialMissing) {
		t.Fatalf("Generate() error = %v, want credential missing", err)
	}
}

func TestSaveCustomGatewayCredentialUsesStrictTwoXXProbe(t *testing.T) {
	const envKey = "CUSTOM_PROBE_API_KEY"
	previous := customGatewayProbeClient
	t.Cleanup(func() { customGatewayProbeClient = previous })
	statusCode := http.StatusNoContent
	customGatewayProbeClient = &http.Client{Transport: engineRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://probe.example.test/v1/models" {
			t.Fatalf("probe URL = %q", req.URL.String())
		}
		if req.Header.Get("Authorization") != "Bearer custom-secret-1234567890" {
			t.Fatalf("probe authorization header was not derived from the supplied credential")
		}
		return &http.Response{
			StatusCode: statusCode,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{}`)),
			Request:    req,
		}, nil
	})}
	store := &credentials.MapStore{}
	eng, err := New(Options{
		SecretStore: store, StateDir: t.TempDir(),
		CustomGateways: []CustomGateway{{
			ID: "strict-probe", BaseURL: "https://probe.example.test/v1", CredentialEnv: envKey,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	status, err := eng.SaveCredential(context.Background(), "strict-probe", "custom-secret-1234567890")
	if err != nil || !status.Configured || !status.Verified {
		t.Fatalf("2xx custom probe = %+v, err=%v", status, err)
	}
	statusCode = http.StatusForbidden
	status, err = eng.SaveCredential(context.Background(), "strict-probe", "custom-secret-1234567890")
	if !status.Configured || status.Verified || !IsCode(err, ErrorAuthentication) {
		t.Fatalf("403 custom probe = %+v, err=%v", status, err)
	}
	statusCode = http.StatusInternalServerError
	status, err = eng.SaveCredential(context.Background(), "strict-probe", "custom-secret-1234567890")
	if !status.Configured || status.Verified || !IsCode(err, ErrorProviderUnavailable) {
		t.Fatalf("500 custom probe = %+v, err=%v", status, err)
	}
}

type engineRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn engineRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func registerCustomGatewayForTest(t *testing.T, gateway CustomGateway) {
	t.Helper()
	id := NormalizeProviderID(gateway.ID)
	customGatewayRegistry.RLock()
	previous, existed := customGatewayRegistry.gateways[id]
	customGatewayRegistry.RUnlock()
	if err := RegisterCustomGateway(gateway); err != nil {
		t.Fatalf("RegisterCustomGateway() error = %v", err)
	}
	t.Cleanup(func() {
		customGatewayRegistry.Lock()
		defer customGatewayRegistry.Unlock()
		if existed {
			customGatewayRegistry.gateways[id] = previous
			return
		}
		delete(customGatewayRegistry.gateways, id)
	})
}
