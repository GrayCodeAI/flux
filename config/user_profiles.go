package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// ProviderProfiles — saved, named provider configurations that users can
// switch between. Stored at ~/.graycode-router/profiles.json.
// ─────────────────────────────────────────────────────────────────────────────

// ProviderProfile is a saved provider configuration.
type ProviderProfile struct {
	Name        string    `json:"name"`
	Provider    string    `json:"provider"`
	Model       string    `json:"model"`
	BaseURL     string    `json:"base_url,omitempty"`
	APIKeyEnv   string    `json:"api_key_env,omitempty"` // env var to read key from
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// ProfilesPath returns the path to the profiles file.
func ProfilesPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".graycode-router", "profiles.json")
}

// LoadProfiles reads saved provider profiles.
func LoadProfiles() ([]ProviderProfile, error) {
	data, err := os.ReadFile(ProfilesPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read profiles: %w", err)
	}
	var profiles []ProviderProfile
	if err := json.Unmarshal(data, &profiles); err != nil {
		return nil, fmt.Errorf("parse profiles: %w", err)
	}
	return profiles, nil
}

// SaveProfiles persists provider profiles.
func SaveProfiles(profiles []ProviderProfile) error {
	dir := filepath.Dir(ProfilesPath())
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create profiles directory: %w", err)
	}
	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal profiles: %w", err)
	}
	return os.WriteFile(ProfilesPath(), data, 0o600)
}

// FindProfile returns a profile by name (case-insensitive).
func FindProfile(name string) (*ProviderProfile, error) {
	profiles, err := LoadProfiles()
	if err != nil {
		return nil, err
	}
	for _, p := range profiles {
		if strings.EqualFold(p.Name, name) {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("profile %q not found", name)
}

// AddProfile adds or updates a profile.
func AddProfile(profile ProviderProfile) error {
	if profile.Name == "" {
		return fmt.Errorf("profile name is required")
	}
	if profile.CreatedAt.IsZero() {
		profile.CreatedAt = time.Now()
	}

	profiles, err := LoadProfiles()
	if err != nil {
		return err
	}

	// Update existing or append
	for i, p := range profiles {
		if strings.EqualFold(p.Name, profile.Name) {
			profiles[i] = profile
			return SaveProfiles(profiles)
		}
	}

	profiles = append(profiles, profile)
	return SaveProfiles(profiles)
}

// DeleteProfile removes a profile by name.
func DeleteProfile(name string) error {
	profiles, err := LoadProfiles()
	if err != nil {
		return err
	}

	for i, p := range profiles {
		if strings.EqualFold(p.Name, name) {
			profiles = append(profiles[:i], profiles[i+1:]...)
			return SaveProfiles(profiles)
		}
	}
	return fmt.Errorf("profile %q not found", name)
}

// ListProfiles returns a human-readable list of profiles.
func ListProfiles() string {
	profiles, err := LoadProfiles()
	if err != nil {
		return "Failed to load profiles: " + err.Error()
	}
	if len(profiles) == 0 {
		return "No saved profiles."
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Saved profiles (%d):\n\n", len(profiles))
	for _, p := range profiles {
		fmt.Fprintf(&b, "  • %s — %s/%s", p.Name, p.Provider, p.Model)
		if p.Description != "" {
			fmt.Fprintf(&b, " (%s)", p.Description)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// RecommendedProfile returns a profile based on the user's goal.
func RecommendedProfile(goal string) ProviderProfile {
	switch strings.ToLower(goal) {
	case "coding", "code":
		return ProviderProfile{
			Name: "coding", Provider: "anthropic", Model: "claude-sonnet-4-20250514",
			Description: "Balanced coding with strong reasoning",
		}
	case "speed", "fast", "latency":
		return ProviderProfile{
			Name: "speed", Provider: "openai", Model: "gpt-4o-mini",
			Description: "Fast responses for quick tasks",
		}
	case "quality", "best", "complex":
		return ProviderProfile{
			Name: "quality", Provider: "anthropic", Model: "claude-opus-20250514",
			Description: "Maximum quality for complex tasks",
		}
	case "cheap", "budget", "frugal":
		return ProviderProfile{
			Name: "budget", Provider: "openai", Model: "gpt-4o-mini",
			Description: "Cost-optimized for high-volume use",
		}
	case "balanced", "default":
		return ProviderProfile{
			Name: "balanced", Provider: "anthropic", Model: "claude-sonnet-4-20250514",
			Description: "Best balance of speed, quality, and cost",
		}
	default:
		return ProviderProfile{
			Name: "balanced", Provider: "anthropic", Model: "claude-sonnet-4-20250514",
			Description: "Best balance of speed, quality, and cost",
		}
	}
}
