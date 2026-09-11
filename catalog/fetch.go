package catalog

import (
	"github.com/GrayCodeAI/eyrie/catalog/live"
)

// LiveEntriesToCatalog converts live fetch rows to catalog entries.
func LiveEntriesToCatalog(in []live.Entry) []ModelCatalogEntry {
	if len(in) == 0 {
		return nil
	}
	out := make([]ModelCatalogEntry, len(in))
	for i, e := range in {
		out[i] = ModelCatalogEntry{
			ID:               e.ID,
			DisplayName:      DisplayModelLabel(e.ID, e.DisplayName),
			Description:      e.Description,
			Owner:            DisplayModelOwner(e.OwnedBy, e.ID),
			ContextWindow:    e.ContextWindow,
			MaxOutput:        e.MaxOutput,
			InputPricePer1M:  e.InputPricePer1M,
			OutputPricePer1M: e.OutputPricePer1M,
			ServerTools:      append([]string(nil), e.Features...),
			LiveMetadata:     e.RawJSON,
		}
	}
	return out
}

// FetchOllamaModels lists models installed on a running Ollama instance.
func FetchOllamaModels(env map[string]string) ([]ModelCatalogEntry, error) {
	entries, err := live.FetchOllama(env)
	if err != nil {
		return nil, err
	}
	return LiveEntriesToCatalog(entries), nil
}
