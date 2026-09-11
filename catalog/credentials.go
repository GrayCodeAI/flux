package catalog

// Credentials carries API keys and related env (base URLs) for provider-backed catalog discovery.
// Keys use standard env var names (e.g. OPENROUTER_API_KEY). Populate via config.DiscoveryCredentials.
// or pass an explicit map from hawk — do not hardcode provider lists in hawk.
type Credentials struct {
	APIKeys map[string]string
}

// Env returns a copy of the key map suitable for catalog discovery.
func (c Credentials) Env() map[string]string {
	out := make(map[string]string, len(c.APIKeys))
	for k, v := range c.APIKeys {
		if k != "" && v != "" {
			out[k] = v
		}
	}
	return out
}

// MergeCredentials merges additional keys into c (later keys win).
func (c *Credentials) Merge(other Credentials) {
	if c.APIKeys == nil {
		c.APIKeys = map[string]string{}
	}
	for k, v := range other.APIKeys {
		if k != "" && v != "" {
			c.APIKeys[k] = v
		}
	}
}
