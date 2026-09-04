package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// OpenAI-compatible image generation and audio transcription clients.
//
// These are provider-agnostic backends for the two well-defined, broadly
// supported public APIs (OpenAI Images: POST /v1/images/generations; OpenAI
// Audio: POST /v1/audio/transcriptions). They give hawk's pluggable
// MediaEngine / Transcriber seams a concrete default backend while staying
// testable against an httptest server. A future provider (xAI image-gen,
// etc.) can replace the endpoint/credentials without touching callers.

// ImageGenRequest is the body for POST /v1/images/generations.
type ImageGenRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"` // "url" | "b64_json"
}

// ImageGenResult is one generated image.
type ImageGenResult struct {
	URL           string `json:"url,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

// ImageGenResponse is the top-level response.
type ImageGenResponse struct {
	Created int64            `json:"created"`
	Data    []ImageGenResult `json:"data"`
}

// ImageClient generates images via an OpenAI-compatible endpoint.
type ImageClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// NewImageClient creates an image client. baseURL defaults to
// https://api.openai.com; set it to an OpenAI-compatible endpoint for others.
func NewImageClient(apiKey, baseURL string) *ImageClient {
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	return &ImageClient{apiKey: apiKey, baseURL: baseURL, httpClient: NewPooledHTTPClient(2 * time.Minute)}
}

// Generate creates n images for prompt. Returns each as bytes (b64 decoded)
// plus the provider URL when present. size is e.g. "1024x1024".
func (c *ImageClient) Generate(ctx context.Context, prompt, model, size string, n int) ([][]byte, []string, error) {
	if n <= 0 {
		n = 1
	}
	body, err := json.Marshal(ImageGenRequest{
		Model: model, Prompt: prompt, N: n, Size: size,
		ResponseFormat: "b64_json",
	})
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/images/generations", bytes.NewReader(body)) // #nosec G107 -- configured base URL
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("graycode-router: image generate: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, nil, fmt.Errorf("graycode-router: image API %d: %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
	}
	var out ImageGenResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, nil, fmt.Errorf("graycode-router: image decode: %w", err)
	}
	imgs := make([][]byte, 0, len(out.Data))
	urls := make([]string, 0, len(out.Data))
	for _, d := range out.Data {
		if d.B64JSON != "" {
			b, derr := base64.StdEncoding.DecodeString(d.B64JSON)
			if derr != nil {
				return nil, nil, fmt.Errorf("graycode-router: image b64 decode: %w", derr)
			}
			imgs = append(imgs, b)
		}
		if d.URL != "" {
			urls = append(urls, d.URL)
		}
	}
	return imgs, urls, nil
}

// TranscriptionRequest is the multipart body for /v1/audio/transcriptions.
// audio is the raw bytes; model is the transcription model.
type TranscriptionRequest struct {
	Model    string
	File     []byte
	FileName string
	Language string // optional ISO-639-1
	Prompt   string // optional context/hint
}

// Transcript is the response.
type Transcript struct {
	Text string `json:"text"`
}

// AudioClient transcribes audio via an OpenAI-compatible endpoint.
type AudioClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// NewAudioClient creates a transcription client.
func NewAudioClient(apiKey, baseURL string) *AudioClient {
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	return &AudioClient{apiKey: apiKey, baseURL: baseURL, httpClient: NewPooledHTTPClient(2 * time.Minute)}
}

// Transcribe sends the audio file and returns the transcript text.
func (c *AudioClient) Transcribe(ctx context.Context, r TranscriptionRequest) (string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	if err := writer.WriteField("model", r.Model); err != nil {
		return "", err
	}
	if r.Language != "" {
		if err := writer.WriteField("language", r.Language); err != nil {
			return "", err
		}
	}
	if r.Prompt != "" {
		if err := writer.WriteField("prompt", r.Prompt); err != nil {
			return "", err
		}
	}
	name := r.FileName
	if name == "" {
		name = "audio.wav"
	}
	fw, err := writer.CreateFormFile("file", name)
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(r.File); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/audio/transcriptions", &buf) // #nosec G107 -- configured base URL
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("graycode-router: transcribe: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("graycode-router: audio API %d: %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
	}
	var tr Transcript
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", fmt.Errorf("graycode-router: transcript decode: %w", err)
	}
	return tr.Text, nil
}
