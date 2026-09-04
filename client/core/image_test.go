package core

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeImageSource_DataURL(t *testing.T) {
	t.Parallel()
	mt, data, isB64, err := NormalizeImageSource("data:image/png;base64,QUJD")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !isB64 || mt != "image/png" || data != "QUJD" {
		t.Errorf("got (%q,%q,%v)", mt, data, isB64)
	}
}

func TestNormalizeImageSource_DataURLUnsupported(t *testing.T) {
	t.Parallel()
	_, _, _, err := NormalizeImageSource("data:image/tiff;base64,QUJD")
	if err == nil || !strings.Contains(err.Error(), "unsupported image format") {
		t.Errorf("expected unsupported-format error, got %v", err)
	}
}

func TestNormalizeImageSource_HTTPPassthrough(t *testing.T) {
	t.Parallel()
	mt, data, isB64, err := NormalizeImageSource("https://example.com/cat.png")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if isB64 || mt != "" || data != "https://example.com/cat.png" {
		t.Errorf("HTTP URL should pass through unchanged, got (%q,%q,%v)", mt, data, isB64)
	}
}

func TestNormalizeImageSource_LocalFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "pic.jpg")
	raw := []byte("\xff\xd8\xff\xe0fakejpegbytes")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	mt, data, isB64, err := NormalizeImageSource(path)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !isB64 {
		t.Fatal("local file should be base64-encoded")
	}
	if mt != "image/jpeg" {
		t.Errorf("mediaType = %q, want image/jpeg", mt)
	}
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		t.Fatalf("data is not valid base64: %v", err)
	}
	if string(decoded) != string(raw) {
		t.Errorf("round-trip mismatch")
	}
}

func TestNormalizeImageSource_NonImageExtensionTreatedAsRawBase64(t *testing.T) {
	t.Parallel()
	// A token without a recognized image extension is treated as raw base64
	// data, not a file path (preserving graycode-router's long-standing default).
	mt, data, isB64, err := NormalizeImageSource("QUJDtoken")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if isB64 || mt != "" || data != "QUJDtoken" {
		t.Errorf("got (%q,%q,%v), want passthrough raw token", mt, data, isB64)
	}
}

func TestNormalizeImageSource_MissingFile(t *testing.T) {
	t.Parallel()
	_, _, _, err := NormalizeImageSource(filepath.Join(t.TempDir(), "nope.png"))
	if err == nil || !strings.Contains(err.Error(), "reading image file") {
		t.Errorf("expected read error, got %v", err)
	}
}

func TestOpenAIImageURL(t *testing.T) {
	t.Parallel()
	// HTTP passes through.
	if got := OpenAIImageURL("https://x/y.png"); got != "https://x/y.png" {
		t.Errorf("http url = %q", got)
	}
	// data URL passes through.
	if got := OpenAIImageURL("data:image/png;base64,QUJD"); got != "data:image/png;base64,QUJD" {
		t.Errorf("data url = %q", got)
	}
	// Local file becomes a data URL.
	dir := t.TempDir()
	path := filepath.Join(dir, "a.png")
	if err := os.WriteFile(path, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := OpenAIImageURL(path)
	if !strings.HasPrefix(got, "data:image/png;base64,") {
		t.Errorf("local file url = %q, want data:image/png prefix", got)
	}
	// A bare token (no path extension, no scheme) is wrapped as raw base64 PNG.
	if got := OpenAIImageURL("AAAA"); got != "data:image/png;base64,AAAA" {
		t.Errorf("raw base64 = %q, want data:image/png;base64,AAAA", got)
	}
}

// ParseImageString shim must keep its lenient behavior for callers.
func TestParseImageStringShim(t *testing.T) {
	t.Parallel()
	mt, data, isB64 := ParseImageString("data:image/png;base64,QUJD")
	if !isB64 || mt != "image/png" || data != "QUJD" {
		t.Errorf("data url shim got (%q,%q,%v)", mt, data, isB64)
	}
	// An HTTP URL stays a pass-through URL.
	mt, data, isB64 = ParseImageString("https://x/y.png")
	if isB64 || data != "https://x/y.png" {
		t.Errorf("http shim got (%q,%q,%v)", mt, data, isB64)
	}
}
