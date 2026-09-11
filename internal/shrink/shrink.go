// Package shrink compresses LLM tool descriptions before they are
// sent to the provider. It implements the tool-description shrink pattern
// GrayCode tool-description shrink pipeline:
//
//  1. For each tool description, apply the shrink rules:
//     - drop articles ("a", "an", "the")
//     - drop filler words ("just", "really", "basically")
//     - drop pleasantries
//     - dictionary substitutions ("in order to" -> "to")
//  2. Skip descriptions that mention security/destructive keywords
//     (auto-clarity: preserve verbatim).
//  3. Cap the result to a max length (default 200 chars).
//  4. Report aggregate bytes/tokens saved across all tools.
//
// The shrink is applied to descriptions only; tool names and
// schemas are passed through unchanged so the LLM can still
// invoke the tool correctly.
//
// This is a low-level utility. Callers (typically the eyrie
// client) call ShrinkTools() on a []*types.Tool before passing
// them to the provider. Original tools are not modified.
package shrink

import (
	"sort"
	"strings"

	"github.com/GrayCodeAI/eyrie/types"
)

// Result reports per-call savings.
type Result struct {
	OriginalBytes   int
	CompressedBytes int
	BytesSaved      int
	PercentOff      float64
	ToolsProcessed  int
	ToolsSkipped    int // skipped due to security/destructive keywords
}

// MaxLen is the default cap on a single description length.
const MaxLen = 200

// safetyKeywords are substrings that, if present in a tool
// description, force pass-through (no shrinking). Matched
// case-insensitive. Standard auto-clarity safety list;
// smaller and tuned for LLM tool descriptions.
var safetyKeywords = []string{
	"rm -rf", "sudo ", "su -", "doas ",
	"chmod 777", "chown ",
	"format c:", "format /", "mkfs.",
	"dd if=",
	"shutdown", "reboot", "halt", "poweroff",
	"private key", "secret key", "api key", "password",
	"passphrase", "credential", "auth token",
	"force push", "force-push", "--force",
	"reset --hard", "drop database", "production",
	"--hard",
	"deploy",
	"destructive",
	"delet",
}

// shrinkDictionary is a small set of phrase substitutions tuned
// for tool descriptions. Kept small (vs the full substitution dictionary)
// because tool descriptions are short and over-eager substitution
// can break meaning.
var shrinkDictionary = []struct {
	from string
	to   string
}{
	{"in order to", "to"},
	{"due to the fact that", "because"},
	{"make use of", "use"},
	{"as well as", "and"},
	{"a number of", "many"},
	{"a variety of", "various"},
	{"in the event that", "if"},
	{"at this point in time", "now"},
	{"for the purpose of", "for"},
	{"with regard to", "about"},
	{"in spite of the fact that", "although"},
	{"in conjunction with", "with"},
	{"it is important to note that", "note:"},
	{"please be aware that", "note:"},
	{"you can use this tool to", ""},
	{"this tool allows you to", ""},
	{"use this tool to", ""},
}

// shrinkDrops is a small drop-list for tool descriptions.
var shrinkDrops = []string{
	"just", "really", "very", "quite", "rather", "basically",
	"literally", "actually", "simply", "totally", "completely",
	"absolutely", "definitely", "obviously", "clearly", "essentially",
	"particularly", "specifically", "generally", "usually",
	"of course", "sure", "certainly", "happy to",
	"a", "an", "the", // articles
	"you", // pronoun in second-person
}

func init() {
	// Sort dictionary once at startup (longest first) for greedy matching.
	sort.SliceStable(shrinkDictionary, func(i, j int) bool {
		return len(shrinkDictionary[i].from) > len(shrinkDictionary[j].from)
	})
}

// ShrinkDescription returns the shrunk version of desc. Returns
// (desc, false) if desc contains safety keywords (caller should
// keep the original verbatim).
func ShrinkDescription(desc string) (string, bool) {
	if desc == "" {
		return "", false
	}
	// Safety check
	lower := strings.ToLower(desc)
	for _, kw := range safetyKeywords {
		if strings.Contains(lower, kw) {
			return desc, false
		}
	}
	out := desc
	// Dictionary pass (already sorted longest-first at init time)
	for _, r := range shrinkDictionary {
		out = replaceCI(out, r.from, r.to)
	}
	// Drop pass
	for _, w := range shrinkDrops {
		out = removeWordCI(out, w)
	}
	// Cap length. "…" is 3 bytes in UTF-8, so account for it.
	if len(out) > MaxLen {
		out = out[:MaxLen-3] + "…"
	}
	// Collapse whitespace
	out = collapseWS(out)
	return out, true
}

// ShrinkTools returns a new slice with each tool's description
// shrunk. Original tools are not modified. The Result reports
// aggregate savings.
func ShrinkTools(tools []types.Tool) ([]types.Tool, Result) {
	if len(tools) == 0 {
		return tools, Result{}
	}
	out := make([]types.Tool, len(tools))
	var r Result
	for i, t := range tools {
		original := t.Description
		shrunk, shrunk_ok := ShrinkDescription(original)
		r.OriginalBytes += len(original)
		if !shrunk_ok {
			r.ToolsSkipped++
			r.CompressedBytes += len(original)
			out[i] = t
			continue
		}
		r.ToolsProcessed++
		r.CompressedBytes += len(shrunk)
		t.Description = shrunk
		out[i] = t
	}
	r.BytesSaved = r.OriginalBytes - r.CompressedBytes
	if r.OriginalBytes > 0 {
		r.PercentOff = float64(r.BytesSaved) / float64(r.OriginalBytes) * 100
	}
	return out, r
}

// ShrinkToolsIf returns ShrinkTools result only if enabled is true.
// Convenience for callers that have a config flag.
func ShrinkToolsIf(tools []types.Tool, enabled bool) ([]types.Tool, Result) {
	if !enabled {
		var r Result
		for _, t := range tools {
			r.OriginalBytes += len(t.Description)
		}
		r.CompressedBytes = r.OriginalBytes
		return tools, r
	}
	return ShrinkTools(tools)
}

// replaceCI replaces all case-insensitive occurrences of from in s
// with to. Returns s unchanged if from is empty.
func replaceCI(s, from, to string) string {
	if from == "" || from == to {
		return s
	}
	flen := len(from)
	var out []byte
	pos := 0
	for pos < len(s) {
		if pos+flen <= len(s) && matchCI(s[pos:pos+flen], from) {
			out = append(out, to...)
			pos += flen
			continue
		}
		out = append(out, s[pos])
		pos++
	}
	return string(out)
}

// removeWordCI removes all case-insensitive occurrences of word
// in s as a whole word (with word-boundary checks). Returns the
// result. Uses position tracking (not string mutation) to avoid
// quadratic behavior on long inputs.
func removeWordCI(s, word string) string {
	if word == "" {
		return s
	}
	wlen := len(word)
	if wlen == 0 {
		return s
	}
	var out []byte
	pos := 0
	for pos < len(s) {
		// Try to match word at pos
		if pos+wlen <= len(s) && matchCI(s[pos:pos+wlen], word) {
			leftBoundary := pos == 0 || !isWordByte(s[pos-1])
			rightBoundary := pos+wlen == len(s) || !isWordByte(s[pos+wlen])
			if leftBoundary && rightBoundary {
				// Skip the word and one trailing space.
				pos += wlen
				if pos < len(s) && s[pos] == ' ' {
					pos++
				}
				continue
			}
		}
		// No match (or failed word boundary) — emit s[pos] and advance
		out = append(out, s[pos])
		pos++
	}
	return string(out)
}

// matchCI reports whether a and b are equal, case-insensitive
// (ASCII only). Pre-condition: len(a) == len(b).
func matchCI(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		x, y := a[i], b[i]
		if x >= 'A' && x <= 'Z' {
			x += 'a' - 'A'
		}
		if y >= 'A' && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}

// isWordByte reports whether b is part of a word (letter or digit).
func isWordByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') || b == '_'
}

// collapseWS collapses runs of spaces to a single space.
func collapseWS(s string) string {
	var out []byte
	prevSpace := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' {
			if !prevSpace {
				out = append(out, ' ')
			}
			prevSpace = true
			continue
		}
		prevSpace = false
		out = append(out, c)
	}
	// Trim
	return strings.TrimSpace(string(out))
}
