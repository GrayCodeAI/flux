package config

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// ModelCategory identifies a semantic task category for model routing.
// Instead of selecting specific models, agents delegate by category
// (e.g., "deep" for reasoning-heavy tasks, "quick" for fast responses).
type ModelCategory string

const (
	CategoryDeep              ModelCategory = "deep"
	CategoryQuick             ModelCategory = "quick"
	CategoryUltraBrain        ModelCategory = "ultrabrain"
	CategoryVisualEngineering ModelCategory = "visual-engineering"
	CategoryArtistry          ModelCategory = "artistry"
	CategoryWriting           ModelCategory = "writing"
	CategoryUnspecifiedLow    ModelCategory = "unspecified-low"
	CategoryUnspecifiedHigh   ModelCategory = "unspecified-high"
)

// CategoryConfig maps a category to a specific model configuration.
type CategoryConfig struct {
	Model       string  `json:"model"`
	Provider    string  `json:"provider,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Description string  `json:"description,omitempty"`
}

// CategoryRegistry holds the mapping from categories to model configs.
type CategoryRegistry struct {
	mu         sync.RWMutex
	categories map[ModelCategory]CategoryConfig
}

var (
	globalRegistry *CategoryRegistry
	registryOnce   sync.Once
)

// DefaultCategories returns the built-in category-to-model mappings.
func DefaultCategories() map[ModelCategory]CategoryConfig {
	return map[ModelCategory]CategoryConfig{
		CategoryDeep: {
			Model:       "claude-sonnet-4-6",
			Temperature: 0.3,
			MaxTokens:   16384,
			Description: "High reasoning tasks: architecture, debugging, complex logic",
		},
		CategoryQuick: {
			Model:       "claude-haiku-4-5-20251001",
			Temperature: 0.5,
			MaxTokens:   4096,
			Description: "Fast responses: simple questions, formatting, quick lookups",
		},
		CategoryUltraBrain: {
			Model:       "claude-opus-4-8",
			Temperature: 0.2,
			MaxTokens:   32768,
			Description: "Maximum quality: critical decisions, complex refactors, security audits",
		},
		CategoryVisualEngineering: {
			Model:       "claude-sonnet-4-6",
			Temperature: 0.4,
			MaxTokens:   8192,
			Description: "Code + visual tasks: UI components, CSS, layout",
		},
		CategoryArtistry: {
			Model:       "claude-opus-4-8",
			Temperature: 0.8,
			MaxTokens:   16384,
			Description: "Creative tasks: documentation, naming, design patterns",
		},
		CategoryWriting: {
			Model:       "claude-sonnet-4-6",
			Temperature: 0.7,
			MaxTokens:   8192,
			Description: "Writing tasks: docs, comments, commit messages, PR descriptions",
		},
		CategoryUnspecifiedLow: {
			Model:       "claude-haiku-4-5-20251001",
			Temperature: 0.5,
			MaxTokens:   4096,
			Description: "Default low-cost model for unspecified tasks",
		},
		CategoryUnspecifiedHigh: {
			Model:       "claude-sonnet-4-6",
			Temperature: 0.4,
			MaxTokens:   8192,
			Description: "Default high-quality model for unspecified tasks",
		},
	}
}

// GetCategoryRegistry returns the global category registry.
// It loads overrides from Hawk user config if present.
func GetCategoryRegistry() *CategoryRegistry {
	registryOnce.Do(func() {
		globalRegistry = &CategoryRegistry{
			categories: DefaultCategories(),
		}
		globalRegistry.loadOverrides()
	})
	return globalRegistry
}

// ResetCategoryRegistry clears the global registry (for testing).
func ResetCategoryRegistry() {
	globalRegistry = nil
	registryOnce = sync.Once{}
}

func (r *CategoryRegistry) loadOverrides() {
	configDir := os.Getenv("GRAYCODE_ROUTER_CONFIG_DIR")
	if configDir == "" {
		configDir = os.Getenv("HAWK_CONFIG_DIR")
	}
	if configDir == "" {
		dir, err := os.UserConfigDir()
		if err != nil || dir == "" {
			return
		}
		configDir = filepath.Join(dir, "graycode-router")
	}
	path := filepath.Join(configDir, "categories.json")
	data, err := os.ReadFile(path) // #nosec G304 -- path is built from os.UserConfigDir(), not untrusted input
	if err != nil {
		return // file not found is fine
	}
	var overrides map[ModelCategory]CategoryConfig
	if err := json.Unmarshal(data, &overrides); err != nil {
		slog.Warn("category: failed to parse overrides", "path", path, "error", err)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for cat, cfg := range overrides {
		r.categories[cat] = cfg
	}
}

// Resolve returns the CategoryConfig for the given category.
// Falls back to CategoryUnspecifiedHigh if the category is unknown.
func (r *CategoryRegistry) Resolve(cat ModelCategory) CategoryConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if cfg, ok := r.categories[cat]; ok {
		return cfg
	}
	return r.categories[CategoryUnspecifiedHigh]
}

// ResolveWithDefaults returns the config for a category, falling back to
// the provided defaults if the category-specific field is empty.
func (r *CategoryRegistry) ResolveWithDefaults(cat ModelCategory, defaultModel string) CategoryConfig {
	cfg := r.Resolve(cat)
	if cfg.Model == "" {
		cfg.Model = defaultModel
	}
	return cfg
}

// SetCategory overrides a category configuration at runtime.
func (r *CategoryRegistry) SetCategory(cat ModelCategory, cfg CategoryConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.categories[cat] = cfg
}

// AllCategories returns all registered categories.
func (r *CategoryRegistry) AllCategories() map[ModelCategory]CategoryConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[ModelCategory]CategoryConfig, len(r.categories))
	for k, v := range r.categories {
		result[k] = v
	}
	return result
}

// ResolveCategory is a convenience function that resolves a category using the global registry.
func ResolveCategory(cat ModelCategory) CategoryConfig {
	return GetCategoryRegistry().Resolve(cat)
}
