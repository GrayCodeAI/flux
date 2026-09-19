package observability

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
)

// UsageEntry represents a single recorded usage event.
type UsageEntry struct {
	Tokens    int
	CostUSD   float64
	Timestamp time.Time
	Provider  string
	Model     string
}

// Alert represents a usage threshold alert.
type Alert struct {
	Level     string
	Message   string
	Timestamp time.Time
	Threshold float64
}

// UsageSummary provides a snapshot of current usage across all windows.
type UsageSummary struct {
	HourlyTokens     int
	HourlyRemaining  int
	DailyTokens      int
	DailyRemaining   int
	SessionTokens    int
	SessionRemaining int
	DailyCostUSD     float64
	CostRemaining    float64
	HourlyPct        float64
	DailyPct         float64
}

// UsageTracker tracks API usage across sessions and prevents surprise bills.
type UsageTracker struct {
	DailyLimit   int
	HourlyLimit  int
	SessionLimit int
	CostLimitUSD float64

	hourlyUsage  []UsageEntry
	dailyUsage   []UsageEntry
	sessionUsage int
	mu           sync.Mutex
	Alerts       []Alert

	firedThresholds map[string]bool
}

// NewUsageTracker creates a UsageTracker with sensible defaults.
func NewUsageTracker() *UsageTracker {
	return &UsageTracker{
		DailyLimit:      1_000_000,
		HourlyLimit:     200_000,
		SessionLimit:    500_000,
		CostLimitUSD:    10.00,
		hourlyUsage:     make([]UsageEntry, 0),
		dailyUsage:      make([]UsageEntry, 0),
		firedThresholds: make(map[string]bool),
	}
}

func (u *UsageTracker) Record(tokens int, costUSD float64, provider, model string) {
	u.mu.Lock()
	defer u.mu.Unlock()

	entry := UsageEntry{
		Tokens:    tokens,
		CostUSD:   costUSD,
		Timestamp: time.Now(),
		Provider:  provider,
		Model:     model,
	}

	u.hourlyUsage = append(u.hourlyUsage, entry)
	u.dailyUsage = append(u.dailyUsage, entry)
	u.sessionUsage += tokens
	u.checkThresholdsLocked()
}

func (u *UsageTracker) CanProceed() (bool, string) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.pruneOldLocked()

	hourlyTokens := u.hourlyTokensLocked()
	if hourlyTokens >= u.HourlyLimit {
		return false, fmt.Sprintf("hourly token limit reached (%d/%d)", hourlyTokens, u.HourlyLimit)
	}

	dailyTokens := u.dailyTokensLocked()
	if dailyTokens >= u.DailyLimit {
		return false, fmt.Sprintf("daily token limit reached (%d/%d)", dailyTokens, u.DailyLimit)
	}

	if u.sessionUsage >= u.SessionLimit {
		return false, fmt.Sprintf("session token limit reached (%d/%d)", u.sessionUsage, u.SessionLimit)
	}

	dailyCost := u.dailyCostLocked()
	if dailyCost >= u.CostLimitUSD {
		return false, fmt.Sprintf("daily cost limit reached ($%.2f/$%.2f)", dailyCost, u.CostLimitUSD)
	}

	return true, ""
}

func (u *UsageTracker) GetUsage() UsageSummary {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.pruneOldLocked()

	hourlyTokens := u.hourlyTokensLocked()
	dailyTokens := u.dailyTokensLocked()
	dailyCost := u.dailyCostLocked()

	hourlyRemaining := max(0, u.HourlyLimit-hourlyTokens)
	dailyRemaining := max(0, u.DailyLimit-dailyTokens)
	sessionRemaining := max(0, u.SessionLimit-u.sessionUsage)
	costRemaining := u.CostLimitUSD - dailyCost
	if costRemaining < 0 {
		costRemaining = 0
	}

	var hourlyPct, dailyPct float64
	if u.HourlyLimit > 0 {
		hourlyPct = float64(hourlyTokens) / float64(u.HourlyLimit) * 100
	}
	if u.DailyLimit > 0 {
		dailyPct = float64(dailyTokens) / float64(u.DailyLimit) * 100
	}

	return UsageSummary{
		HourlyTokens:     hourlyTokens,
		HourlyRemaining:  hourlyRemaining,
		DailyTokens:      dailyTokens,
		DailyRemaining:   dailyRemaining,
		SessionTokens:    u.sessionUsage,
		SessionRemaining: sessionRemaining,
		DailyCostUSD:     dailyCost,
		CostRemaining:    costRemaining,
		HourlyPct:        hourlyPct,
		DailyPct:         dailyPct,
	}
}

func (u *UsageTracker) CheckThresholds() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.checkThresholdsLocked()
}

func (u *UsageTracker) Reset() {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.sessionUsage = 0
	u.Alerts = nil
	u.firedThresholds = make(map[string]bool)
}

func (u *UsageTracker) PruneOld() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.pruneOldLocked()
}

func (u *UsageTracker) EstimateRemaining(tokensPerRequest int) int {
	u.mu.Lock()
	defer u.mu.Unlock()

	if tokensPerRequest <= 0 {
		return 0
	}

	u.pruneOldLocked()

	minRemaining := u.HourlyLimit - u.hourlyTokensLocked()
	if dailyRemaining := u.DailyLimit - u.dailyTokensLocked(); dailyRemaining < minRemaining {
		minRemaining = dailyRemaining
	}
	if sessionRemaining := u.SessionLimit - u.sessionUsage; sessionRemaining < minRemaining {
		minRemaining = sessionRemaining
	}
	if minRemaining <= 0 {
		return 0
	}
	return minRemaining / tokensPerRequest
}

func FormatUsageBar(pct float64, width int) string {
	if width <= 0 {
		return ""
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}

	filled := int(math.Round(pct / 100 * float64(width)))
	if filled > width {
		filled = width
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return fmt.Sprintf("[%s] %d%%", bar, int(pct))
}

func (u *UsageTracker) FormatSummary() string {
	summary := u.GetUsage()

	sessionPct := float64(0)
	if u.SessionLimit > 0 {
		sessionPct = float64(summary.SessionTokens) / float64(u.SessionLimit) * 100
	}
	costPct := float64(0)
	if u.CostLimitUSD > 0 {
		costPct = summary.DailyCostUSD / u.CostLimitUSD * 100
	}

	barWidth := 16
	var sb strings.Builder
	sb.WriteString("Token Usage:\n")
	fmt.Fprintf(&sb, "  Hourly:  %s / %s (%d%%) %s\n",
		formatNumber(summary.HourlyTokens),
		formatNumber(u.HourlyLimit),
		int(summary.HourlyPct),
		FormatUsageBar(summary.HourlyPct, barWidth))
	fmt.Fprintf(&sb, "  Daily:  %s / %s (%d%%) %s\n",
		formatNumber(summary.DailyTokens),
		formatNumber(u.DailyLimit),
		int(summary.DailyPct),
		FormatUsageBar(summary.DailyPct, barWidth))
	fmt.Fprintf(&sb, "  Session: %s / %s (%d%%) %s\n",
		formatNumber(summary.SessionTokens),
		formatNumber(u.SessionLimit),
		int(sessionPct),
		FormatUsageBar(sessionPct, barWidth))
	fmt.Fprintf(&sb, "  Cost:   $%.2f / $%.2f (%d%%) %s",
		summary.DailyCostUSD,
		u.CostLimitUSD,
		int(costPct),
		FormatUsageBar(costPct, barWidth))

	return sb.String()
}

func (u *UsageTracker) checkThresholdsLocked() {
	u.pruneOldLocked()

	if u.HourlyLimit > 0 {
		pct := float64(u.hourlyTokensLocked()) / float64(u.HourlyLimit) * 100
		u.emitAlert("hourly", pct, "hourly token usage")
	}
	if u.DailyLimit > 0 {
		pct := float64(u.dailyTokensLocked()) / float64(u.DailyLimit) * 100
		u.emitAlert("daily", pct, "daily token usage")
	}
	if u.SessionLimit > 0 {
		pct := float64(u.sessionUsage) / float64(u.SessionLimit) * 100
		u.emitAlert("session", pct, "session token usage")
	}
	if u.CostLimitUSD > 0 {
		pct := u.dailyCostLocked() / u.CostLimitUSD * 100
		u.emitAlert("cost", pct, "daily cost")
	}
}

func (u *UsageTracker) emitAlert(category string, pct float64, label string) {
	type threshold struct {
		pct   float64
		level string
	}
	for _, t := range []threshold{{100, "limit_reached"}, {80, "critical"}, {50, "warning"}} {
		if pct >= t.pct {
			key := fmt.Sprintf("%s_%d", category, int(t.pct))
			if !u.firedThresholds[key] {
				u.firedThresholds[key] = true
				u.Alerts = append(u.Alerts, Alert{
					Level:     t.level,
					Message:   fmt.Sprintf("%s at %.0f%% of limit", label, pct),
					Timestamp: time.Now(),
					Threshold: t.pct,
				})
			}
			return
		}
	}
}

func (u *UsageTracker) pruneOldLocked() {
	now := time.Now()
	hourAgo := now.Add(-1 * time.Hour)
	dayAgo := now.Add(-24 * time.Hour)

	prunedHourly := u.hourlyUsage[:0]
	for _, e := range u.hourlyUsage {
		if !e.Timestamp.Before(hourAgo) {
			prunedHourly = append(prunedHourly, e)
		}
	}
	u.hourlyUsage = prunedHourly

	prunedDaily := u.dailyUsage[:0]
	for _, e := range u.dailyUsage {
		if !e.Timestamp.Before(dayAgo) {
			prunedDaily = append(prunedDaily, e)
		}
	}
	u.dailyUsage = prunedDaily
}

func (u *UsageTracker) hourlyTokensLocked() int {
	total := 0
	for _, e := range u.hourlyUsage {
		total += e.Tokens
	}
	return total
}

func (u *UsageTracker) dailyTokensLocked() int {
	total := 0
	for _, e := range u.dailyUsage {
		total += e.Tokens
	}
	return total
}

func (u *UsageTracker) dailyCostLocked() float64 {
	total := 0.0
	for _, e := range u.dailyUsage {
		total += e.CostUSD
	}
	return total
}

func formatNumber(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}

	var out []byte
	prefix := len(s) % 3
	if prefix == 0 {
		prefix = 3
	}
	out = append(out, s[:prefix]...)
	for i := prefix; i < len(s); i += 3 {
		out = append(out, ',')
		out = append(out, s[i:i+3]...)
	}
	return string(out)
}
