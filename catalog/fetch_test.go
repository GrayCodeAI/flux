package catalog

import (
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog/live"
)

func TestFetchOllamaModels_DelegatesLive(t *testing.T) {
	t.Parallel()
	entries, err := FetchOllamaModels(map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if entries != nil {
		t.Fatalf("expected nil without base url, got %d", len(entries))
	}
	_ = live.FetchOllama
}
