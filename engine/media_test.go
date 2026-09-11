package engine

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/credentials"
)

func newMediaTestEngine(t *testing.T) *Engine {
	t.Helper()
	eng, err := New(Options{SecretStore: &credentials.MapStore{}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return eng
}

func TestEngineGenerateImage(t *testing.T) {
	pngB64 := base64.StdEncoding.EncodeToString([]byte("fakepng"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		fmt.Fprintf(w, `{"created":1,"data":[{"b64_json":%q},{"b64_json":%q}]}`, pngB64, pngB64)
	}))
	defer srv.Close()

	eng := newMediaTestEngine(t)
	results, err := eng.GenerateImage(context.Background(), GenerateImageRequest{
		MediaOptions: MediaOptions{APIKey: "k", BaseURL: srv.URL},
		Prompt:       "a cat", Model: "dall-e-3", Size: "1024x1024", N: 2,
	})
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	if string(results[0].Image) != "fakepng" {
		t.Fatalf("image[0] = %q", results[0].Image)
	}
}

func TestEngineGenerateImageError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"bad key"}}`)
	}))
	defer srv.Close()

	eng := newMediaTestEngine(t)
	_, err := eng.GenerateImage(context.Background(), GenerateImageRequest{
		MediaOptions: MediaOptions{APIKey: "k", BaseURL: srv.URL},
		Prompt:       "p", N: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v, want 401", err)
	}
}

func TestEngineTranscribe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/transcriptions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if !strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Fatal("expected multipart")
		}
		fmt.Fprint(w, `{"text":"hello world"}`)
	}))
	defer srv.Close()

	eng := newMediaTestEngine(t)
	text, err := eng.Transcribe(context.Background(), TranscribeRequest{
		MediaOptions: MediaOptions{APIKey: "k", BaseURL: srv.URL},
		Audio:        []byte("audio-bytes"), FileName: "voice.m4a", Model: "whisper-1",
	})
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if text != "hello world" {
		t.Fatalf("text = %q", text)
	}
}
