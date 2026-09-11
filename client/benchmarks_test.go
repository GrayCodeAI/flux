package client

import (
	"context"
	"testing"
)

// ---------- buildRequestBase ----------

func BenchmarkBuildRequestBase_SimpleMessages(b *testing.B) {
	messages := []EyrieMessage{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
		{Role: "user", Content: "How are you?"},
	}
	opts := ChatOptions{
		Model:       "gpt-4",
		Temperature: floatPtr(0.7),
		MaxTokens:   4096,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildRequestBase(messages, opts, false, nil)
	}
}

func BenchmarkBuildRequestBase_WithToolUse(b *testing.B) {
	messages := []EyrieMessage{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Search for files"},
		{Role: "assistant", ToolUse: []ToolCall{
			{ID: "tc-1", Name: "search", Arguments: map[string]interface{}{"query": "main.go"}},
			{ID: "tc-2", Name: "read", Arguments: map[string]interface{}{"path": "main.go"}},
		}},
		{Role: "user", ToolResults: []ToolResult{{ToolUseID: "tc-1", Content: "Found 1 file"}}},
		{Role: "user", ToolResults: []ToolResult{{ToolUseID: "tc-2", Content: "package main\nfunc main() {}"}}},
	}
	opts := ChatOptions{
		Model: "gpt-4",
		Tools: []EyrieTool{
			{Name: "search", Description: "Search for files", Parameters: map[string]interface{}{"query": map[string]string{"type": "string"}}},
			{Name: "read", Description: "Read a file", Parameters: map[string]interface{}{"path": map[string]string{"type": "string"}}},
		},
		MaxTokens: 4096,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildRequestBase(messages, opts, false, nil)
	}
}

func BenchmarkBuildRequestBase_WithImages(b *testing.B) {
	messages := []EyrieMessage{
		{Role: "user", Content: "What's in this image?", Images: []string{"data:image/png;base64,iVBORw0KGgo="}},
	}
	opts := ChatOptions{Model: "gpt-4-vision", MaxTokens: 4096}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildRequestBase(messages, opts, false, nil)
	}
}

func BenchmarkBuildRequestBase_Streaming(b *testing.B) {
	messages := []EyrieMessage{
		{Role: "user", Content: "Write a long essay"},
	}
	opts := ChatOptions{Model: "gpt-4", MaxTokens: 4096}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildRequestBase(messages, opts, true, nil)
	}
}

// ---------- buildCacheKey ----------

func BenchmarkBuildCacheKey_Short(b *testing.B) {
	messages := []EyrieMessage{
		{Role: "user", Content: "Hello"},
	}
	opts := ChatOptions{Model: "gpt-4"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildCacheKey(messages, opts)
	}
}

func BenchmarkBuildCacheKey_Long(b *testing.B) {
	longContent := make([]byte, 4000)
	for i := range longContent {
		longContent[i] = 'a'
	}
	messages := []EyrieMessage{
		{Role: "system", Content: string(longContent)},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: string(longContent)},
		{Role: "user", Content: "Continue"},
	}
	opts := ChatOptions{Model: "gpt-4", System: "You are helpful"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildCacheKey(messages, opts)
	}
}

func BenchmarkBuildCacheKey_WithToolCalls(b *testing.B) {
	messages := []EyrieMessage{
		{Role: "assistant", ToolUse: []ToolCall{
			{ID: "tc-1", Name: "search", Arguments: map[string]interface{}{"query": "test"}},
		}},
		{Role: "user", ToolResults: []ToolResult{{ToolUseID: "tc-1", Content: "result"}}},
	}
	opts := ChatOptions{Model: "gpt-4"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildCacheKey(messages, opts)
	}
}

// ---------- CachedProvider ----------

func BenchmarkCachedProvider_CacheHit(b *testing.B) {
	mock := NewMockProvider(MockModeFixed)
	mock.Response = "cached response"
	cp := NewCachedProvider(mock, DefaultCacheConfig())
	messages := []EyrieMessage{{Role: "user", Content: "Hello"}}
	opts := ChatOptions{Model: "gpt-4"}

	// Prime the cache
	_, _ = cp.Chat(context.Background(), messages, opts)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cp.Chat(context.Background(), messages, opts)
	}
}

func BenchmarkCachedProvider_CacheMiss(b *testing.B) {
	mock := NewMockProvider(MockModeFixed)
	mock.Response = "response"
	cp := NewCachedProvider(mock, DefaultCacheConfig())
	opts := ChatOptions{Model: "gpt-4"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		messages := []EyrieMessage{{Role: "user", Content: "unique query"}}
		_, _ = cp.Chat(context.Background(), messages, opts)
	}
}

// ---------- SanitizeMessages ----------

func BenchmarkSanitizeMessages_Clean(b *testing.B) {
	messages := []EyrieMessage{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
		{Role: "user", Content: "How are you?"},
		{Role: "assistant", Content: "I'm good."},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = SanitizeMessages(messages)
	}
}

func BenchmarkSanitizeMessages_WithOrphans(b *testing.B) {
	messages := []EyrieMessage{
		{Role: "user", Content: "Search for files"},
		{Role: "assistant", ToolUse: []ToolCall{
			{ID: "tc-1", Name: "search", Arguments: map[string]interface{}{"query": "test"}},
			{ID: "tc-2", Name: "read", Arguments: map[string]interface{}{"path": "main.go"}},
		}},
		// tc-1 has result, tc-2 is orphaned
		{Role: "user", ToolResults: []ToolResult{{ToolUseID: "tc-1", Content: "Found 1 file"}}},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = SanitizeMessages(messages)
	}
}

func BenchmarkSanitizeMessages_Large(b *testing.B) {
	messages := make([]EyrieMessage, 50)
	for i := range messages {
		if i%3 == 0 {
			messages[i] = EyrieMessage{Role: "user", Content: "message"}
		} else {
			messages[i] = EyrieMessage{Role: "assistant", Content: "response"}
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = SanitizeMessages(messages)
	}
}

// ---------- MergeConsecutiveRoles ----------

func BenchmarkMergeConsecutiveRoles_NoMerge(b *testing.B) {
	messages := []EyrieMessage{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi"},
		{Role: "user", Content: "How are you?"},
		{Role: "assistant", Content: "Good"},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = MergeConsecutiveRoles(messages)
	}
}

func BenchmarkMergeConsecutiveRoles_WithMerges(b *testing.B) {
	messages := []EyrieMessage{
		{Role: "user", Content: "Hello"},
		{Role: "user", Content: "World"},
		{Role: "assistant", Content: "Hi"},
		{Role: "assistant", Content: "There"},
		{Role: "user", Content: "How are you?"},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = MergeConsecutiveRoles(messages)
	}
}

func BenchmarkMergeConsecutiveRoles_WithToolUse(b *testing.B) {
	messages := []EyrieMessage{
		{Role: "assistant", ToolUse: []ToolCall{{ID: "tc-1", Name: "search"}}},
		{Role: "assistant", Content: "Let me search"},
		{Role: "user", ToolResults: []ToolResult{{ToolUseID: "tc-1", Content: "result"}}},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = MergeConsecutiveRoles(messages)
	}
}

// ---------- MetricsCollector ----------

func BenchmarkMetricsCollector_Record(b *testing.B) {
	mc := NewMetricsCollector()
	m := CallMetrics{Model: "gpt-4", Provider: "openai", InputTokens: 100, OutputTokens: 50, LatencyMs: 100}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mc.Record(m)
	}
}

func BenchmarkMetricsCollector_Recent(b *testing.B) {
	mc := NewMetricsCollector()
	m := CallMetrics{Model: "gpt-4", Provider: "openai", InputTokens: 100, OutputTokens: 50, LatencyMs: 100}
	for i := 0; i < 100; i++ {
		mc.Record(m)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mc.Recent(10)
	}
}

func BenchmarkMetricsCollector_TotalCost(b *testing.B) {
	mc := NewMetricsCollector()
	m := CallMetrics{Model: "gpt-4", Provider: "openai", InputTokens: 1000, OutputTokens: 500, CacheReadTokens: 200, CacheCreationTokens: 100}
	for i := 0; i < 100; i++ {
		mc.Record(m)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mc.TotalCost()
	}
}

// ---------- helpers ----------

func floatPtr(f float64) *float64 { return &f }
