package catalog_test

import (
	"encoding/json"
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/catalog/live"
)

func TestLiveEntriesToCatalog_PreservesFullJSONInOffering(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"id":"moonshotai/kimi-k2.6","owned_by":"moonshotai"}`)
	entries := catalog.LiveEntriesToCatalog([]live.Entry{{
		ID: "moonshotai/kimi-k2.6", DisplayName: "Kimi K2.6", RawJSON: raw,
	}})
	if len(entries) != 1 {
		t.Fatal("expected one entry")
	}
	if string(entries[0].LiveMetadata) != string(raw) {
		t.Fatalf("metadata = %s", entries[0].LiveMetadata)
	}
	c := &catalog.Catalog{
		Models: map[string]catalog.Model{
			"canopywave/moonshotai-kimi-k2.6": {ID: "canopywave/moonshotai-kimi-k2.6", ProviderID: "canopywave", Name: "Kimi K2.6"},
		},
		Offerings: []catalog.ModelOffering{
			{ID: "canopywave:moonshotai/kimi-k2.6", CanonicalModelID: "canopywave/moonshotai-kimi-k2.6", DeploymentID: "canopywave", NativeModelID: "moonshotai/kimi-k2.6", LiveMetadata: raw},
		},
	}
	var offering catalog.ModelOffering
	for _, o := range c.Offerings {
		if o.DeploymentID == "canopywave" && o.NativeModelID == "moonshotai/kimi-k2.6" {
			offering = o
			break
		}
	}
	if offering.ID == "" {
		t.Fatal("canopywave offering not found")
	}
	if string(offering.LiveMetadata) != string(raw) {
		t.Fatalf("offering metadata = %s", offering.LiveMetadata)
	}
}
