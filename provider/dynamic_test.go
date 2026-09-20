package provider

import (
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/GrayCodeAI/flux/provider/adapters"
	"github.com/GrayCodeAI/flux/provider/core"
)

func TestCustomProviderIsClientOwned(t *testing.T) {
	first := Client(nil)
	second := Client(nil)
	if err := first.RegisterCustomProvider("private", "https://first.example.test/v1", ""); err != nil {
		t.Fatal(err)
	}
	if second.GetProviderInfo("private") != nil {
		t.Fatal("custom provider leaked to another client")
	}
	if !slices.Contains(first.GetProviders(), "private") || slices.Contains(second.GetProviders(), "private") {
		t.Fatal("provider listing is not instance-isolated")
	}
	info := first.GetProviderInfo("private")
	if info == nil || info.Type != adapters.ProviderTypeOpenAICompatible || info.BaseURL != "https://first.example.test/v1" {
		t.Fatalf("custom provider info = %+v", info)
	}
	info.Compat.MaxTokensField = "corrupted"
	if first.GetProviderInfo("private").Compat.MaxTokensField != "max_tokens" {
		t.Fatal("provider info exposed mutable registry state")
	}
	firstProvider, err := first.getOrCreateProvider("private")
	if err != nil {
		t.Fatal(err)
	}
	if got := firstProvider.(*adapters.OpenAIClient).BaseURL(); got != "https://first.example.test/v1" {
		t.Fatalf("first URL = %q", got)
	}
	if err := first.RegisterCustomProvider("private", "https://second.example.test/v1", ""); err != nil {
		t.Fatal(err)
	}
	replaced, err := first.getOrCreateProvider("private")
	if err != nil {
		t.Fatal(err)
	}
	if replaced == firstProvider || replaced.(*adapters.OpenAIClient).BaseURL() != "https://second.example.test/v1" {
		t.Fatal("re-registration did not replace cached transport")
	}
	if second.GetProviderInfo("private") != nil {
		t.Fatal("re-registration leaked to another client")
	}
}

func TestExplicitCustomProviderNeedsNoGlobalRegistration(t *testing.T) {
	c := Client(&core.FluxConfig{Provider: "private", BaseURL: "https://private.example.test/v1", APIKey: "local-key"})
	info := c.GetProviderInfo("private")
	if info == nil || info.BaseURL != "https://private.example.test/v1" {
		t.Fatalf("explicit provider info = %+v", info)
	}
	p, err := c.getOrCreateProvider("private")
	if err != nil {
		t.Fatal(err)
	}
	if got := p.(*adapters.OpenAIClient).BaseURL(); got != "https://private.example.test/v1" {
		t.Fatalf("explicit URL = %q", got)
	}
	if Client(nil).GetProviderInfo("private") != nil {
		t.Fatal("explicit config changed global provider registry")
	}
}

func TestCustomProviderRejectsUnsafeConfiguration(t *testing.T) {
	c := Client(nil)
	for _, tc := range []struct{ name, url string }{
		{"", "https://example.test/v1"},
		{"openai", "https://example.test/v1"},
		{"private", ""},
		{"private", "file:///tmp/socket"},
		{"private", "https://user:secret@example.test/v1"},
		{"private", "https://example.test/v1?key=secret"},
	} {
		if err := c.RegisterCustomProvider(tc.name, tc.url, ""); err == nil {
			t.Errorf("unsafe custom provider %q %q accepted", tc.name, tc.url)
		}
	}
	if c.GetProviderInfo("private") != nil {
		t.Fatal("invalid custom provider was registered")
	}
	invalid := Client(&core.FluxConfig{Provider: "private", BaseURL: "https://user:secret@example.test/v1"})
	if _, err := invalid.getOrCreateProvider("private"); err == nil || !strings.Contains(err.Error(), "invalid base URL") {
		t.Fatalf("unsafe explicit URL error = %v", err)
	}
}

func TestCustomProviderOverridesInitialExplicitEndpoint(t *testing.T) {
	c := Client(&core.FluxConfig{Provider: "private", BaseURL: "https://old.example.test/v1", APIKey: "local-key"})
	if err := c.RegisterCustomProvider("private", "https://new.example.test/v1", ""); err != nil {
		t.Fatal(err)
	}
	p, err := c.getOrCreateProvider("private")
	if err != nil {
		t.Fatal(err)
	}
	if got := p.(*adapters.OpenAIClient).BaseURL(); got != "https://new.example.test/v1" {
		t.Fatalf("registered URL = %q, want new endpoint", got)
	}
}

func TestCustomProviderConcurrentReadAndReplace(t *testing.T) {
	c := Client(nil)
	if err := c.RegisterCustomProvider("private", "https://one.example.test/v1", ""); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if i%2 == 0 {
				if err := c.RegisterCustomProvider("private", "https://two.example.test/v1", ""); err != nil {
					t.Errorf("register: %v", err)
				}
				return
			}
			if info := c.GetProviderInfo("private"); info == nil || info.BaseURL == "" {
				t.Error("missing custom provider during update")
			}
			_ = c.GetProviders()
		}()
	}
	wg.Wait()
}
