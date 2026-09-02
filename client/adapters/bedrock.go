package adapters

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/GrayCodeAI/eyrie/client/core"
	"github.com/GrayCodeAI/eyrie/llm"
)

const (
	// maxBedrockRequestSize is the maximum request body size for Bedrock (30 MB).
	maxBedrockRequestSize = 30 * 1024 * 1024
)

type BedrockClient struct {
	accessKeyID     string
	secretAccessKey []byte
	sessionToken    string
	region          string
	httpClient      *http.Client
	retry           core.RetryConfig
	logger          *slog.Logger
	guardrails      *core.Guardrails
}

// String returns a safe string representation of the Bedrock client.
// Sensitive fields (secret access key) are masked to prevent accidental exposure in logs.
func (c *BedrockClient) String() string {
	masked := "****"
	if len(c.secretAccessKey) > 4 {
		masked = string(c.secretAccessKey[:4]) + "****"
	}
	return fmt.Sprintf("BedrockClient{access_key_id=%s, region=%s, secret_access_key=%s}", c.accessKeyID, c.region, masked)
}

var _ core.Provider = (*BedrockClient)(nil)

func NewBedrockClient(accessKeyID, secretAccessKey, sessionToken, region string) *BedrockClient {
	return &BedrockClient{
		accessKeyID:     accessKeyID,
		secretAccessKey: []byte(secretAccessKey),
		sessionToken:    sessionToken,
		region:          region,
		httpClient:      core.NewPooledHTTPClient(core.DefaultTimeout),
		retry:           core.DefaultRetryConfig(),
		logger:          slog.Default().With("component", "bedrock"),
	}
}

func (c *BedrockClient) Name() string { return "anthropic-bedrock" }

func (c *BedrockClient) Chat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.EyrieResponse, error) {
	if opts.Model == "" {
		return nil, fmt.Errorf("eyrie: model is required for bedrock")
	}
	body, err := c.buildBody(messages, opts)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.modelURL(opts.Model), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("eyrie: bedrock request creation failed: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	if err := c.sign(req, body, time.Now().UTC()); err != nil {
		return nil, err
	}

	resp, err := core.DoWithRetry(ctx, c.httpClient, req, c.retry, c.logger)
	if err != nil {
		return nil, fmt.Errorf("eyrie: bedrock request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("bedrock: close response body", "error", err)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		detail, readErr := core.ParseProviderError(resp.Body)
		return nil, core.FormatAPIError("bedrock", "chat", resp.StatusCode, resp.Header.Get("X-Amzn-Requestid"), detail, readErr)
	}

	var ar anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return nil, fmt.Errorf("eyrie: bedrock decode failed: %w", err)
	}
	eyrieResp := ParseAnthropicResponse(ar, resp.Header.Get("X-Amzn-Requestid"), "")

	if err := core.ApplyGuardrails(ctx, eyrieResp, c.guardrails); err != nil {
		return nil, err
	}

	return eyrieResp, nil
}

func (c *BedrockClient) StreamChat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	if opts.Model == "" {
		return nil, fmt.Errorf("eyrie: model is required for bedrock")
	}
	body, err := c.buildBody(messages, opts)
	if err != nil {
		return nil, err
	}
	// Set stream: true for Bedrock invoke-with-response-stream
	var bodyMap map[string]interface{}
	if err := json.Unmarshal(body, &bodyMap); err != nil {
		return nil, err
	}
	bodyMap["stream"] = true
	streamBody, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, err
	}

	streamURL := strings.Replace(c.modelURL(opts.Model), "/invoke", "/invoke-with-response-stream", 1)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, streamURL, bytes.NewReader(streamBody))
	if err != nil {
		return nil, fmt.Errorf("eyrie: bedrock stream request creation failed: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.amazon.eventstream")
	req.Header.Set("Content-Type", "application/json")
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(streamBody)), nil }
	if err := c.sign(req, streamBody, time.Now().UTC()); err != nil {
		return nil, err
	}

	resp, err := core.DoWithRetry(ctx, c.httpClient, req, c.retry, c.logger) //nolint:bodyclose // closed in goroutine for streaming
	if err != nil {
		return nil, fmt.Errorf("eyrie: bedrock stream request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer func() {
			if err := resp.Body.Close(); err != nil {
				slog.Warn("bedrock: close error response body", "error", err)
			}
		}()
		detail, readErr := core.ParseProviderError(resp.Body)
		return nil, core.FormatAPIError("bedrock stream", "stream", resp.StatusCode, resp.Header.Get("X-Amzn-Requestid"), detail, readErr)
	}

	streamCtx, cancel := context.WithCancel(ctx)
	ch := make(chan core.EyrieStreamEvent, 64)

	go func() {
		defer close(ch)
		defer cancel()
		defer func() {
			if err := resp.Body.Close(); err != nil {
				slog.Warn("bedrock: close stream response body", "error", err)
			}
		}()

		var contentBuf strings.Builder
		var usage *core.EyrieUsage
		var finishReason string

		// Bedrock uses Amazon EventStream format. Parse it.
		reader := newEventStreamReader(resp.Body)
		for {
			event, err := func() (*eventStreamFrame, error) {
				var result *eventStreamFrame
				var readErr error
				done := make(chan struct{})
				go func() {
					result, readErr = reader.ReadEvent()
					close(done)
				}()
				select {
				case <-done:
					return result, readErr
				case <-streamCtx.Done():
					return nil, streamCtx.Err()
				}
			}()
			if err != nil {
				if err != io.EOF {
					select {
					case ch <- core.EyrieStreamEvent{Type: "error", Content: err.Error()}:
					case <-streamCtx.Done():
					}
				}
				break
			}

			// Parse the chunk payload
			var chunk anthropicStreamChunk
			if err := json.Unmarshal(event.Payload, &chunk); err != nil {
				continue
			}

			switch chunk.Type {
			case "content_block_delta":
				if chunk.Delta != nil && chunk.Delta.Text != "" {
					contentBuf.WriteString(chunk.Delta.Text)
					select {
					case ch <- core.EyrieStreamEvent{Type: "content", Content: chunk.Delta.Text}:
					case <-streamCtx.Done():
						return
					}
				}
				if chunk.Delta != nil && chunk.Delta.Type == "input_json_delta" && chunk.Delta.PartialJSON != "" {
					// Accumulate tool input JSON
					select {
					case ch <- core.EyrieStreamEvent{Type: "tool_input_delta", Content: chunk.Delta.PartialJSON}:
					case <-streamCtx.Done():
						return
					}
				}
			case "content_block_start":
				if chunk.ContentBlock != nil && chunk.ContentBlock.Type == "tool_use" {
					var args map[string]interface{}
					_ = json.Unmarshal(chunk.ContentBlock.Input, &args)
					tc := core.ToolCall{ID: chunk.ContentBlock.ID, Name: chunk.ContentBlock.Name, Arguments: args}
					select {
					case ch <- core.EyrieStreamEvent{Type: "tool_call", ToolCall: &tc}:
					case <-streamCtx.Done():
						return
					}
				}
			case "message_delta":
				if chunk.Delta != nil && chunk.Delta.StopReason != "" {
					finishReason = chunk.Delta.StopReason
				}
			case "message_start":
				if chunk.Message != nil && chunk.Message.Usage != nil {
					usage = chunk.Message.Usage
				}
			}
		}

		// Send final done event with usage (matching Anthropic/OpenAI pattern)
		doneEvt := core.EyrieStreamEvent{Type: "done", StopReason: finishReason}
		if usage != nil {
			doneEvt.Usage = usage
		}
		select {
		case ch <- doneEvt:
		case <-streamCtx.Done():
		}
	}()

	return llm.NewStreamResult(ch, resp.Header.Get("X-Amzn-Requestid"), cancel), nil
}

func (c *BedrockClient) Ping(ctx context.Context) error {
	if c.region == "" || c.accessKeyID == "" || len(c.secretAccessKey) == 0 {
		return fmt.Errorf("eyrie: bedrock credentials are incomplete")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://bedrock.%s.amazonaws.com/foundation-models", c.region), nil)
	if err != nil {
		return err
	}
	if err := c.sign(req, nil, time.Now().UTC()); err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("eyrie: bedrock ping failed: %w", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("eyrie: bedrock: invalid credentials")
	}
	return nil
}

func (c *BedrockClient) buildBody(messages []core.EyrieMessage, opts core.ChatOptions) ([]byte, error) {
	messages = core.SanitizeMessages(messages)
	msgs, system := buildAnthropicMessages(messages)
	if opts.System != "" {
		if system != "" {
			system = opts.System + "\n\n" + system
		} else {
			system = opts.System
		}
	}
	maxTokens := opts.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}
	base := anthropicRequest{
		Model:         opts.Model,
		MaxTokens:     maxTokens,
		Messages:      msgs,
		System:        system,
		Temperature:   opts.Temperature,
		TopP:          opts.TopP,
		TopK:          opts.TopK,
		StopSequences: opts.StopSequences,
		Tools:         ConvertToAnthropicTools(opts.Tools),
		ToolChoice:    resolveToolChoice(opts.ToolChoice),
		Thinking:      resolveThinking(opts),
		Metadata:      resolveMetadata(opts),
		ServiceTier:   opts.ServiceTier,
		OutputConfig:  resolveOutputConfig(opts),
	}
	raw, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	var body map[string]interface{}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	body["anthropic_version"] = "bedrock-2023-05-31"
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	if len(data) > maxBedrockRequestSize {
		return nil, fmt.Errorf("eyrie: request size %d bytes exceeds Bedrock limit of %d bytes", len(data), maxBedrockRequestSize)
	}
	return data, nil
}

func (c *BedrockClient) modelURL(model string) string {
	return fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com/model/%s/invoke", c.region, url.PathEscape(model))
}

// anthropicStreamChunk represents a chunk from Bedrock's streaming response.
type anthropicStreamChunk struct {
	Type         string `json:"type"`
	Index        int    `json:"index,omitempty"`
	ContentBlock *struct {
		Type  string          `json:"type"`
		ID    string          `json:"id,omitempty"`
		Name  string          `json:"name,omitempty"`
		Input json.RawMessage `json:"input,omitempty"`
	} `json:"content_block,omitempty"`
	Delta *struct {
		Type        string `json:"type"`
		Text        string `json:"text,omitempty"`
		PartialJSON string `json:"partial_json,omitempty"`
		StopReason  string `json:"stop_reason,omitempty"`
	} `json:"delta,omitempty"`
	Message *struct {
		Usage *core.EyrieUsage `json:"usage,omitempty"`
	} `json:"message,omitempty"`
}

// eventStreamReader parses Amazon EventStream binary frames from a reader.
// This is a minimal implementation for Bedrock's invoke-with-response-stream.
type eventStreamReader struct {
	r io.Reader
}

func newEventStreamReader(r io.Reader) *eventStreamReader {
	return &eventStreamReader{r: r}
}

type eventStreamFrame struct {
	Headers map[string]string
	Payload []byte
}

// ReadEvent reads one EventStream frame. Returns io.EOF when the stream ends.
func (es *eventStreamReader) ReadEvent() (*eventStreamFrame, error) {
	// Read prelude: total_length(4) + headers_length(4) + prelude_crc(4)
	var prelude [12]byte
	if _, err := io.ReadFull(es.r, prelude[:]); err != nil {
		return nil, err
	}
	totalLen := int(prelude[0])<<24 | int(prelude[1])<<16 | int(prelude[2])<<8 | int(prelude[3])
	headersLen := int(prelude[4])<<24 | int(prelude[5])<<16 | int(prelude[6])<<8 | int(prelude[7])
	if totalLen < 12 || totalLen > 10*1024*1024 {
		return nil, fmt.Errorf("eventstream: invalid total length %d", totalLen)
	}
	if headersLen > totalLen-12 {
		return nil, fmt.Errorf("eventstream: headers length %d exceeds total %d", headersLen, totalLen)
	}

	// Validate prelude CRC: bytes 8-11 should be CRC32 of bytes 0-7
	preludeCRCExpected := binary.BigEndian.Uint32(prelude[8:12])
	preludeCRCComputed := crc32.ChecksumIEEE(prelude[0:8])
	if preludeCRCExpected != preludeCRCComputed {
		return nil, fmt.Errorf("eventstream: prelude CRC mismatch (expected %08x, got %08x)", preludeCRCExpected, preludeCRCComputed)
	}

	// Read remaining bytes: headers + payload + message_crc(4)
	remaining := totalLen - 12
	buf := make([]byte, remaining)
	if _, err := io.ReadFull(es.r, buf); err != nil {
		return nil, err
	}

	// Validate message CRC: last 4 bytes should be CRC32 of (prelude + headers + payload)
	msgCRCExpected := binary.BigEndian.Uint32(buf[len(buf)-4:])
	crcInput := make([]byte, 0, totalLen-4)
	crcInput = append(crcInput, prelude[:8]...)
	crcInput = append(crcInput, buf[:len(buf)-4]...)
	msgCRCComputed := crc32.ChecksumIEEE(crcInput)
	if msgCRCExpected != msgCRCComputed {
		return nil, fmt.Errorf("eventstream: message CRC mismatch (expected %08x, got %08x)", msgCRCExpected, msgCRCComputed)
	}

	// Parse headers (skip for now, we only need the payload)
	payloadStart := headersLen
	if payloadStart > len(buf)-4 {
		return nil, fmt.Errorf("eventstream: invalid headers length")
	}
	payload := buf[payloadStart : len(buf)-4] // exclude trailing CRC

	// Decode headers for content-type
	headers := make(map[string]string)
	headerBuf := buf[:headersLen]
	for len(headerBuf) > 0 {
		if len(headerBuf) < 2 {
			break
		}
		nameLen := int(headerBuf[0])
		if len(headerBuf) < 1+nameLen+1 {
			break
		}
		name := string(headerBuf[1 : 1+nameLen])
		headerBuf = headerBuf[1+nameLen:]
		valueType := headerBuf[0]
		headerBuf = headerBuf[1:]
		switch valueType {
		case 7: // string
			if len(headerBuf) < 2 {
				break
			}
			strLen := int(headerBuf[0])<<8 | int(headerBuf[1])
			if len(headerBuf) < 2+strLen {
				break
			}
			headers[name] = string(headerBuf[2 : 2+strLen])
			headerBuf = headerBuf[2+strLen:]
		case 0: // bool true
			headers[name] = "true"
		case 1: // bool false
			headers[name] = "false"
		default:
			// Skip other types
			return nil, fmt.Errorf("eventstream: unsupported header value type %d", valueType)
		}
	}

	return &eventStreamFrame{Headers: headers, Payload: payload}, nil
}

func (c *BedrockClient) sign(req *http.Request, body []byte, now time.Time) error {
	if c.region == "" || c.accessKeyID == "" || len(c.secretAccessKey) == 0 {
		return fmt.Errorf("eyrie: bedrock credentials are incomplete")
	}
	service := "bedrock"
	if strings.HasPrefix(req.URL.Host, "bedrock-runtime.") {
		service = "bedrock-runtime"
	}
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	payloadHash := sha256Hex(body)
	req.Header.Set("Host", req.URL.Host)
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	if c.sessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", c.sessionToken)
	}
	canonicalHeaders, signedHeaders := canonicalAWSHeaders(req.Header)
	canonicalRequest := strings.Join([]string{
		req.Method,
		awsCanonicalURI(req.URL.EscapedPath()),
		req.URL.RawQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")
	scope := dateStamp + "/" + c.region + "/" + service + "/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
	signingKey := awsSigningKey(string(c.secretAccessKey), dateStamp, c.region, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))
	req.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s", c.accessKeyID, scope, signedHeaders, signature))
	return nil
}

func canonicalAWSHeaders(headers http.Header) (string, string) {
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, strings.ToLower(key))
	}
	sort.Strings(keys)
	var canonical strings.Builder
	for _, lower := range keys {
		values := headers.Values(lower)
		if len(values) == 0 {
			values = headers.Values(http.CanonicalHeaderKey(lower))
		}
		for i := range values {
			values[i] = strings.Join(strings.Fields(values[i]), " ")
		}
		canonical.WriteString(lower)
		canonical.WriteByte(':')
		canonical.WriteString(strings.Join(values, ","))
		canonical.WriteByte('\n')
	}
	return canonical.String(), strings.Join(keys, ";")
}

func awsCanonicalURI(path string) string {
	if path == "" {
		return "/"
	}
	return path
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func awsSigningKey(secret, dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), dateStamp)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	return hmacSHA256(kService, "aws4_request")
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(data))
	return mac.Sum(nil)
}

// ModelURL returns the signed model URL for Bedrock.
func (c *BedrockClient) ModelURL(model string) string {
	return c.modelURL(model)
}

// BuildBody creates the Anthropic-compatible Bedrock request payload.
func (c *BedrockClient) BuildBody(messages []core.EyrieMessage, opts core.ChatOptions) ([]byte, error) {
	return c.buildBody(messages, opts)
}

// HTTPClient returns the underlying HTTP client.
func (c *BedrockClient) HTTPClient() *http.Client { return c.httpClient }

// Retry returns the retry configuration.
func (c *BedrockClient) Retry() core.RetryConfig { return c.retry }

// Region returns the AWS region.
func (c *BedrockClient) Region() string { return c.region }

// AWSCanonicalURI returns the canonical URI used by SigV4.
func AWSCanonicalURI(uri string) string { return awsCanonicalURI(uri) }

// AWSSigningKey derives a SigV4 signing key from explicit inputs.
func AWSSigningKey(secret, dateStamp, region, service string) []byte {
	return awsSigningKey(secret, dateStamp, region, service)
}

// Sign applies AWS SigV4 headers to a Bedrock request.
func (c *BedrockClient) Sign(req *http.Request, body []byte, now time.Time) error {
	return c.sign(req, body, now)
}

// Sha256Hex returns a lowercase SHA-256 digest.
func Sha256Hex(data []byte) string { return sha256Hex(data) }

// CanonicalAWSHeaders canonicalizes a header set for SigV4.
func CanonicalAWSHeaders(headers http.Header) (string, string) {
	return canonicalAWSHeaders(headers)
}
