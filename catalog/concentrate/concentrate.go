// Package concentrate holds shared constants and helpers for the Concentrate AI gateway
// (https://concentrate.ai). Models are discovered via GET /v1/models; chat
// routing picks /v1/chat/completions or /v1/messages per model based on owned_by.
package concentrate

import (
	"strings"
	"sync"
)

// DefaultBaseURL is the Concentrate AI API root.
const DefaultBaseURL = "https://api.concentrate.ai/v1"

// protocolFromOwner maps Concentrate's owned_by field to a protocol.
// Anthropic models use the Messages API; everything else uses Chat Completions.
func protocolFromOwner(ownedBy string) string {
	switch strings.ToLower(strings.TrimSpace(ownedBy)) {
	case "anthropic":
		return "anthropic"
	default:
		return "openai"
	}
}

// Dynamic protocol map — populated from live fetch results.
var (
	protocolMapMu    sync.RWMutex
	protocolMap      = map[string]string{} // model ID → "anthropic" | "openai"
	protocolMapValid = false
)

// UpdateProtocolMap refreshes the dynamic protocol map from live fetch entries.
// Called after every successful FetchConcentrate in the discover pipeline.
func UpdateProtocolMap(entries []struct{ ID, Protocol string }) {
	protocolMapMu.Lock()
	defer protocolMapMu.Unlock()
	for _, e := range entries {
		id := strings.ToLower(e.ID)
		if id == "" || e.Protocol == "" {
			continue
		}
		protocolMap[id] = e.Protocol
	}
	protocolMapValid = true
}

// UsesMessagesAPI reports whether a model should use Anthropic /v1/messages on
// Concentrate AI.
//
// Resolution order:
//  1. Dynamic protocol map (populated from live /v1/models response)
//  2. Heuristic fallback (model name pattern matching for Claude models)
func UsesMessagesAPI(modelID string) bool {
	id := strings.ToLower(modelID)
	if id == "" {
		return false
	}

	// Check dynamic map (from live fetch).
	protocolMapMu.RLock()
	if protocolMapValid {
		if proto, ok := protocolMap[id]; ok {
			protocolMapMu.RUnlock()
			return proto == "anthropic"
		}
	}
	protocolMapMu.RUnlock()

	// Fallback to heuristic: Claude models use Messages API.
	return strings.Contains(id, "claude")
}

// ProtocolMapSnapshot returns a copy of the current protocol map for testing/debugging.
func ProtocolMapSnapshot() map[string]string {
	protocolMapMu.RLock()
	defer protocolMapMu.RUnlock()
	out := make(map[string]string, len(protocolMap))
	for k, v := range protocolMap {
		out[k] = v
	}
	return out
}

// ResetProtocolMap clears the dynamic protocol map. For testing only.
func ResetProtocolMap() {
	protocolMapMu.Lock()
	defer protocolMapMu.Unlock()
	protocolMap = map[string]string{}
	protocolMapValid = false
}
