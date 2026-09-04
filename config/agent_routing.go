package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
// AgentRouting — per-agent model selection. Maps agent types to specific
// models/providers for cost optimization. Stored at ~/.graycode-router/agent-routing.json.
// ─────────────────────────────────────────────────────────────────────────────

// AgentRoutingConfig holds per-agent model routing rules.
type AgentRoutingConfig struct {
	// Routes maps agent type to model identifier (provider/model).
	// Special keys: "default" for fallback, "general-purpose" for main chat.
	Routes map[string]string `json:"routes"`
}

// AgentRoutingPath returns the path to the agent routing config.
func AgentRoutingPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".graycode-router", "agent-routing.json")
}

// LoadAgentRouting reads the agent routing config.
func LoadAgentRouting() (*AgentRoutingConfig, error) {
	data, err := os.ReadFile(AgentRoutingPath())
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultAgentRouting(), nil
		}
		return nil, fmt.Errorf("read agent routing: %w", err)
	}
	var cfg AgentRoutingConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse agent routing: %w", err)
	}
	return &cfg, nil
}

// SaveAgentRouting persists the agent routing config.
func SaveAgentRouting(cfg *AgentRoutingConfig) error {
	dir := filepath.Dir(AgentRoutingPath())
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal agent routing: %w", err)
	}
	return os.WriteFile(AgentRoutingPath(), data, 0o600)
}

// DefaultAgentRouting returns the default routing config.
func DefaultAgentRouting() *AgentRoutingConfig {
	return &AgentRoutingConfig{
		Routes: map[string]string{
			"general-purpose": "anthropic/claude-sonnet-4-20250514",
			"default":         "anthropic/claude-sonnet-4-20250514",
		},
	}
}

// ResolveModel returns the model for the given agent type.
// Falls back to "default" route, then to the provided fallback.
func (c *AgentRoutingConfig) ResolveModel(agentType, fallback string) string {
	if c.Routes == nil {
		return fallback
	}
	if model, ok := c.Routes[agentType]; ok {
		return model
	}
	if model, ok := c.Routes["default"]; ok {
		return model
	}
	return fallback
}

// SetRoute sets the model for an agent type.
func (c *AgentRoutingConfig) SetRoute(agentType, model string) {
	if c.Routes == nil {
		c.Routes = make(map[string]string)
	}
	c.Routes[agentType] = model
}

// ParseRoute splits a route string into provider and model.
// Format: "provider/model" or just "model".
func ParseRoute(route string) (provider, model string) {
	parts := splitRoute(route)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", parts[0]
}

func splitRoute(route string) []string {
	for i := 0; i < len(route); i++ {
		if route[i] == '/' {
			return []string{route[:i], route[i+1:]}
		}
	}
	return []string{route}
}

// AgentRoutingSummary returns a human-readable summary of routing config.
func AgentRoutingSummary() string {
	cfg, err := LoadAgentRouting()
	if err != nil {
		return "Failed to load agent routing: " + err.Error()
	}

	var b strings.Builder
	b.WriteString("Agent routing:\n")
	for agentType, route := range cfg.Routes {
		fmt.Fprintf(&b, "  %s → %s\n", agentType, route)
	}
	return b.String()
}
