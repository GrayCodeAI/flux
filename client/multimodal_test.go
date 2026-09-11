//nolint:errcheck
package client

import (
	"encoding/json"
	"testing"
)

// --- Helper function tests ---

func TestNewImageMessage_URL(t *testing.T) {
	t.Parallel()
	msg := NewImageMessage("https://example.com/cat.jpg")
	if msg.Role != "user" {
		t.Errorf("expected role=user, got %s", msg.Role)
	}
	if len(msg.ContentParts) != 1 {
		t.Fatalf("expected 1 content part, got %d", len(msg.ContentParts))
	}
	part := msg.ContentParts[0]
	if part.Type != "image_url" {
		t.Errorf("expected type=image_url, got %s", part.Type)
	}
	if part.ImageURL == nil {
		t.Fatal("expected ImageURL to be set")
	}
	if part.ImageURL.URL != "https://example.com/cat.jpg" {
		t.Errorf("unexpected URL: %s", part.ImageURL.URL)
	}
}

func TestNewImageMessageWithText(t *testing.T) {
	t.Parallel()
	msg := NewImageMessageWithText("What is this?", "https://example.com/pic.png")
	if msg.Role != "user" {
		t.Errorf("expected role=user, got %s", msg.Role)
	}
	if len(msg.ContentParts) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(msg.ContentParts))
	}
	if msg.ContentParts[0].Type != "text" || msg.ContentParts[0].Text != "What is this?" {
		t.Errorf("unexpected first part: %+v", msg.ContentParts[0])
	}
	if msg.ContentParts[1].Type != "image_url" || msg.ContentParts[1].ImageURL.URL != "https://example.com/pic.png" {
		t.Errorf("unexpected second part: %+v", msg.ContentParts[1])
	}
}

func TestNewBase64ImageMessage(t *testing.T) {
	t.Parallel()
	msg := NewBase64ImageMessage("iVBORw0KGgoAAAANS", "image/png")
	if msg.Role != "user" {
		t.Errorf("expected role=user, got %s", msg.Role)
	}
	if len(msg.ContentParts) != 1 {
		t.Fatalf("expected 1 content part, got %d", len(msg.ContentParts))
	}
	part := msg.ContentParts[0]
	if part.Type != "image_url" {
		t.Errorf("expected type=image_url, got %s", part.Type)
	}
	if part.ImageURL == nil {
		t.Fatal("expected ImageURL to be set")
	}
	expectedURL := "data:image/png;base64,iVBORw0KGgoAAAANS"
	if part.ImageURL.URL != expectedURL {
		t.Errorf("expected URL=%q, got %q", expectedURL, part.ImageURL.URL)
	}
}

func TestNewBase64ImageMessageWithText(t *testing.T) {
	t.Parallel()
	msg := NewBase64ImageMessageWithText("Describe this image", "/9j/4AAQ", "image/jpeg")
	if len(msg.ContentParts) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(msg.ContentParts))
	}
	if msg.ContentParts[0].Type != "text" || msg.ContentParts[0].Text != "Describe this image" {
		t.Errorf("unexpected first part: %+v", msg.ContentParts[0])
	}
	expectedURL := "data:image/jpeg;base64,/9j/4AAQ"
	if msg.ContentParts[1].ImageURL.URL != expectedURL {
		t.Errorf("expected URL=%q, got %q", expectedURL, msg.ContentParts[1].ImageURL.URL)
	}
}

func TestNewAudioMessage(t *testing.T) {
	t.Parallel()
	msg := NewAudioMessage("UklGRiQAAABXQVZF", "wav")
	if msg.Role != "user" {
		t.Errorf("expected role=user, got %s", msg.Role)
	}
	if len(msg.ContentParts) != 1 {
		t.Fatalf("expected 1 content part, got %d", len(msg.ContentParts))
	}
	part := msg.ContentParts[0]
	if part.Type != "input_audio" {
		t.Errorf("expected type=input_audio, got %s", part.Type)
	}
	if part.InputAudio == nil {
		t.Fatal("expected InputAudio to be set")
	}
	if part.InputAudio.Data != "UklGRiQAAABXQVZF" {
		t.Errorf("unexpected audio data: %s", part.InputAudio.Data)
	}
	if part.InputAudio.Format != "wav" {
		t.Errorf("expected format=wav, got %s", part.InputAudio.Format)
	}
}

func TestNewAudioMessageWithText(t *testing.T) {
	t.Parallel()
	msg := NewAudioMessageWithText("Transcribe this", "SGVsbG8=", "mp3")
	if len(msg.ContentParts) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(msg.ContentParts))
	}
	if msg.ContentParts[0].Type != "text" || msg.ContentParts[0].Text != "Transcribe this" {
		t.Errorf("unexpected first part: %+v", msg.ContentParts[0])
	}
	if msg.ContentParts[1].Type != "input_audio" {
		t.Errorf("expected input_audio, got %s", msg.ContentParts[1].Type)
	}
	if msg.ContentParts[1].InputAudio.Format != "mp3" {
		t.Errorf("expected format=mp3, got %s", msg.ContentParts[1].InputAudio.Format)
	}
}

// --- OpenAI ContentParts serialization tests ---

func TestOpenAI_ContentParts_ImageURL(t *testing.T) {
	t.Parallel()
	msgs := []EyrieMessage{
		NewImageMessage("https://example.com/cat.jpg"),
	}
	req := buildRequestBase(msgs, ChatOptions{Model: "gpt-4o"}, false, nil)
	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(req.Messages))
	}
	content, ok := req.Messages[0]["content"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected content to be []map, got %T", req.Messages[0]["content"])
	}
	if len(content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(content))
	}
	if content[0]["type"] != "image_url" {
		t.Errorf("expected type=image_url, got %v", content[0]["type"])
	}
	imgURL, ok := content[0]["image_url"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected image_url map, got %T", content[0]["image_url"])
	}
	if imgURL["url"] != "https://example.com/cat.jpg" {
		t.Errorf("unexpected URL: %v", imgURL["url"])
	}
}

func TestOpenAI_ContentParts_ImageURLWithDetail(t *testing.T) {
	t.Parallel()
	msgs := []EyrieMessage{{
		Role: "user",
		ContentParts: []ContentPart{
			{Type: "image_url", ImageURL: &ImageURLPart{URL: "https://example.com/cat.jpg", Detail: "high"}},
		},
	}}
	req := buildRequestBase(msgs, ChatOptions{Model: "gpt-4o"}, false, nil)
	content := req.Messages[0]["content"].([]map[string]interface{})
	imgURL := content[0]["image_url"].(map[string]interface{})
	if imgURL["detail"] != "high" {
		t.Errorf("expected detail=high, got %v", imgURL["detail"])
	}
}

func TestOpenAI_ContentParts_TextPlusImage(t *testing.T) {
	t.Parallel()
	msgs := []EyrieMessage{
		NewImageMessageWithText("What is this?", "https://example.com/pic.png"),
	}
	req := buildRequestBase(msgs, ChatOptions{Model: "gpt-4o"}, false, nil)
	content := req.Messages[0]["content"].([]map[string]interface{})
	if len(content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(content))
	}
	if content[0]["type"] != "text" || content[0]["text"] != "What is this?" {
		t.Errorf("unexpected text block: %+v", content[0])
	}
	if content[1]["type"] != "image_url" {
		t.Errorf("expected image_url, got %v", content[1]["type"])
	}
}

func TestOpenAI_ContentParts_Audio(t *testing.T) {
	t.Parallel()
	msgs := []EyrieMessage{
		NewAudioMessage("UklGRiQAAABXQVZF", "wav"),
	}
	req := buildRequestBase(msgs, ChatOptions{Model: "gpt-4o-audio-preview"}, false, nil)
	content := req.Messages[0]["content"].([]map[string]interface{})
	if len(content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(content))
	}
	if content[0]["type"] != "input_audio" {
		t.Errorf("expected type=input_audio, got %v", content[0]["type"])
	}
	audio, ok := content[0]["input_audio"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected input_audio map, got %T", content[0]["input_audio"])
	}
	if audio["data"] != "UklGRiQAAABXQVZF" {
		t.Errorf("unexpected audio data: %v", audio["data"])
	}
	if audio["format"] != "wav" {
		t.Errorf("unexpected format: %v", audio["format"])
	}
}

func TestOpenAI_ContentParts_TextPlusAudio(t *testing.T) {
	t.Parallel()
	msgs := []EyrieMessage{
		NewAudioMessageWithText("Transcribe this audio", "SGVsbG8=", "mp3"),
	}
	req := buildRequestBase(msgs, ChatOptions{Model: "gpt-4o-audio-preview"}, false, nil)
	content := req.Messages[0]["content"].([]map[string]interface{})
	if len(content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(content))
	}
	if content[0]["type"] != "text" || content[0]["text"] != "Transcribe this audio" {
		t.Errorf("unexpected text block: %+v", content[0])
	}
	if content[1]["type"] != "input_audio" {
		t.Errorf("expected input_audio, got %v", content[1]["type"])
	}
}

func TestOpenAI_ContentParts_MixedImageAndAudio(t *testing.T) {
	t.Parallel()
	msgs := []EyrieMessage{{
		Role: "user",
		ContentParts: []ContentPart{
			{Type: "text", Text: "Analyze this"},
			{Type: "image_url", ImageURL: &ImageURLPart{URL: "https://example.com/pic.jpg"}},
			{Type: "input_audio", InputAudio: &InputAudioPart{Data: "base64data", Format: "wav"}},
		},
	}}
	req := buildRequestBase(msgs, ChatOptions{Model: "gpt-4o"}, false, nil)
	content := req.Messages[0]["content"].([]map[string]interface{})
	if len(content) != 3 {
		t.Fatalf("expected 3 content blocks, got %d", len(content))
	}
	if content[0]["type"] != "text" {
		t.Errorf("expected text, got %v", content[0]["type"])
	}
	if content[1]["type"] != "image_url" {
		t.Errorf("expected image_url, got %v", content[1]["type"])
	}
	if content[2]["type"] != "input_audio" {
		t.Errorf("expected input_audio, got %v", content[2]["type"])
	}
}

// Test that ContentParts take precedence over Images
func TestOpenAI_ContentParts_PrecedenceOverImages(t *testing.T) {
	t.Parallel()
	msgs := []EyrieMessage{{
		Role:         "user",
		Content:      "Old text",
		Images:       []string{"https://example.com/old.jpg"},
		ContentParts: []ContentPart{{Type: "text", Text: "New text"}},
	}}
	req := buildRequestBase(msgs, ChatOptions{Model: "gpt-4o"}, false, nil)
	content := req.Messages[0]["content"].([]map[string]interface{})
	if len(content) != 1 {
		t.Fatalf("expected 1 content block (ContentParts takes precedence), got %d", len(content))
	}
	if content[0]["text"] != "New text" {
		t.Errorf("expected 'New text', got %v", content[0]["text"])
	}
}

// Test that legacy Images still work
func TestOpenAI_LegacyImages_StillWork(t *testing.T) {
	t.Parallel()
	msgs := []EyrieMessage{{
		Role:    "user",
		Content: "Describe this",
		Images:  []string{"https://example.com/cat.jpg"},
	}}
	req := buildRequestBase(msgs, ChatOptions{Model: "gpt-4o"}, false, nil)
	content := req.Messages[0]["content"].([]map[string]interface{})
	if len(content) != 2 {
		t.Fatalf("expected 2 content blocks (text + image), got %d", len(content))
	}
	if content[0]["type"] != "text" {
		t.Errorf("expected text first, got %v", content[0]["type"])
	}
	if content[1]["type"] != "image_url" {
		t.Errorf("expected image_url, got %v", content[1]["type"])
	}
}

// --- Anthropic ContentParts serialization tests ---

func TestAnthropic_ContentParts_ImageURL(t *testing.T) {
	t.Parallel()
	msgs := []EyrieMessage{
		NewImageMessage("https://example.com/cat.jpg"),
	}
	result, _ := buildAnthropicMessages(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	content, ok := result[0]["content"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected multi-part content, got %T", result[0]["content"])
	}
	if len(content) != 1 {
		t.Fatalf("expected 1 block, got %d", len(content))
	}
	if content[0]["type"] != "image" {
		t.Errorf("expected image type, got %v", content[0]["type"])
	}
	source := content[0]["source"].(map[string]interface{})
	if source["type"] != "url" {
		t.Errorf("expected url source type, got %v", source["type"])
	}
	if source["url"] != "https://example.com/cat.jpg" {
		t.Errorf("unexpected URL: %v", source["url"])
	}
}

func TestAnthropic_ContentParts_ImageBase64(t *testing.T) {
	t.Parallel()
	msgs := []EyrieMessage{
		NewBase64ImageMessage("iVBORw0KGgoAAAANS", "image/png"),
	}
	result, _ := buildAnthropicMessages(msgs)
	content := result[0]["content"].([]map[string]interface{})
	if content[0]["type"] != "image" {
		t.Errorf("expected image type, got %v", content[0]["type"])
	}
	source := content[0]["source"].(map[string]interface{})
	if source["type"] != "base64" {
		t.Errorf("expected base64 source type, got %v", source["type"])
	}
	if source["media_type"] != "image/png" {
		t.Errorf("expected media_type=image/png, got %v", source["media_type"])
	}
	if source["data"] != "iVBORw0KGgoAAAANS" {
		t.Errorf("unexpected data: %v", source["data"])
	}
}

func TestAnthropic_ContentParts_TextPlusImage(t *testing.T) {
	t.Parallel()
	msgs := []EyrieMessage{
		NewImageMessageWithText("What is this?", "https://example.com/pic.png"),
	}
	result, _ := buildAnthropicMessages(msgs)
	content := result[0]["content"].([]map[string]interface{})
	if len(content) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(content))
	}
	if content[0]["type"] != "text" || content[0]["text"] != "What is this?" {
		t.Errorf("unexpected text block: %+v", content[0])
	}
	if content[1]["type"] != "image" {
		t.Errorf("expected image type, got %v", content[1]["type"])
	}
}

func TestAnthropic_ContentParts_AudioWAV(t *testing.T) {
	t.Parallel()
	msgs := []EyrieMessage{
		NewAudioMessage("UklGRiQAAABXQVZF", "wav"),
	}
	result, _ := buildAnthropicMessages(msgs)
	content := result[0]["content"].([]map[string]interface{})
	if len(content) != 1 {
		t.Fatalf("expected 1 block, got %d", len(content))
	}
	if content[0]["type"] != "audio" {
		t.Errorf("expected audio type, got %v", content[0]["type"])
	}
	source := content[0]["source"].(map[string]interface{})
	if source["type"] != "base64" {
		t.Errorf("expected base64 source type, got %v", source["type"])
	}
	if source["media_type"] != "audio/wav" {
		t.Errorf("expected media_type=audio/wav, got %v", source["media_type"])
	}
	if source["data"] != "UklGRiQAAABXQVZF" {
		t.Errorf("unexpected data: %v", source["data"])
	}
}

func TestAnthropic_ContentParts_AudioMP3(t *testing.T) {
	t.Parallel()
	msgs := []EyrieMessage{
		NewAudioMessage("SGVsbG8=", "mp3"),
	}
	result, _ := buildAnthropicMessages(msgs)
	content := result[0]["content"].([]map[string]interface{})
	source := content[0]["source"].(map[string]interface{})
	if source["media_type"] != "audio/mpeg" {
		t.Errorf("expected media_type=audio/mpeg, got %v", source["media_type"])
	}
}

func TestAnthropic_ContentParts_TextPlusAudio(t *testing.T) {
	t.Parallel()
	msgs := []EyrieMessage{
		NewAudioMessageWithText("Transcribe this", "SGVsbG8=", "wav"),
	}
	result, _ := buildAnthropicMessages(msgs)
	content := result[0]["content"].([]map[string]interface{})
	if len(content) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(content))
	}
	if content[0]["type"] != "text" || content[0]["text"] != "Transcribe this" {
		t.Errorf("unexpected text block: %+v", content[0])
	}
	if content[1]["type"] != "audio" {
		t.Errorf("expected audio type, got %v", content[1]["type"])
	}
}

func TestAnthropic_ContentParts_PrecedenceOverImages(t *testing.T) {
	t.Parallel()
	msgs := []EyrieMessage{{
		Role:         "user",
		Content:      "Old text",
		Images:       []string{"https://example.com/old.jpg"},
		ContentParts: []ContentPart{{Type: "text", Text: "New text"}},
	}}
	result, _ := buildAnthropicMessages(msgs)
	content := result[0]["content"].([]map[string]interface{})
	if len(content) != 1 {
		t.Fatalf("expected 1 block (ContentParts takes precedence), got %d", len(content))
	}
	if content[0]["text"] != "New text" {
		t.Errorf("expected 'New text', got %v", content[0]["text"])
	}
}

// --- audioFormatToMediaType tests ---

func TestAudioFormatToMediaType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		expected string
	}{
		{"wav", "audio/wav"},
		{"mp3", "audio/mpeg"},
		{"flac", "audio/flac"},
		{"ogg", "audio/ogg"},
		{"aac", "audio/aac"},
		{"webm", "audio/webm"},
		{"WAV", "audio/wav"},
		{"MP3", "audio/mpeg"},
		{"audio/wav", "audio/wav"},
		{"audio/mpeg", "audio/mpeg"},
		{"custom", "audio/custom"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := audioFormatToMediaType(tt.input)
			if got != tt.expected {
				t.Errorf("AudioFormatToMediaType(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// --- JSON serialization round-trip test ---

func TestContentParts_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	original := EyrieMessage{
		Role: "user",
		ContentParts: []ContentPart{
			{Type: "text", Text: "What do you see?"},
			{Type: "image_url", ImageURL: &ImageURLPart{URL: "https://example.com/img.png", Detail: "high"}},
			{Type: "input_audio", InputAudio: &InputAudioPart{Data: "base64audio", Format: "wav"}},
		},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	var decoded EyrieMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if decoded.Role != "user" {
		t.Errorf("expected role=user, got %s", decoded.Role)
	}
	if len(decoded.ContentParts) != 3 {
		t.Fatalf("expected 3 content parts, got %d", len(decoded.ContentParts))
	}
	if decoded.ContentParts[0].Type != "text" || decoded.ContentParts[0].Text != "What do you see?" {
		t.Errorf("unexpected first part: %+v", decoded.ContentParts[0])
	}
	if decoded.ContentParts[1].ImageURL == nil || decoded.ContentParts[1].ImageURL.URL != "https://example.com/img.png" {
		t.Errorf("unexpected second part: %+v", decoded.ContentParts[1])
	}
	if decoded.ContentParts[1].ImageURL.Detail != "high" {
		t.Errorf("expected detail=high, got %s", decoded.ContentParts[1].ImageURL.Detail)
	}
	if decoded.ContentParts[2].InputAudio == nil || decoded.ContentParts[2].InputAudio.Data != "base64audio" {
		t.Errorf("unexpected third part: %+v", decoded.ContentParts[2])
	}
}
