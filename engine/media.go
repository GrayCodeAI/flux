package engine

import (
	"context"

	"github.com/GrayCodeAI/graycode-router/client"
)

// MediaOptions carries the credentials and endpoint for a media backend call.
// Credentials are supplied per call by the host — no new secret paths are
// introduced, and the engine never stores media credentials.
type MediaOptions struct {
	APIKey  string
	BaseURL string
}

// GenerateImageRequest is the host-facing request for image generation.
type GenerateImageRequest struct {
	MediaOptions
	Prompt string
	Model  string
	Size   string // e.g. "1024x1024"
	N      int
}

// GenerateImageResult is one generated image plus its provider URL when present.
type GenerateImageResult struct {
	Image       []byte
	ProviderURL string
}

// GenerateImage generates images through the OpenAI-compatible endpoint
// configured in req. It is a stateless facade over client.ImageClient,
// returning decoded image bytes (plus any provider URL). The engine keeps no
// media state; the host owns conversation and persistence.
func (e *Engine) GenerateImage(ctx context.Context, req GenerateImageRequest) ([]GenerateImageResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	c := client.NewImageClient(req.APIKey, req.BaseURL)
	imgs, urls, err := c.Generate(ctx, req.Prompt, req.Model, req.Size, req.N)
	if err != nil {
		return nil, err
	}
	out := make([]GenerateImageResult, 0, len(imgs))
	for i := range imgs {
		r := GenerateImageResult{Image: imgs[i]}
		if i < len(urls) {
			r.ProviderURL = urls[i]
		}
		out = append(out, r)
	}
	return out, nil
}

// TranscribeRequest is the host-facing request for audio transcription.
type TranscribeRequest struct {
	MediaOptions
	Audio    []byte
	FileName string
	Model    string
	Language string // optional ISO-639-1
	Prompt   string // optional context/hint
}

// Transcribe transcribes audio through the OpenAI-compatible endpoint
// configured in req, returning the transcript text. It is a stateless facade
// over client.AudioClient.
func (e *Engine) Transcribe(ctx context.Context, req TranscribeRequest) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	c := client.NewAudioClient(req.APIKey, req.BaseURL)
	return c.Transcribe(ctx, client.TranscriptionRequest{
		Model:    req.Model,
		File:     req.Audio,
		FileName: req.FileName,
		Language: req.Language,
		Prompt:   req.Prompt,
	})
}
