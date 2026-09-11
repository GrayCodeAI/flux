package core

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// supportedImageMediaTypes is the allow-list of image formats eyrie will encode
// from a local file or validate from a data-URL. Keeping the set explicit means
// an unsupported type fails fast with a readable error at eyrie's boundary
// rather than as an opaque 400 from the upstream provider.
var supportedImageMediaTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/gif":  true,
}

// extToMediaType maps a file extension (lowercase, no dot) to an image MIME type.
var extToMediaType = map[string]string{
	"jpg":  "image/jpeg",
	"jpeg": "image/jpeg",
	"png":  "image/png",
	"webp": "image/webp",
	"gif":  "image/gif",
}

// NormalizeImageSource turns any of the three image source forms into a canonical
// representation for provider clients:
//
//   - data:<mediaType>;base64,<data>  → returned as base64 (mediaType, data, true)
//   - http(s)://…                     → returned as a pass-through URL ("", url, false)
//   - a local filesystem path         → read, MIME-sniffed by extension, and base64
//     encoded → (mediaType, data, true)
//
// It is the single entry point for image handling so the provider clients and
// hawk no longer each carry their own divergent encoder. Local files and
// data-URLs are validated against supportedImageMediaTypes; HTTP URLs are left
// for the provider to fetch (avoiding an SSRF surface inside eyrie).
func NormalizeImageSource(src string) (mediaType, data string, isBase64 bool, err error) {
	switch {
	case strings.HasPrefix(src, "data:"):
		mt, d, ok := parseDataURL(src)
		if !ok {
			return "", "", false, fmt.Errorf("eyrie: malformed image data URL")
		}
		if mt != "" && !supportedImageMediaTypes[mt] {
			return "", "", false, fmt.Errorf("eyrie: unsupported image format %q (supported: jpeg, png, webp, gif)", mt)
		}
		return mt, d, true, nil

	case strings.HasPrefix(src, "http://"), strings.HasPrefix(src, "https://"):
		// Provider fetches the URL itself; pass through unchanged.
		return "", src, false, nil

	default:
		// A string with a recognized image extension is a local file path; read
		// and encode it. Anything else is treated as raw base64 image data
		// (preserving eyrie's long-standing default), so a bare token is not
		// mistaken for a missing file.
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(src), "."))
		mt, ok := extToMediaType[ext]
		if !ok {
			// Not a path with a known image extension → assume raw base64.
			return "", src, false, nil
		}
		raw, readErr := os.ReadFile(src) // #nosec G304 -- src is a caller-supplied local image path, an intentional API input, not untrusted request data
		if readErr != nil {
			return "", "", false, fmt.Errorf("eyrie: reading image file %q: %w", src, readErr)
		}
		return mt, base64.StdEncoding.EncodeToString(raw), true, nil
	}
}

// parseDataURL splits a data: URL of the form data:<mediaType>;base64,<data>.
func parseDataURL(src string) (mediaType, data string, ok bool) {
	rest := strings.TrimPrefix(src, "data:")
	if semiIdx := strings.Index(rest, ";base64,"); semiIdx >= 0 {
		return rest[:semiIdx], rest[semiIdx+len(";base64,"):], true
	}
	return "", "", false
}

// OpenAIImageURL renders an image source into the single-string form the
// OpenAI-compatible image_url field expects: an http(s) URL or a data URL are
// passed through; a local file is read and encoded into a data URL. On any
// normalization error it falls back to the raw input so the request is not
// dropped (the provider will surface a clearer error than eyrie can here).
func OpenAIImageURL(src string) string {
	// Already a usable URL form: pass through unchanged.
	if strings.HasPrefix(src, "data:") || strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		return src
	}
	mt, data, isBase64, err := NormalizeImageSource(src)
	if err != nil {
		return src
	}
	if isBase64 {
		if mt == "" {
			mt = "image/png"
		}
		return fmt.Sprintf("data:%s;base64,%s", mt, data)
	}
	// A bare token that is neither a path nor a URL is treated as raw base64,
	// defaulting to PNG (eyrie's long-standing behavior).
	return "data:image/png;base64," + data
}

// ParseImageString is the backward-compatible shim retained for existing call
// sites. It now routes through NormalizeImageSource so local paths are encoded
// and formats are validated. On error it falls back to treating the input as a
// pass-through URL, preserving the previous lenient behavior.
func ParseImageString(img string) (mediaType, data string, isBase64 bool) {
	mt, d, b64, err := NormalizeImageSource(img)
	if err != nil {
		return "", img, false
	}
	return mt, d, b64
}
