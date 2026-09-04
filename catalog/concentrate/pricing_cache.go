package concentrate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// pricingCache provides file-based caching for model pricing data to avoid
// repeated API calls to /v1/models/{model} on every run.
type pricingCache struct {
	mu      sync.Mutex
	dir     string
	ttl     time.Duration
	entries map[string]PricingCacheEntry
}

// PricingCacheEntry holds all pricing data for a model, including reasoning and tiered pricing.
type PricingCacheEntry struct {
	// InputPrice is the per-token input price in USD.
	InputPrice float64 `json:"input_price"`
	// OutputPrice is the per-token output price in USD.
	OutputPrice float64 `json:"output_price"`
	// ReasoningPrice is the per-token reasoning price in USD (0 if not separate).
	ReasoningPrice float64 `json:"reasoning_price,omitempty"`
	// TierThreshold is the token count at which tiered pricing kicks in (0 if no tiers).
	TierThreshold int `json:"tier_threshold,omitempty"`
	// TieredInputPrice is the per-token input price above the threshold.
	TieredInputPrice float64 `json:"tiered_input_price,omitempty"`
	// TieredOutputPrice is the per-token output price above the threshold.
	TieredOutputPrice float64 `json:"tiered_output_price,omitempty"`
	// CachedAt is when this entry was cached.
	CachedAt time.Time `json:"cached_at"`
}

var (
	defaultCache     *pricingCache
	defaultCacheOnce sync.Once
)

// cacheDir returns the directory for storing the pricing cache.
// Uses $XDG_CACHE_HOME/graycode-router/ or ~/.cache/graycode-router/ as fallback.
func cacheDir() string {
	if d := os.Getenv("XDG_CACHE_HOME"); d != "" {
		return filepath.Join(d, "graycode-router")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "graycode-router-cache")
	}
	return filepath.Join(home, ".cache", "graycode-router")
}

// getCache returns the shared pricing cache instance.
func getCache() *pricingCache {
	defaultCacheOnce.Do(func() {
		dir := cacheDir()
		_ = os.MkdirAll(dir, 0o755)
		defaultCache = &pricingCache{
			dir:     dir,
			ttl:     24 * time.Hour,
			entries: make(map[string]PricingCacheEntry),
		}
	})
	return defaultCache
}

// GetPricing looks up cached pricing for a model.
// Returns ok=false if not found or expired.
func GetPricing(modelID string) (entry PricingCacheEntry, ok bool) {
	return getCache().get(modelID)
}

// SetPricing stores pricing for a model in the cache.
func SetPricing(modelID string, entry PricingCacheEntry) {
	getCache().set(modelID, entry)
}

// ResetCache clears the pricing cache. For testing only.
func ResetCache() {
	getCache().reset()
}

func (c *pricingCache) get(modelID string) (PricingCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	cacheFile := filepath.Join(c.dir, "concentrate-pricing.json")
	if err := c.load(cacheFile); err != nil {
		// Cache miss on read error — just return not-ok
		return PricingCacheEntry{}, false
	}

	entry, ok := c.entries[modelID]
	if !ok {
		return PricingCacheEntry{}, false
	}
	if time.Since(entry.CachedAt) > c.ttl {
		return PricingCacheEntry{}, false
	}
	return entry, true
}

func (c *pricingCache) set(modelID string, entry PricingCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry.CachedAt = time.Now()
	c.entries[modelID] = entry

	cacheFile := filepath.Join(c.dir, "concentrate-pricing.json")
	_ = c.save(cacheFile)
}

func (c *pricingCache) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]PricingCacheEntry)
	cacheFile := filepath.Join(c.dir, "concentrate-pricing.json")
	os.Remove(cacheFile)
}

func (c *pricingCache) load(cacheFile string) error {
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &c.entries)
}

func (c *pricingCache) save(cacheFile string) error {
	data, err := json.MarshalIndent(c.entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cacheFile, data, 0o644)
}
