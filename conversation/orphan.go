package conversation

import (
	"encoding/json"
	"strings"

	"github.com/GrayCodeAI/graycode-router/storage"
)

func extractToolResultIDsFromContent(content string) []string {
	trimmed := strings.TrimSpace(content)
	if len(trimmed) == 0 || trimmed[0] != '[' || !json.Valid([]byte(trimmed)) {
		return nil
	}
	var blocks []struct {
		Type      string `json:"type"`
		ToolUseID string `json:"tool_use_id"`
	}
	if json.Unmarshal([]byte(trimmed), &blocks) != nil {
		return nil
	}
	var ids []string
	for _, b := range blocks {
		if b.Type == "tool_result" && b.ToolUseID != "" {
			ids = append(ids, b.ToolUseID)
		}
	}
	return ids
}

func extractToolUseIDsFromContent(content string) []string {
	trimmed := strings.TrimSpace(content)
	if len(trimmed) == 0 || trimmed[0] != '[' || !json.Valid([]byte(trimmed)) {
		return nil
	}
	var blocks []struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}
	if json.Unmarshal([]byte(trimmed), &blocks) != nil {
		return nil
	}
	var ids []string
	for _, b := range blocks {
		if b.Type == "tool_use" && b.ID != "" {
			ids = append(ids, b.ID)
		}
	}
	return ids
}

// injectSyntheticToolResults inserts in-memory tool_result nodes after any
// assistant node that still has orphaned tool_use IDs on the ancestor path.
func injectSyntheticToolResults(ancestors []*storage.Node, orphans map[string][]string) []*storage.Node {
	if len(orphans) == 0 {
		return ancestors
	}
	var result []*storage.Node
	for _, node := range ancestors {
		result = append(result, node)
		toolIDs, ok := orphans[node.ID]
		if !ok || len(toolIDs) == 0 {
			continue
		}
		var blocks []map[string]any
		for _, id := range toolIDs {
			blocks = append(blocks, map[string]any{
				"type":        "tool_result",
				"tool_use_id": id,
				"content":     "Tool call was not completed.",
				"is_error":    true,
			})
		}
		content, _ := json.Marshal(blocks)
		result = append(result, &storage.Node{
			NodeType: storage.NodeTypeToolResult,
			Content:  string(content),
			Sequence: node.Sequence,
		})
	}
	return result
}
