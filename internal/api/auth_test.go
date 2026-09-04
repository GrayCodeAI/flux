package api

import (
	"testing"

	"github.com/GrayCodeAI/graycode-router/internal/httputil"
)

func TestConstantTimeEqual(t *testing.T) {
	t.Parallel()
	key := "super-secret-api-key"
	if !httputil.ConstantTimeEqual(key, key) {
		t.Fatal("expected equal tokens to match")
	}
	if httputil.ConstantTimeEqual(key, key+"x") {
		t.Fatal("expected different-length tokens to not match")
	}
	if httputil.ConstantTimeEqual("short", "much-longer-token") {
		t.Fatal("expected mismatched tokens to not match")
	}
}
