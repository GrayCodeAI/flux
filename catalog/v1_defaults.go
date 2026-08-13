package catalog

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// defaultProviders registers all known API providers with their display names.
func defaultProviders() map[string]Provider {
	return map[string]Provider{
		"anthropic":              {ID: "anthropic", Name: "Anthropic"},
		"openai":                 {ID: "openai", Name: "OpenAI"},
		"google":                 {ID: "google", Name: "Google"},
		"xai":                    {ID: "xai", Name: "xAI"},
		"openrouter":             {ID: "openrouter", Name: "OpenRouter"},
		"concentrate":            {ID: "concentrate", Name: "Concentrate AI (Pay-as-you-go)"},
		"opengateway":            {ID: "opengateway", Name: "OpenGateway (Pay-as-you-go)"},
		"canopywave":             {ID: "canopywave", Name: "CanopyWave"},
		"zai_payg":               {ID: "zai_payg", Name: "Z.AI Pay-as-you-go"},
		"zai_coding":             {ID: "zai_coding", Name: "Z.AI Coding Plan"},
		"ollama":                 {ID: "ollama", Name: "Ollama"},
		"opencodego":             {ID: "opencodego", Name: "OpenCode Go"},
		"moonshotai":             {ID: "moonshotai", Name: "Moonshot AI"},
		"kimi":                   {ID: "kimi", Name: "Kimi (Moonshot)"},
		"xiaomi_mimo_payg":       {ID: "xiaomi_mimo_payg", Name: "Xiaomi MiMo (Pay-as-you-go)"},
		"xiaomi_mimo_token_plan": {ID: "xiaomi_mimo_token_plan", Name: "Xiaomi MiMo (Token Plan)"},
		"deepseek":               {ID: "deepseek", Name: "DeepSeek"},
		"stepfun":                {ID: "stepfun", Name: "StepFun"},
		"fireworks":              {ID: "fireworks", Name: "Fireworks AI"},
	}
}

// defaultProtocols registers the known API protocol types.
func defaultProtocols() map[string]Protocol {
	return map[string]Protocol{
		"openai-chat-completions": {ID: "openai-chat-completions", Name: "OpenAI Chat Completions"},
		"openai-responses":        {ID: "openai-responses", Name: "OpenAI Responses"},
	}
}

// defaultDeployments registers the known API deployment configurations.
func defaultDeployments() map[string]Deployment {
	return map[string]Deployment{
		"anthropic-direct":              deployment("anthropic-direct", "Anthropic", "anthropic", "openai-chat-completions", "anthropic", NativeModelIDCatalogKnown),
		"anthropic-bedrock":             deployment("anthropic-bedrock", "Anthropic on Bedrock", "anthropic", "openai-chat-completions", "anthropic-bedrock", NativeModelIDCatalogKnown),
		"anthropic-vertex":              deployment("anthropic-vertex", "Anthropic on Vertex", "anthropic", "openai-chat-completions", "anthropic-vertex", NativeModelIDCatalogKnown),
		"openai-direct":                 deployment("openai-direct", "OpenAI", "openai", "openai-chat-completions", "openai", NativeModelIDCatalogKnown),
		"openai-azure":                  azureDeployment(),
		"gemini-direct":                 deployment("gemini-direct", "Gemini", "google", "openai-chat-completions", "gemini", NativeModelIDCatalogKnown),
		"gemini-vertex":                 deployment("gemini-vertex", "Gemini on Vertex", "google", "openai-chat-completions", "gemini-vertex", NativeModelIDCatalogKnown),
		"grok-direct":                   deployment("grok-direct", "Grok", "xai", "openai-chat-completions", "grok", NativeModelIDCatalogKnown),
		"openrouter":                    deployment("openrouter", "OpenRouter", "openrouter", "openai-chat-completions", "openrouter", NativeModelIDDiscovered),
		"concentrate-payg":              deployment("concentrate-payg", "Concentrate AI (Pay-as-you-go)", "concentrate", "openai-responses", "concentrate-responses", NativeModelIDDiscovered),
		"opengateway-payg":              deployment("opengateway-payg", "OpenGateway (Pay-as-you-go)", "opengateway", "openai-chat-completions", "openai", NativeModelIDDiscovered),
		"zai_payg-direct":               deployment("zai_payg-direct", "Z.AI Pay-as-you-go", "zai_payg", "openai-chat-completions", "zai_payg", NativeModelIDCatalogKnown),
		"zai_coding-direct":             deployment("zai_coding-direct", "Z.AI Coding Plan", "zai_coding", "openai-chat-completions", "zai_coding", NativeModelIDCatalogKnown),
		"canopywave":                    deployment("canopywave", "CanopyWave", "canopywave", "openai-chat-completions", "canopywave", NativeModelIDDiscovered),
		"ollama-local":                  localDeployment(),
		"opencodego":                    deployment("opencodego", "OpenCode Go", "opencodego", "openai-chat-completions", "opencodego", NativeModelIDDiscovered),
		"kimi-direct":                   deployment("kimi-direct", "Kimi (Moonshot)", "kimi", "openai-chat-completions", "kimi", NativeModelIDDiscovered),
		"xiaomi_mimo_payg-direct":       deployment("xiaomi_mimo_payg-direct", "Xiaomi MiMo Pay-as-you-go", "xiaomi_mimo_payg", "openai-chat-completions", "xiaomi_mimo", NativeModelIDDiscovered),
		"xiaomi_mimo_token_plan-direct": deployment("xiaomi_mimo_token_plan-direct", "Xiaomi MiMo Token Plan", "xiaomi_mimo_token_plan", "openai-chat-completions", "xiaomi_mimo", NativeModelIDDiscovered),
		"deepseek-direct":               deployment("deepseek-direct", "DeepSeek", "deepseek", "openai-chat-completions", "deepseek", NativeModelIDCatalogKnown),
		"stepfun-direct":                deployment("stepfun-direct", "StepFun", "stepfun", "openai-chat-completions", "openai", NativeModelIDDiscovered),
		"fireworks-direct":              deployment("fireworks-direct", "Fireworks AI", "fireworks", "openai-chat-completions", "openai", NativeModelIDDiscovered),
	}
}

func deployment(id, name, providerID, protocolID, adapter string, source NativeModelIDSource) Deployment {
	return Deployment{ID: id, Name: name, ProviderID: providerID, APIProtocolID: protocolID, AdapterConstructor: adapter, NativeModelIDSource: source}
}

func azureDeployment() Deployment {
	d := deployment("openai-azure", "Azure OpenAI", "openai", "openai-chat-completions", "openai-azure", NativeModelIDUserConfigured)
	d.ModelMappingsRequired = true
	return d
}

func localDeployment() Deployment {
	d := deployment("ollama-local", "Ollama local", "ollama", "openai-chat-completions", "ollama", NativeModelIDDiscovered)
	d.Local = true
	return d
}

// CanonicalProviderID normalizes legacy provider aliases (e.g. gemini -> google, cline-pass -> clinepass).
func CanonicalProviderID(providerID string) string {
	return canonicalProviderID(providerID)
}

func canonicalProviderID(providerID string) string {
	switch providerID {
	case "gemini":
		return "google"
	case "grok":
		return "xai"
	case "moonshotai":
		return "moonshotai"
	case "cline-pass":
		return "clinepass"
	case "xiaomi-mimo", "xiaomi_mimo", "xiaomi-mimo-payg":
		return "xiaomi_mimo_payg"
	case "xiaomi-mimo-token-plan":
		return "xiaomi_mimo_token_plan"
	default:
		return providerID
	}
}

func hasSlash(s string) bool {
	_, _, ok := strings.Cut(s, "/")
	return ok
}

func splitOwner(s string) (owner, rest string, ok bool) {
	owner, rest, ok = strings.Cut(s, "/")
	return
}

func hasInputPricing(raw json.RawMessage) bool {
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	_, ok := m["input_token_price_per_m"]
	return ok
}

// DefaultOfferingTemplates returns offering templates for Azure deployments (model mappings required).
func DefaultOfferingTemplates(generatedAt time.Time) []ModelOfferingTemplate {
	var out []ModelOfferingTemplate
	for _, model := range seedOpenAIModels {
		modelID := "openai/" + model.ID
		out = append(out, ModelOfferingTemplate{
			ID:                  "openai-azure:" + modelID,
			CanonicalModelID:    modelID,
			DeploymentID:        "openai-azure",
			NativeModelIDSource: NativeModelIDUserConfigured,
			MappingRequired:     true,
			Capabilities:        capabilitySetFromLegacy(model),
			Pricing:             pricingFromLegacy(model, generatedAt),
		})
	}
	return out
}

// SanitizePricing drops invalid rate dimensions (e.g. negative prices).
func SanitizePricing(c *Catalog) {
	if c == nil {
		return
	}
	for i := range c.Offerings {
		c.Offerings[i].Pricing = sanitizePricing(c.Offerings[i].Pricing)
	}
	for i := range c.OfferingTemplates {
		c.OfferingTemplates[i].Pricing = sanitizePricing(c.OfferingTemplates[i].Pricing)
	}
}

func sanitizePricing(p Pricing) Pricing {
	if len(p.RatesPer1M) == 0 {
		return p
	}
	clean := make(map[string]float64, len(p.RatesPer1M))
	for dim, rate := range p.RatesPer1M {
		if dim == "" || rate < 0 {
			continue
		}
		clean[dim] = rate
	}
	if len(clean) == 0 {
		p.Status = PricingUnknown
		p.RatesPer1M = nil
		return p
	}
	p.RatesPer1M = clean
	if p.Status == PricingKnown && (p.Currency == "" || len(p.RatesPer1M) == 0) {
		p.Status = PricingUnknown
		p.RatesPer1M = nil
	}
	return p
}

func capabilitySetFromLegacy(entry ModelCatalogEntry) CapabilitySet {
	set := CapabilitySet{
		ServerTools:     map[string]CapabilityState{},
		MaxInputTokens:  entry.ContextWindow,
		MaxOutputTokens: entry.MaxOutput,
	}
	for _, tool := range entry.ServerTools {
		if tool != "" {
			set.ServerTools[tool] = CapabilitySupported
		}
	}
	if len(set.ServerTools) == 0 {
		set.ServerTools = nil
	}
	for _, feat := range entry.ServerTools {
		switch strings.ToLower(strings.TrimSpace(feat)) {
		case "function-calling", "tools":
			set.FunctionCalling = CapabilitySupported
		case "thinking:enabled":
			set.ExplicitThinkingBudget = CapabilitySupported
			set.ThinkingTypes = append(set.ThinkingTypes, "enabled")
		case "thinking:adaptive":
			set.AdaptiveThinking = CapabilitySupported
			set.ThinkingTypes = append(set.ThinkingTypes, "adaptive")
		case "effort":
			set.Effort = CapabilitySupported
		case "structured_output":
			set.StructuredOutput = CapabilitySupported
		case "code_execution":
			set.CodeExecution = CapabilitySupported
		case "citations":
			set.Citations = CapabilitySupported
		case "pdf_input":
			set.PDFInput = CapabilitySupported
		case "image_input":
			set.ImageInput = CapabilitySupported
		}
	}
	// Parse effort levels from features (format: "effort:low,medium,high")
	for _, feat := range entry.ServerTools {
		if strings.HasPrefix(strings.ToLower(feat), "effort:") {
			levels := strings.TrimPrefix(strings.ToLower(feat), "effort:")
			set.EffortLevels = strings.Split(levels, ",")
		}
	}
	return set
}

func pricingFromLegacy(entry ModelCatalogEntry, effectiveAt time.Time) Pricing {
	in := entry.InputPricePer1M
	out := entry.OutputPricePer1M
	if in < 0 || out < 0 {
		return Pricing{Status: PricingUnknown, Currency: "USD", EffectiveAt: effectiveAt}
	}
	pricing := Pricing{
		Status:      PricingKnown,
		Currency:    "USD",
		EffectiveAt: effectiveAt,
		RatesPer1M:  map[string]float64{"input_tokens": in, "output_tokens": out},
	}
	if in == 0 && out == 0 {
		pricing.Status = PricingUnknown
		pricing.RatesPer1M = nil
		if strings.Contains(entry.ID, ":free") {
			pricing.Status = PricingFree
			pricing.RatesPer1M = map[string]float64{"input_tokens": 0, "output_tokens": 0}
		}
	}
	return pricing
}

func validNativeModelIDSource(source NativeModelIDSource) bool {
	switch source {
	case NativeModelIDCatalogKnown, NativeModelIDDiscovered, NativeModelIDUserConfigured, NativeModelIDCatalogOrUser:
		return true
	default:
		return false
	}
}

func looksCanonicalModelID(value string) bool {
	owner, model, ok := strings.Cut(value, "/")
	return ok && owner != "" && model != "" && !strings.ContainsAny(value, " \t\r\n")
}

func validatePricing(problems *[]string, id string, pricing Pricing) {
	switch pricing.Status {
	case PricingKnown, PricingPartial:
		if pricing.Currency == "" || len(pricing.RatesPer1M) == 0 {
			*problems = append(*problems, fmt.Sprintf("%s pricing is missing currency or rates", id))
		}
	case PricingUnknown:
		if len(pricing.RatesPer1M) > 0 {
			*problems = append(*problems, fmt.Sprintf("%s unknown pricing must not include rates", id))
		}
	case PricingFree:
		if pricing.Currency == "" {
			*problems = append(*problems, fmt.Sprintf("%s free pricing missing currency", id))
		}
	default:
		*problems = append(*problems, fmt.Sprintf("%s invalid pricing status %q", id, pricing.Status))
	}
	for dim, rate := range pricing.RatesPer1M {
		if dim == "" || rate < 0 {
			*problems = append(*problems, fmt.Sprintf("%s invalid pricing dimension %q", id, dim))
		}
	}
}

func validateCapabilities(problems *[]string, id string, capabilities CapabilitySet) {
	valid := func(state CapabilityState) bool {
		return state == "" || state == CapabilitySupported || state == CapabilityUnsupported || state == CapabilityUnknown
	}
	if !valid(capabilities.FunctionCalling) {
		*problems = append(*problems, fmt.Sprintf("%s invalid function_calling capability", id))
	}
	if !valid(capabilities.ExplicitThinkingBudget) {
		*problems = append(*problems, fmt.Sprintf("%s invalid explicit_thinking_budget capability", id))
	}
	for tool, state := range capabilities.ServerTools {
		if tool == "" || !valid(state) {
			*problems = append(*problems, fmt.Sprintf("%s invalid server tool capability", id))
		}
	}
}

// uniqueNonEmpty returns unique non-empty values from the input slice.
func uniqueNonEmpty(values ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func cloneMap[T any](in map[string]T) map[string]T {
	out := make(map[string]T, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
