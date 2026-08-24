package client

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestImageClientGenerateB64(t *testing.T) {
	pngB64 := base64.StdEncoding.EncodeToString([]byte("fakepng"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		fmt.Fprintf(w, `{"created":1,"data":[{"b64_json":%q},{"b64_json":%q}]}`, pngB64, pngB64)
	}))
	defer srv.Close()
	c := NewImageClient("k", srv.URL)
	imgs, urls, err := c.Generate(context.Background(), "a cat", "dall-e-3", "1024x1024", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(imgs) != 2 {
		t.Fatalf("imgs = %d", len(imgs))
	}
	if string(imgs[0]) != "fakepng" {
		t.Fatalf("img0 = %q", imgs[0])
	}
	if len(urls) != 0 {
		t.Fatalf("unexpected urls: %v", urls)
	}
}

func TestImageClientURLResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"created":1,"data":[{"url":"https://example.com/i.png"}]}`)
	}))
	defer srv.Close()
	c := NewImageClient("k", srv.URL)
	imgs, urls, err := c.Generate(context.Background(), "p", "", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(imgs) != 0 || len(urls) != 1 || urls[0] != "https://example.com/i.png" {
		t.Fatalf("imgs=%d urls=%v", len(imgs), urls)
	}
}

func TestImageClientError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"bad key"}}`)
	}))
	defer srv.Close()
	c := NewImageClient("k", srv.URL)
	if _, _, err := c.Generate(context.Background(), "p", "", "", 1); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v", err)
	}
}

func TestAudioClientTranscribe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/transcriptions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		// Verify multipart with a file part.
		if !strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Fatal("expected multipart")
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.FormValue("model") != "whisper-1" {
			t.Fatalf("model = %q", r.FormValue("model"))
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("form file: %v", err)
		}
		file.Close()
		fmt.Fprint(w, `{"text":"hello world"}`)
	}))
	defer srv.Close()
	c := NewAudioClient("k", srv.URL)
	text, err := c.Transcribe(context.Background(), TranscriptionRequest{
		Model: "whisper-1", File: []byte("audiobytes"), FileName: "voice.ogg",
	})
	if err != nil {
		t.Fatal(err)
	}
	if text != "hello world" {
		t.Fatalf("text = %q", text)
	}
}

func TestAudioClientError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"bad audio"}}`)
	}))
	defer srv.Close()
	c := NewAudioClient("k", srv.URL)
	if _, err := c.Transcribe(context.Background(), TranscriptionRequest{Model: "whisper-1", File: []byte("x")}); err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("err = %v", err)
	}
}
