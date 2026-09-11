package xiaomi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/GrayCodeAI/eyrie/internal/probehttp"
)

// ProbeOpenAIModels GETs {baseURL}/models using api-key auth, then Bearer on 401.
func ProbeOpenAIModels(ctx context.Context, baseURL, apiKey string) error {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return fmt.Errorf("xiaomi probe: missing base URL")
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return fmt.Errorf("xiaomi probe: missing API key")
	}
	url := baseURL + "/models"
	commonHeaders := map[string]string{
		"Accept":     "application/json",
		"User-Agent": probehttp.UserAgent(),
	}

	status, _, err := probehttp.DoGet(ctx, url, func() map[string]string {
		h := map[string]string{}
		for k, v := range commonHeaders {
			h[k] = v
		}
		setAPIKeyAuthHeader(h, apiKey)
		return h
	}())
	if err != nil {
		return fmt.Errorf("xiaomi probe: network error: %w", err)
	}
	if status == http.StatusUnauthorized {
		status, _, err = probehttp.DoGet(ctx, url, func() map[string]string {
			h := map[string]string{}
			for k, v := range commonHeaders {
				h[k] = v
			}
			h["Authorization"] = "Bearer " + apiKey
			return h
		}())
		if err != nil {
			return fmt.Errorf("xiaomi probe: network error: %w", err)
		}
	}
	if status >= 200 && status < 300 {
		return nil
	}
	return probehttp.ProbeError(status)
}

func setAPIKeyAuthHeader(h map[string]string, apiKey string) {
	h["api-key"] = apiKey
}

// SetMimoRequestAuth applies MiMo-preferred auth (api-key header).
func SetMimoRequestAuth(req *http.Request, apiKey string) {
	req.Header.Set("api-key", apiKey)
}

// FetchOpenAIModelsJSON GETs /models and returns raw model objects from the OpenAI list response.
func FetchOpenAIModelsJSON(ctx context.Context, baseURL, apiKey string) ([]json.RawMessage, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	apiKey = strings.TrimSpace(apiKey)
	if baseURL == "" || apiKey == "" {
		return nil, fmt.Errorf("xiaomi: base URL and API key required")
	}
	url := baseURL + "/models"

	headers := map[string]string{
		"Accept":     "application/json",
		"User-Agent": probehttp.UserAgent(),
	}
	setAPIKeyAuthHeader(headers, apiKey)

	status, body, err := probehttp.DoGet(ctx, url, headers)
	if err != nil {
		return nil, err
	}
	if status == http.StatusUnauthorized {
		delete(headers, "api-key")
		headers["Authorization"] = "Bearer " + apiKey
		status, body, err = probehttp.DoGet(ctx, url, headers)
		if err != nil {
			return nil, err
		}
	}
	if status < 200 || status >= 300 {
		return nil, probehttp.ProbeError(status)
	}
	var payload struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return payload.Data, nil
}

// IsRetryableHTTPStatus reports whether chat may retry via Anthropic compatibility.
func IsRetryableHTTPStatus(status int) bool {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden,
		http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return status >= 500
	}
}
