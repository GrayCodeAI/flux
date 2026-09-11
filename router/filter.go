package router

import "github.com/GrayCodeAI/eyrie/client"

type ToolFilter struct {
	modelTools map[string][]string
}

func NewToolFilter(modelTools map[string][]string) *ToolFilter {
	return &ToolFilter{modelTools: modelTools}
}

func (f *ToolFilter) FilterTools(model string, tools []client.EyrieTool) []client.EyrieTool {
	if f == nil || len(f.modelTools) == 0 {
		return tools
	}
	supported, ok := f.modelTools[model]
	if !ok {
		return tools
	}
	supportedSet := make(map[string]bool, len(supported))
	for _, t := range supported {
		supportedSet[t] = true
	}
	var filtered []client.EyrieTool
	for _, t := range tools {
		if len(t.Parameters) > 0 {
			filtered = append(filtered, t)
			continue
		}
		if supportedSet[t.Name] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}
