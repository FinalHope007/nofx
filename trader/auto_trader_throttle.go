package trader

import (
	"fmt"
	"nofx/kernel"
	"nofx/market"
	"nofx/store"
	"strings"
	"time"
)

type throttleDurationConfig struct {
	minHold, noiseClose, reentry time.Duration
}

// throttleConfig returns the effective throttle gates. Zero/unset fields fall
// back to the historical hardcoded defaults (Section B) so existing strategies
// and nil-config tests behave exactly as before.
func (at *AutoTrader) throttleConfig() store.ThrottlingConfig {
	if at == nil || at.config.StrategyConfig == nil {
		return defaultThrottlingConfig()
	}
	return withThrottlingDefaults(at.config.StrategyConfig.RiskControl.Throttling)
}

// defaultThrottlingConfig returns the historical Section B defaults.
func defaultThrottlingConfig() store.ThrottlingConfig {
	return store.ThrottlingConfig{
		MaxOpensPerHour:               3,
		MaxOpensPerCycle:              2,
		MinHoldDurationMin:            90,
		NoiseCloseHoldDurationMin:     180,
		ReentryCooldownMin:            240,
		EarlyCloseStopLossBypassPct:   -3.0,
		EarlyCloseTakeProfitBypassPct: 8.0,
		NoiseCloseLossFloorPct:        -2.0,
		NoiseCloseProfitCeilingPct:    3.0,
	}
}

// withThrottlingDefaults fills any zero-valued gate with the historical default.
func withThrottlingDefaults(t store.ThrottlingConfig) store.ThrottlingConfig {
	d := defaultThrottlingConfig()
	if t.MaxOpensPerHour <= 0 {
		t.MaxOpensPerHour = d.MaxOpensPerHour
	}
	if t.MaxOpensPerCycle <= 0 {
		t.MaxOpensPerCycle = d.MaxOpensPerCycle
	}
	if t.MinHoldDurationMin <= 0 {
		t.MinHoldDurationMin = d.MinHoldDurationMin
	}
	if t.NoiseCloseHoldDurationMin <= 0 {
		t.NoiseCloseHoldDurationMin = d.NoiseCloseHoldDurationMin
	}
	if t.ReentryCooldownMin <= 0 {
		t.ReentryCooldownMin = d.ReentryCooldownMin
	}
	// The four percentage gates share store.NormalizeThrottlingPercentageDefaults
	// as the single source of truth, so persisted == enforced.
	store.NormalizeThrottlingPercentageDefaults(&t)
	return t
}

func (at *AutoTrader) throttleDurations() throttleDurationConfig {
	tc := at.throttleConfig()
	return throttleDurationConfig{
		minHold:    time.Duration(tc.MinHoldDurationMin) * time.Minute,
		noiseClose: time.Duration(tc.NoiseCloseHoldDurationMin) * time.Minute,
		reentry:    time.Duration(tc.ReentryCooldownMin) * time.Minute,
	}
}

// positionPricePnLPct converts the margin-based UnrealizedPnLPct reported for
// a position into the underlying price-move percentage.
func positionPricePnLPct(pos *kernel.PositionInfo) float64 {
	if pos == nil {
		return 0
	}
	if pos.Leverage > 1 {
		return pos.UnrealizedPnLPct / float64(pos.Leverage)
	}
	return pos.UnrealizedPnLPct
}

func isOpenAction(action string) bool {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "open_long", "open_short":
		return true
	default:
		return false
	}
}

func isCloseAction(action string) bool {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "close_long", "close_short":
		return true
	default:
		return false
	}
}

func closeActionSide(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "close_long":
		return "long"
	case "close_short":
		return "short"
	default:
		return ""
	}
}

func openActionSide(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "open_long":
		return "long"
	case "open_short":
		return "short"
	default:
		return ""
	}
}

func normalizedDecisionSymbol(symbol string) string {
	return market.Normalize(strings.TrimSpace(symbol))
}

func (at *AutoTrader) tradeThrottleReason(decision kernel.Decision, ctx *kernel.Context, opensQueuedThisCycle int) string {
	if ctx == nil {
		return ""
	}

	switch {
	case isOpenAction(decision.Action):
		return at.openThrottleReason(decision, ctx, opensQueuedThisCycle)
	case isCloseAction(decision.Action):
		return at.closeThrottleReason(decision, ctx)
	default:
		return ""
	}
}

func (at *AutoTrader) openThrottleReason(decision kernel.Decision, ctx *kernel.Context, opensQueuedThisCycle int) string {
	symbol := normalizedDecisionSymbol(decision.Symbol)
	if symbol == "" {
		return ""
	}

	if opensQueuedThisCycle >= at.throttleConfig().MaxOpensPerCycle {
		return fmt.Sprintf("trade throttle: only %d new position may be opened per cycle", at.throttleConfig().MaxOpensPerCycle)
	}

	if pos := findAnyContextPosition(ctx, symbol); pos != nil {
		return fmt.Sprintf("trade throttle: %s already has an open %s position; manage or close it before opening another side", symbol, pos.Side)
	}

	openCount, err := at.countRecentOpenOrders(time.Now().Add(-1 * time.Hour))
	if err != nil {
		at.logWarnf("⚠️ Trade throttle could not read recent open orders: %v", err)
	} else if openCount >= at.throttleConfig().MaxOpensPerHour {
		return fmt.Sprintf("trade throttle: %d open order already executed in the last hour; max is %d", openCount, at.throttleConfig().MaxOpensPerHour)
	}

	d := at.throttleDurations()
	if order := at.findRecentCloseOrder(symbol, time.Now().Add(-d.reentry)); order != nil {
		age := time.Since(time.UnixMilli(order.CreatedAt))
		remaining := d.reentry - age
		if remaining < 0 {
			remaining = 0
		}
		return fmt.Sprintf("trade throttle: %s was closed %s ago; wait %s before re-entry", symbol, roundDuration(age), roundDuration(remaining))
	}

	return ""
}

func (at *AutoTrader) closeThrottleReason(decision kernel.Decision, ctx *kernel.Context) string {
	symbol := normalizedDecisionSymbol(decision.Symbol)
	side := closeActionSide(decision.Action)
	if symbol == "" || side == "" {
		return ""
	}

	pos := findContextPosition(ctx, symbol, side)
	pnlPct := 0.0
	entryTime := int64(0)
	if pos != nil {
		pnlPct = positionPricePnLPct(pos)
		entryTime = pos.UpdateTime
	}

	d := at.throttleDurations()
	if order := at.findRecentOpenOrder(symbol, side, time.Now().Add(-d.noiseClose)); order != nil && order.CreatedAt > entryTime {
		entryTime = order.CreatedAt
	}
	if entryTime <= 0 {
		return ""
	}

	heldFor := time.Since(time.UnixMilli(entryTime))
	if heldFor < 0 {
		heldFor = 0
	}
	tc := at.throttleConfig()
	if heldFor >= d.minHold {
		if heldFor >= d.noiseClose ||
			pnlPct <= tc.NoiseCloseLossFloorPct ||
			pnlPct >= tc.NoiseCloseProfitCeilingPct {
			return ""
		}

		remaining := d.noiseClose - heldFor
		return fmt.Sprintf(
			"trade throttle: %s %s has been held for %s with price PnL %.2f%%; it is still inside the noise band %.1f%% to %.1f%%, so wait about %s before a flat/small close",
			symbol,
			side,
			roundDuration(heldFor),
			pnlPct,
			tc.NoiseCloseLossFloorPct,
			tc.NoiseCloseProfitCeilingPct,
			roundDuration(remaining),
		)
	}

	// Do not block true risk exits or unusually strong take-profit exits.
	if pnlPct <= tc.EarlyCloseStopLossBypassPct || pnlPct >= tc.EarlyCloseTakeProfitBypassPct {
		return ""
	}

	remaining := d.minHold - heldFor
	return fmt.Sprintf(
		"trade throttle: %s %s has only been held for %s with price PnL %.2f%%; min AI-managed hold is %s unless price loss <= %.1f%% or price profit >= %.1f%%",
		symbol,
		side,
		roundDuration(heldFor),
		pnlPct,
		roundDuration(d.minHold),
		tc.EarlyCloseStopLossBypassPct,
		tc.EarlyCloseTakeProfitBypassPct,
	) + fmt.Sprintf("; wait about %s", roundDuration(remaining))
}

func findContextPosition(ctx *kernel.Context, symbol string, side string) *kernel.PositionInfo {
	if ctx == nil {
		return nil
	}
	for i := range ctx.Positions {
		pos := &ctx.Positions[i]
		if normalizedDecisionSymbol(pos.Symbol) == symbol && strings.EqualFold(pos.Side, side) {
			return pos
		}
	}
	return nil
}

func findAnyContextPosition(ctx *kernel.Context, symbol string) *kernel.PositionInfo {
	if ctx == nil {
		return nil
	}
	for i := range ctx.Positions {
		pos := &ctx.Positions[i]
		if normalizedDecisionSymbol(pos.Symbol) == symbol {
			return pos
		}
	}
	return nil
}

func (at *AutoTrader) recentOrders(limit int) ([]*store.TraderOrder, error) {
	if at == nil || at.store == nil {
		return nil, nil
	}
	return at.store.Order().GetTraderOrders(at.id, limit)
}

func (at *AutoTrader) countRecentOpenOrders(since time.Time) (int, error) {
	orders, err := at.recentOrders(100)
	if err != nil {
		return 0, err
	}
	return countDistinctOpenEvents(orders, since.UTC().UnixMilli()), nil
}

// orderPositionKey returns a stable dedup key for a position-side regardless of
// how the exchange labels it (positionSide may be empty on some adapters).
func orderPositionKey(o *store.TraderOrder) string {
	key := o.Symbol
	if ps := strings.TrimSpace(o.PositionSide); ps != "" {
		return key + "|" + strings.ToUpper(ps)
	}
	return key + "|" + strings.ToUpper(strings.TrimSpace(o.Side))
}

// countDistinctOpenEvents counts distinct (symbol, side) position-open events,
// so a single position opened as multiple fills counts once.
func countDistinctOpenEvents(orders []*store.TraderOrder, sinceMs int64) int {
	seen := map[string]bool{}
	count := 0
	for _, order := range orders {
		if order == nil || order.CreatedAt < sinceMs || isCanceledOrder(order) {
			continue
		}
		if !isOpenAction(order.OrderAction) {
			continue
		}
		key := orderPositionKey(order)
		if !seen[key] {
			seen[key] = true
			count++
		}
	}
	return count
}

func (at *AutoTrader) findRecentCloseOrder(symbol string, since time.Time) *store.TraderOrder {
	orders, err := at.recentOrders(100)
	if err != nil {
		at.logWarnf("⚠️ Trade throttle could not read recent close orders: %v", err)
		return nil
	}
	sinceMs := since.UTC().UnixMilli()
	// Latest close for the symbol (deduped by position-side), so a multi-fill
	// close is treated as a single close event.
	var latest *store.TraderOrder
	seen := map[string]bool{}
	for _, order := range orders {
		if order == nil || order.CreatedAt < sinceMs || isCanceledOrder(order) {
			continue
		}
		if !(isCloseAction(order.OrderAction) && normalizedDecisionSymbol(order.Symbol) == symbol) {
			continue
		}
		key := orderPositionKey(order)
		if seen[key] {
			continue
		}
		seen[key] = true
		if latest == nil || order.CreatedAt > latest.CreatedAt {
			latest = order
		}
	}
	return latest
}

func (at *AutoTrader) findRecentOpenOrder(symbol string, side string, since time.Time) *store.TraderOrder {
	orders, err := at.recentOrders(100)
	if err != nil {
		at.logWarnf("⚠️ Trade throttle could not read recent open orders: %v", err)
		return nil
	}
	sinceMs := since.UTC().UnixMilli()
	for _, order := range orders {
		if order == nil || order.CreatedAt < sinceMs || isCanceledOrder(order) {
			continue
		}
		if normalizedDecisionSymbol(order.Symbol) == symbol &&
			strings.EqualFold(openActionSide(order.OrderAction), side) {
			return order
		}
	}
	return nil
}

func isCanceledOrder(order *store.TraderOrder) bool {
	status := strings.ToUpper(strings.TrimSpace(order.Status))
	return status == "CANCELED" || status == "CANCELLED" || status == "REJECTED" || status == "EXPIRED"
}

func roundDuration(d time.Duration) string {
	if d < time.Minute {
		return "0m"
	}
	return d.Round(time.Minute).String()
}
