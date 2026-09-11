package adapters

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/eyrie/client/core"
	"github.com/GrayCodeAI/eyrie/types"
)

func TestNewBedrockClient(t *testing.T) {
	t.Parallel()
	c := NewBedrockClient("AKID", "secret", "session", "us-east-1")
	if c == nil {
		t.Fatal("NewBedrockClient returned nil")
	}
	if c.accessKeyID != "AKID" {
		t.Errorf("accessKeyID = %q", c.accessKeyID)
	}
	if string(c.secretAccessKey) != "secret" {
		t.Errorf("secretAccessKey = %q", string(c.secretAccessKey))
	}
	if c.sessionToken != "session" {
		t.Errorf("sessionToken = %q", c.sessionToken)
	}
	if c.region != "us-east-1" {
		t.Errorf("region = %q", c.region)
	}
}

func TestBedrockClient_Name(t *testing.T) {
	t.Parallel()
	c := NewBedrockClient("AKID", "secret", "", "us-east-1")
	if c.Name() != "anthropic-bedrock" {
		t.Errorf("Name() = %q", c.Name())
	}
}

func TestBedrockClient_String(t *testing.T) {
	t.Parallel()
	c := NewBedrockClient("AKID", "my-secret-key-here", "", "us-east-1")
	s := c.String()
	if !strings.Contains(s, "AKID") {
		t.Error("expected access key in string")
	}
	if !strings.Contains(s, "my-s") {
		t.Error("expected partial secret in string")
	}
	if strings.Contains(s, "my-secret-key-here") {
		t.Error("full secret should not appear")
	}
}

func TestBedrockClient_String_ShortSecret(t *testing.T) {
	t.Parallel()
	c := NewBedrockClient("AKID", "abc", "", "us-east-1")
	s := c.String()
	if strings.Contains(s, "abc") {
		t.Error("short secret should be fully masked")
	}
}

func TestBedrockClient_Chat_Success(t *testing.T) {
	t.Parallel()
	var capturedReq *http.Request
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return jsonResponse(http.StatusOK, map[string]any{
			"id": "msg_bedrock_1", "type": "message", "role": "assistant",
			"content":     []map[string]string{{"type": "text", "text": "Hello from Bedrock!"}},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 5, "output_tokens": 10},
		}), nil
	})
	c := NewBedrockClient("AKID", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "", "us-east-1")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}

	resp, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "anthropic.claude-sonnet-4-20250514", MaxTokens: 256})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "Hello from Bedrock!" {
		t.Errorf("content = %q", resp.Content)
	}
	if capturedReq == nil {
		t.Fatal("request not captured")
	}
	if capturedReq.Header.Get("Authorization") == "" {
		t.Error("missing Authorization header (SigV4)")
	}
}

func TestBedrockClient_Chat_EmptyModel(t *testing.T) {
	t.Parallel()
	c := NewBedrockClient("AKID", "secret", "", "us-east-1")
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: ""})
	if err == nil {
		t.Fatal("expected error for empty model")
	}
}

func TestBedrockClient_Chat_EmptyRegion(t *testing.T) {
	t.Parallel()
	c := NewBedrockClient("", "", "", "")
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "claude", MaxTokens: 256})
	if err == nil {
		t.Fatal("expected error for empty credentials")
	}
}

func TestBedrockClient_Chat_APIError(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusForbidden, map[string]any{
			"error": map[string]string{"message": "AccessDenied"},
		}), nil
	})
	c := NewBedrockClient("AKID", "secret", "", "us-east-1")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "claude", MaxTokens: 256})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBedrockClient_Ping_Success(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{}), nil
	})
	c := NewBedrockClient("AKID", "secret", "", "us-east-1")
	c.httpClient = &http.Client{Transport: transport}
	err := c.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestBedrockClient_Ping_IncompleteCredentials(t *testing.T) {
	t.Parallel()
	c := NewBedrockClient("", "", "", "")
	err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("expected error for incomplete credentials")
	}
}

func TestBedrockClient_Ping_Unauthorized(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusUnauthorized, map[string]any{}), nil
	})
	c := NewBedrockClient("AKID", "secret", "", "us-east-1")
	c.httpClient = &http.Client{Transport: transport}
	err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("expected auth error")
	}
}

func TestBedrockClient_modelURL(t *testing.T) {
	t.Parallel()
	c := NewBedrockClient("AKID", "secret", "", "us-east-1")
	url := c.ModelURL("anthropic.claude-sonnet-4-20250514")
	expected := "https://bedrock-runtime.us-east-1.amazonaws.com/model/anthropic.claude-sonnet-4-20250514/invoke"
	if url != expected {
		t.Errorf("modelURL = %q, want %q", url, expected)
	}
}

func TestBedrockClient_BuildBody(t *testing.T) {
	t.Parallel()
	c := NewBedrockClient("AKID", "secret", "", "us-east-1")
	body, err := c.BuildBody([]core.EyrieMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{Model: "claude", MaxTokens: 256})
	if err != nil {
		t.Fatalf("BuildBody: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["anthropic_version"] != "bedrock-2023-05-31" {
		t.Errorf("anthropic_version = %v", parsed["anthropic_version"])
	}
}

func TestBedrockClient_HTTPClientAndRetry(t *testing.T) {
	t.Parallel()
	c := NewBedrockClient("AKID", "secret", "", "us-east-1")
	if c.HTTPClient() == nil {
		t.Error("HTTPClient is nil")
	}
	if c.Retry().MaxRetries != 3 {
		t.Errorf("default MaxRetries = %d", c.Retry().MaxRetries)
	}
	if c.Region() != "us-east-1" {
		t.Errorf("Region = %q", c.Region())
	}
}

func TestCanonicalAWSHeaders(t *testing.T) {
	t.Parallel()
	h := http.Header{}
	h.Set("Host", "bedrock-runtime.us-east-1.amazonaws.com")
	h.Set("X-Amz-Date", "20250101T000000Z")
	canonical, signed := CanonicalAWSHeaders(h)
	if canonical == "" || signed == "" {
		t.Error("expected non-empty canonical headers")
	}
	if !strings.Contains(signed, "host") {
		t.Error("expected host in signed headers")
	}
	if !strings.Contains(signed, "x-amz-date") {
		t.Error("expected x-amz-date in signed headers")
	}
}

func TestAWSCanonicalURI(t *testing.T) {
	t.Parallel()
	if AWSCanonicalURI("") != "/" {
		t.Errorf(`AWSCanonicalURI("") = %q`, AWSCanonicalURI(""))
	}
	if AWSCanonicalURI("/path/to/model") != "/path/to/model" {
		t.Errorf(`AWSCanonicalURI("/path") = %q`, AWSCanonicalURI("/path/to/model"))
	}
}

func TestSha256Hex(t *testing.T) {
	t.Parallel()
	hash := Sha256Hex([]byte("test"))
	if len(hash) != 64 {
		t.Errorf("hash length = %d", len(hash))
	}
	if hash != "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08" {
		t.Errorf("hash = %q", hash)
	}
}

func TestAWSSigningKey(t *testing.T) {
	t.Parallel()
	key := AWSSigningKey("wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "20250101", "us-east-1", "bedrock")
	if len(key) != 32 {
		t.Errorf("key length = %d", len(key))
	}
}

func TestBedrockClient_Sign(t *testing.T) {
	t.Parallel()
	c := NewBedrockClient("AKID", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "", "us-east-1")
	req, _ := http.NewRequest("POST", "https://bedrock-runtime.us-east-1.amazonaws.com/model/claude/invoke", nil)
	err := c.Sign(req, []byte(`{"test":true}`), time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if req.Header.Get("Authorization") == "" {
		t.Error("expected Authorization header")
	}
	if req.Header.Get("X-Amz-Date") == "" {
		t.Error("expected X-Amz-Date header")
	}
	if req.Header.Get("X-Amz-Content-Sha256") == "" {
		t.Error("expected X-Amz-Content-Sha256 header")
	}
}

func TestEventStreamReader(t *testing.T) {
	t.Parallel()
	frame := buildEventStreamFrame("test-payload")
	reader := newEventStreamReader(readerFromBytes(frame))
	evt, err := reader.ReadEvent()
	if err != nil {
		t.Fatalf("ReadEvent: %v", err)
	}
	if string(evt.Payload) != "test-payload" {
		t.Errorf("payload = %q", string(evt.Payload))
	}
}

func TestEventStreamReader_InvalidCRC(t *testing.T) {
	t.Parallel()
	reader := newEventStreamReader(strings.NewReader("garbage"))
	_, err := reader.ReadEvent()
	if err == nil {
		t.Fatal("expected error for garbage data")
	}
}

func TestEventStreamReader_WithHeaders(t *testing.T) {
	t.Parallel()
	payload := `{"type":"content_block_delta","delta":{"text":"hi"}}`
	frame := buildEventStreamFrameWithHeaders(payload, map[string]string{":content-type": "application/json"})
	reader := newEventStreamReader(readerFromBytes(frame))
	evt, err := reader.ReadEvent()
	if err != nil {
		t.Fatalf("ReadEvent: %v", err)
	}
	if string(evt.Payload) != payload {
		t.Errorf("payload = %q", string(evt.Payload))
	}
	if evt.Headers[":content-type"] != "application/json" {
		t.Errorf("content-type header = %q", evt.Headers[":content-type"])
	}
}

func TestEventStreamReader_InvalidTotalLen(t *testing.T) {
	t.Parallel()
	frame := make([]byte, 12) // totalLen = 0
	binary.BigEndian.PutUint32(frame[8:12], crc32.ChecksumIEEE(frame[:8]))
	reader := newEventStreamReader(readerFromBytes(frame))
	_, err := reader.ReadEvent()
	if err == nil {
		t.Fatal("expected error for invalid total length")
	}
}

func TestEventStreamReader_HeadersExceedTotal(t *testing.T) {
	t.Parallel()
	totalLen := uint32(20)
	headersLen := uint32(100) // exceeds total
	prelude := make([]byte, 12)
	binary.BigEndian.PutUint32(prelude[0:4], totalLen)
	binary.BigEndian.PutUint32(prelude[4:8], headersLen)
	binary.BigEndian.PutUint32(prelude[8:12], crc32.ChecksumIEEE(prelude[:8]))
	reader := newEventStreamReader(readerFromBytes(prelude))
	_, err := reader.ReadEvent()
	if err == nil {
		t.Fatal("expected error for headers exceeding total")
	}
}

// buildEventStreamFrameWithHeaders constructs a valid EventStream frame with string headers.
func buildEventStreamFrameWithHeaders(payload string, headers map[string]string) []byte {
	payloadBytes := []byte(payload)
	headerBytes := buildEventStreamHeaders(headers)
	headersLen := len(headerBytes)
	totalLen := 12 + headersLen + len(payloadBytes) + 4
	prelude := make([]byte, 12)
	binary.BigEndian.PutUint32(prelude[0:4], uint32(totalLen))
	binary.BigEndian.PutUint32(prelude[4:8], uint32(headersLen))
	binary.BigEndian.PutUint32(prelude[8:12], crc32.ChecksumIEEE(prelude[:8]))

	crcInput := make([]byte, 0, totalLen-4)
	crcInput = append(crcInput, prelude[:8]...)
	crcInput = append(crcInput, headerBytes...)
	crcInput = append(crcInput, payloadBytes...)
	msgCRC := crc32.ChecksumIEEE(crcInput)

	result := make([]byte, 0, totalLen)
	result = append(result, prelude...)
	result = append(result, headerBytes...)
	result = append(result, payloadBytes...)
	result = append(result, byte(msgCRC>>24), byte(msgCRC>>16), byte(msgCRC>>8), byte(msgCRC))
	return result
}

// buildEventStreamHeaders encodes string headers in EventStream binary format.
func buildEventStreamHeaders(headers map[string]string) []byte {
	var buf []byte
	for name, value := range headers {
		buf = append(buf, byte(len(name)))
		buf = append(buf, []byte(name)...)
		buf = append(buf, 7) // string type
		strLen := len(value)
		buf = append(buf, byte(strLen>>8), byte(strLen))
		buf = append(buf, []byte(value)...)
	}
	return buf
}

func TestBedrockClient_StreamChat_Success(t *testing.T) {
	t.Parallel()
	chunks := []string{
		`{"type":"message_start","message":{"usage":{"input_tokens":5}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" from Bedrock stream!"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
	}
	var eventStreamData []byte
	for _, c := range chunks {
		eventStreamData = append(eventStreamData, buildEventStreamFrame(c)...)
	}

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/vnd.amazon.eventstream"}},
			Body:       io.NopCloser(bytes.NewReader(eventStreamData)),
		}, nil
	})
	c := NewBedrockClient("AKID", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "", "us-east-1")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}

	result, err := c.StreamChat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "anthropic.claude-sonnet-4-20250514", MaxTokens: 256})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	defer result.Close()

	var content string
	for evt := range result.Events {
		if evt.Type == "content" {
			content += evt.Content
		}
	}
	if content != "Hello from Bedrock stream!" {
		t.Errorf("content = %q", content)
	}
}

func TestBedrockClient_StreamChat_Error(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusForbidden, map[string]any{
			"error": map[string]string{"message": "AccessDenied"},
		}), nil
	})
	c := NewBedrockClient("AKID", "secret", "", "us-east-1")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	_, err := c.StreamChat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "claude", MaxTokens: 256})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBedrockClient_StreamChat_EmptyModel(t *testing.T) {
	t.Parallel()
	c := NewBedrockClient("AKID", "secret", "", "us-east-1")
	_, err := c.StreamChat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: ""})
	if err == nil {
		t.Fatal("expected error for empty model")
	}
}

// buildEventStreamFrame constructs a valid Amazon EventStream frame containing the given payload.
func buildEventStreamFrame(payload string) []byte {
	payloadBytes := []byte(payload)
	headersLen := 0
	totalLen := 12 + headersLen + len(payloadBytes) + 4
	prelude := make([]byte, 12)
	binary.BigEndian.PutUint32(prelude[0:4], uint32(totalLen))
	binary.BigEndian.PutUint32(prelude[4:8], uint32(headersLen))

	preludeCRC := crc32.ChecksumIEEE(prelude[:8])
	binary.BigEndian.PutUint32(prelude[8:12], preludeCRC)

	crcInput := make([]byte, 0, totalLen-4)
	crcInput = append(crcInput, prelude[:8]...)
	crcInput = append(crcInput, payloadBytes...)
	msgCRC := crc32.ChecksumIEEE(crcInput)

	result := make([]byte, 0, totalLen)
	result = append(result, prelude...)
	result = append(result, payloadBytes...)
	result = append(result, byte(msgCRC>>24), byte(msgCRC>>16), byte(msgCRC>>8), byte(msgCRC))
	return result
}

// readerFromBytes returns an io.Reader from a byte slice.
func readerFromBytes(b []byte) *strings.Reader {
	return strings.NewReader(string(b))
}

func TestBedrockClient_Chat_DefaultMaxTokens(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body map[string]interface{}
		if err := jsonDecodeRequest(req, &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body["max_tokens"] != float64(4096) {
			t.Errorf("max_tokens = %v, expected 4096", body["max_tokens"])
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"id": "msg_bedrock_1", "type": "message", "role": "assistant",
			"content":     []map[string]string{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 5, "output_tokens": 10},
		}), nil
	})
	c := NewBedrockClient("AKID", "secret", "", "us-east-1")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "anthropic.claude-sonnet-4-20250514"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

func TestBedrockClient_Chat_SystemPrompt(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body map[string]interface{}
		if err := jsonDecodeRequest(req, &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		sys, ok := body["system"].(string)
		if !ok {
			t.Fatalf("system is not a string: %T", body["system"])
		}
		if !strings.Contains(sys, "You are a helpful assistant") {
			t.Errorf("system = %q, expected to contain custom system", sys)
		}
		if !strings.Contains(sys, "system from message") {
			t.Errorf("system = %q, expected to contain message system", sys)
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"id": "msg_bedrock_1", "type": "message", "role": "assistant",
			"content":     []map[string]string{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 5, "output_tokens": 10},
		}), nil
	})
	c := NewBedrockClient("AKID", "secret", "", "us-east-1")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	_, err := c.Chat(context.Background(), []core.EyrieMessage{
		{Role: "system", Content: "system from message"},
		{Role: "user", Content: "Hi"},
	}, core.ChatOptions{Model: "anthropic.claude-sonnet-4-20250514", MaxTokens: 256, System: "You are a helpful assistant"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

func TestBedrockClient_Chat_SystemOnlyOpts(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body map[string]interface{}
		if err := jsonDecodeRequest(req, &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		sys, ok := body["system"].(string)
		if !ok {
			t.Fatalf("system is not a string: %T", body["system"])
		}
		if sys != "only from opts" {
			t.Errorf("system = %q, expected 'only from opts'", sys)
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"id": "msg_bedrock_1", "type": "message", "role": "assistant",
			"content":     []map[string]string{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 5, "output_tokens": 10},
		}), nil
	})
	c := NewBedrockClient("AKID", "secret", "", "us-east-1")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "anthropic.claude-sonnet-4-20250514", MaxTokens: 256, System: "only from opts"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}
