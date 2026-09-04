package live

import "github.com/GrayCodeAI/graycode-router/catalog/xiaomi"

func enrichMimoEntry(e Entry, platform map[string]xiaomi.PlatformModel) Entry {
	display, desc, ctx, maxOut, in, out, meta := xiaomi.ApplyPlatformMetadata(
		e.ID, e.DisplayName, e.Description, e.ContextWindow, e.MaxOutput,
		e.InputPricePer1M, e.OutputPricePer1M, e.RawJSON, platform,
	)
	e.DisplayName = display
	e.Description = desc
	e.ContextWindow = ctx
	e.MaxOutput = maxOut
	e.InputPricePer1M = in
	e.OutputPricePer1M = out
	e.RawJSON = meta
	return e
}
