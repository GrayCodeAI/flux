package core

import "testing"

// feedAll runs a sequence of deltas through a single splitter, concatenating
// the content and thinking outputs, as the stream loop does.
func feedAll(deltas []string) (content, thinking string) {
	var s thinkSplitter
	for _, d := range deltas {
		c, t := s.feed(d)
		content += c
		thinking += t
	}
	return content, thinking
}

func TestThinkSplitter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		deltas       []string
		wantContent  string
		wantThinking string
	}{
		{
			name:        "no think tags",
			deltas:      []string{"hello ", "world"},
			wantContent: "hello world",
		},
		{
			name:         "single chunk with think block",
			deltas:       []string{"<think>reasoning here</think>answer"},
			wantContent:  "answer",
			wantThinking: "reasoning here",
		},
		{
			name:         "think block then content, separate chunks",
			deltas:       []string{"<think>step 1 ", "step 2</think>", "the answer"},
			wantContent:  "the answer",
			wantThinking: "step 1 step 2",
		},
		{
			name:         "open tag split across chunks",
			deltas:       []string{"<th", "ink>secret</think>visible"},
			wantContent:  "visible",
			wantThinking: "secret",
		},
		{
			name:         "close tag split across chunks",
			deltas:       []string{"<think>secret</thi", "nk>visible"},
			wantContent:  "visible",
			wantThinking: "secret",
		},
		{
			name:         "content before and after think",
			deltas:       []string{"intro <think>mid</think> outro"},
			wantContent:  "intro  outro",
			wantThinking: "mid",
		},
		{
			name:        "lone less-than is not a tag",
			deltas:      []string{"a < b and c > d"},
			wantContent: "a < b and c > d",
		},
		{
			name:         "tag-like prefix that is not a think tag flushes",
			deltas:       []string{"<thinking-not-a-tag>"},
			wantContent:  "<thinking-not-a-tag>",
			wantThinking: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			content, thinking := feedAll(tc.deltas)
			if content != tc.wantContent {
				t.Errorf("content = %q, want %q", content, tc.wantContent)
			}
			if thinking != tc.wantThinking {
				t.Errorf("thinking = %q, want %q", thinking, tc.wantThinking)
			}
		})
	}
}

func TestPartialSuffixLen(t *testing.T) {
	t.Parallel()
	cases := []struct {
		s, tag string
		want   int
	}{
		{"abc<", "<think>", 1},
		{"abc<thi", "<think>", 4},
		{"<think>", "<think>", 0}, // a full match is not a partial suffix
		{"hello", "<think>", 0},
		{"foo</thi", "</think>", 5},
	}
	for _, c := range cases {
		if got := partialSuffixLen(c.s, c.tag); got != c.want {
			t.Errorf("partialSuffixLen(%q,%q) = %d, want %d", c.s, c.tag, got, c.want)
		}
	}
}
