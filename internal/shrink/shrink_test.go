package shrink_test

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-router/internal/shrink"
	"github.com/GrayCodeAI/graycode-router/types"
)

func TestShrinkDescription_Basic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			"drops articles and filler",
			"This tool basically allows you to just read a file.",
			"This tool allows to read file.",
		},
		{
			"dictionary substitution",
			"In order to install the package, run this command.",
			"to install package, run this command.",
		},
		{
			"redundant prefix removed",
			"You can use this tool to list all files in the directory.",
			"list all files in directory.",
		},
		{
			"no change for short descriptions",
			"List files",
			"List files",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, _ := shrink.ShrinkDescription(tc.in)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestShrinkDescription_SafetyPassThrough(t *testing.T) {
	t.Parallel()
	dangerous := []string{
		"Run rm -rf to clean up",
		"Use sudo apt update",
		"Resets --hard the database",
		"Contains a private key",
		"Production deployment script",
	}
	for _, in := range dangerous {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			got, shrunk := shrink.ShrinkDescription(in)
			if shrunk {
				t.Errorf("expected pass-through for %q, got shrunk: %q", in, got)
			}
			if got != in {
				t.Errorf("expected unchanged %q, got %q", in, got)
			}
		})
	}
}

func TestShrinkDescription_Empty(t *testing.T) {
	t.Parallel()
	got, shrunk := shrink.ShrinkDescription("")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
	if shrunk {
		t.Error("expected shrunk=false for empty")
	}
}

func TestShrinkDescription_MaxLength(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("word ", 100) // ~500 chars
	got, _ := shrink.ShrinkDescription(long)
	if len(got) > shrink.MaxLen+1 { // +1 for the ellipsis
		t.Errorf("expected <= %d chars, got %d", shrink.MaxLen+1, len(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected ellipsis suffix, got %q", got)
	}
}

func TestShrinkDescription_CaseInsensitive(t *testing.T) {
	t.Parallel()
	got, _ := shrink.ShrinkDescription("IN ORDER TO install")
	if !strings.Contains(got, "to") {
		t.Errorf("expected case-insensitive replace, got %q", got)
	}
}

func TestShrinkTools_Empty(t *testing.T) {
	t.Parallel()
	tools, r := shrink.ShrinkTools(nil)
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
	if r.OriginalBytes != 0 {
		t.Errorf("expected 0 bytes, got %d", r.OriginalBytes)
	}
}

func TestShrinkTools_AllSafe(t *testing.T) {
	t.Parallel()
	tools := []types.Tool{
		{Name: "read", Description: "Read the contents of a file from disk."},
		{Name: "write", Description: "In order to write content to a file, use this tool."},
		{Name: "list", Description: "List all files in the current directory."},
	}
	// Snapshot the input descriptions so we can prove ShrinkTools does not
	// mutate the caller's slice (it must operate on copies).
	origDescriptions := make([]string, len(tools))
	for i, t1 := range tools {
		origDescriptions[i] = t1.Description
	}
	out, r := shrink.ShrinkTools(tools)
	if r.ToolsProcessed != 3 {
		t.Errorf("expected 3 processed, got %d", r.ToolsProcessed)
	}
	if r.ToolsSkipped != 0 {
		t.Errorf("expected 0 skipped, got %d", r.ToolsSkipped)
	}
	if r.BytesSaved <= 0 {
		t.Error("expected positive bytes saved")
	}
	if r.PercentOff <= 0 {
		t.Error("expected positive percent off")
	}
	// Originals must not be modified (shrink operates on copies).
	for i, t1 := range tools {
		if t1.Description != origDescriptions[i] {
			t.Errorf("tool %d: original description mutated: %q -> %q", i, origDescriptions[i], t1.Description)
		}
	}
	// The shrunk output should differ from the original where applicable.
	if out[0].Description == origDescriptions[0] {
		t.Errorf("tool 0: expected description to be shrunk, got unchanged %q", out[0].Description)
	}
}

func TestShrinkTools_MixedSafety(t *testing.T) {
	t.Parallel()
	tools := []types.Tool{
		{Name: "read", Description: "Read a file"},
		{Name: "dangerous", Description: "rm -rf the filesystem"},
		{Name: "write", Description: "Write a file"},
	}
	_, r := shrink.ShrinkTools(tools)
	if r.ToolsProcessed != 2 {
		t.Errorf("expected 2 processed, got %d", r.ToolsProcessed)
	}
	if r.ToolsSkipped != 1 {
		t.Errorf("expected 1 skipped, got %d", r.ToolsSkipped)
	}
}

func TestShrinkToolsIf_Disabled(t *testing.T) {
	t.Parallel()
	tools := []types.Tool{
		{Name: "read", Description: "Read a file"},
		{Name: "write", Description: "Write a file"},
	}
	out, r := shrink.ShrinkToolsIf(tools, false)
	if r.BytesSaved != 0 {
		t.Errorf("expected 0 bytes saved when disabled, got %d", r.BytesSaved)
	}
	for i, t1 := range tools {
		if t1.Description != out[i].Description {
			t.Errorf("tool %d description changed when disabled", i)
		}
	}
}

func TestShrinkToolsIf_Enabled(t *testing.T) {
	t.Parallel()
	tools := []types.Tool{
		{Name: "read", Description: "Read a file"},
	}
	_, r := shrink.ShrinkToolsIf(tools, true)
	if r.ToolsProcessed != 1 {
		t.Errorf("expected 1 processed, got %d", r.ToolsProcessed)
	}
}

func TestShrinkTools_OriginalsUnchanged(t *testing.T) {
	t.Parallel()
	orig := "In order to read a file, just use this tool."
	tools := []types.Tool{{Name: "read", Description: orig}}
	_, _ = shrink.ShrinkTools(tools)
	if tools[0].Description != orig {
		t.Errorf("original was modified: %q != %q", tools[0].Description, orig)
	}
}
