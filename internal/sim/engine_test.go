package sim_test

import (
	"testing"
	"time"

	"stock-sim/internal/data"
	"stock-sim/internal/plan"
	"stock-sim/internal/sim"
)

func TestRunExecutesFirstEntryAndLadder(t *testing.T) {
	bars := sampleBars()
	result, err := sim.Run(bars, bars[0].Date, plan.MustParse(plan.DefaultQQQPlanYAML), sim.ExecutionPriceSameDayClose, sim.ReferencePriceClose, nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(result.Actions) == 0 {
		t.Fatal("expected at least one action")
	}
	if result.Summary.TotalInvestedPct <= 0 {
		t.Fatal("expected invested allocation")
	}
}

func TestRunSupportsNextDayOpen(t *testing.T) {
	bars := sampleBars()
	result, err := sim.Run(bars, bars[0].Date, plan.MustParse(plan.DefaultQQQPlanYAML), sim.ExecutionPriceNextDayOpen, sim.ReferencePriceClose, nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(result.Actions) == 0 {
		t.Fatal("expected actions")
	}
	if result.Actions[0].Date == bars[0].Date.Format("2006-01-02") {
		t.Fatal("expected next-day execution date")
	}
}

func TestRunSupportsExactExecutionPrice(t *testing.T) {
	raw := `metadata:
  name: Exact execution
  version: "1"
  symbol_scope: any
reference_price: sell_price
entry_rules:
  - id: first-entry
    label: First
    trigger:
      drop_pct_from_reference: 2
    action:
      type: buy_percent
      buy_percent: 100
constraints:
  max_actions_per_day: 1
  prevent_duplicate_level_buys: true
exit:
  hold_days_after_full_invest: 5
`

	bars := []data.Bar{
		makeBar("2024-01-02", 100, 102, 99, 100),
		makeBar("2024-01-03", 100, 101, 97, 98),
	}

	result, err := sim.Run(bars, bars[0].Date, plan.MustParse(raw), sim.ExecutionPriceExact, sim.ReferencePriceClose, nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(result.Actions) != 1 {
		t.Fatalf("expected one action, got %+v", result.Actions)
	}
	if result.Actions[0].Date != "2024-01-03" {
		t.Fatalf("expected exact execution on trigger date, got %+v", result.Actions[0])
	}
	if result.Actions[0].FillPrice != 98 {
		t.Fatalf("expected exact threshold fill 98.00, got %.4f", result.Actions[0].FillPrice)
	}
}

func TestRunSupportsExactExecutionPriceForTimeTrigger(t *testing.T) {
	raw := `metadata:
  name: Exact time trigger
  version: "1"
  symbol_scope: any
reference_price: sell_price
entry_rules:
  - id: first-entry
    label: First
    trigger:
      trading_days_since_reference: 1
    action:
      type: buy_percent
      buy_percent: 100
constraints:
  max_actions_per_day: 1
  prevent_duplicate_level_buys: true
exit:
  hold_days_after_full_invest: 5
`

	bars := []data.Bar{
		makeBar("2024-01-02", 100, 101, 99, 100),
		makeBar("2024-01-03", 101, 102, 100, 101.5),
	}

	result, err := sim.Run(bars, bars[0].Date, plan.MustParse(raw), sim.ExecutionPriceExact, sim.ReferencePriceClose, nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(result.Actions) != 1 {
		t.Fatalf("expected one action, got %+v", result.Actions)
	}
	if result.Actions[0].FillPrice != 101.5 {
		t.Fatalf("expected time trigger to fall back to close 101.50, got %.4f", result.Actions[0].FillPrice)
	}
}

func TestRunSupportsAverageOfDay(t *testing.T) {
	bars := sampleBars()
	result, err := sim.Run(bars, bars[0].Date, plan.MustParse(plan.DefaultQQQPlanYAML), sim.ExecutionPriceAverageOfDay, sim.ReferencePriceClose, nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(result.Actions) == 0 {
		t.Fatal("expected actions")
	}
	if result.Actions[0].Date != bars[0].Date.Format("2006-01-02") {
		t.Fatal("expected same-day execution date")
	}

	expected := (bars[0].Open + bars[0].High + bars[0].Low + bars[0].Close) / 4
	if result.Actions[0].FillPrice != expected {
		t.Fatalf("expected average-of-day fill %.4f, got %.4f", expected, result.Actions[0].FillPrice)
	}
}

func TestRunReturnsPendingTriggersForUnboughtRules(t *testing.T) {
	raw := `metadata:
  name: Pending trigger projection
  version: "1"
  symbol_scope: any
reference_price: sell_price
entry_rules:
  - id: first-entry
    label: First
    trigger:
      drop_pct_from_reference: 2
    action:
      type: buy_percent
      buy_percent: 20
  - id: ladder-4
    label: Ladder 4
    trigger:
      drop_pct_from_reference: 4
    action:
      type: buy_percent
      buy_percent: 15
constraints:
  max_actions_per_day: 1
  prevent_duplicate_level_buys: true
exit: {}
`

	bars := []data.Bar{
		makeBar("2024-01-02", 100, 101, 99, 100),
		makeBar("2024-01-03", 100, 100, 98, 99),
	}

	result, err := sim.Run(bars, bars[0].Date, plan.MustParse(raw), sim.ExecutionPriceExact, sim.ReferencePriceClose, nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(result.Actions) != 1 {
		t.Fatalf("expected one executed action, got %+v", result.Actions)
	}
	if len(result.PendingTriggers) != 1 {
		t.Fatalf("expected one pending trigger, got %+v", result.PendingTriggers)
	}
	if result.PendingTriggers[0].TriggerID != "ladder-4" {
		t.Fatalf("expected pending ladder-4, got %+v", result.PendingTriggers[0])
	}
	if result.PendingTriggers[0].BuyPrice == nil || *result.PendingTriggers[0].BuyPrice != 96 {
		t.Fatalf("expected projected buy price 96.00, got %+v", result.PendingTriggers[0].BuyPrice)
	}
	if result.PendingTriggers[0].CashToInvestPct != 15 {
		t.Fatalf("expected projected cash allocation 15, got %.2f", result.PendingTriggers[0].CashToInvestPct)
	}
}

func TestRunSupportsRandomInDay(t *testing.T) {
	bars := sampleBars()
	result, err := sim.Run(bars, bars[0].Date, plan.MustParse(plan.DefaultQQQPlanYAML), sim.ExecutionPriceRandomInDay, sim.ReferencePriceClose, nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(result.Actions) == 0 {
		t.Fatal("expected actions")
	}
	if result.Actions[0].Date != bars[0].Date.Format("2006-01-02") {
		t.Fatal("expected same-day execution date")
	}
	if result.Actions[0].FillPrice < bars[0].Low || result.Actions[0].FillPrice > bars[0].High {
		t.Fatalf("expected intraday random fill within bar range [%.4f, %.4f], got %.4f", bars[0].Low, bars[0].High, result.Actions[0].FillPrice)
	}
}

func TestStrongRecoveryRequiresActualSMACross(t *testing.T) {
	bars := sampleBars()
	result, err := sim.Run(bars, bars[0].Date, plan.MustParse(plan.DefaultQQQPlanYAML), sim.ExecutionPriceSameDayClose, sim.ReferencePriceClose, nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(result.Actions) < 2 {
		t.Fatalf("expected multiple actions, got %d", len(result.Actions))
	}
	if result.Actions[1].TriggerID == "strong-recovery" && result.Actions[1].Date == bars[4].Date.Format("2006-01-02") {
		t.Fatal("strong recovery should not fire just because price remains above the SMAs")
	}
}

func TestInvestRemainingCannotBeFirstAction(t *testing.T) {
	raw := `metadata:
  name: Immediate recovery trap
  version: "1"
  symbol_scope: any
reference_price: sell_price
entry_rules:
  - id: strong-recovery
    label: Strong recovery
    trigger:
      close_above_sma: [20, 50]
    action:
      type: invest_remaining
constraints:
  max_actions_per_day: 1
  prevent_duplicate_level_buys: true
exit:
  hold_days_after_full_invest: 20
`

	result, err := sim.Run(sampleBars(), sampleBars()[55].Date, plan.MustParse(raw), sim.ExecutionPriceSameDayClose, sim.ReferencePriceClose, nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(result.Actions) != 0 {
		t.Fatalf("expected no actions, got %+v", result.Actions)
	}
}

func TestExitDefaultsToReferenceReclaimAfterFullInvest(t *testing.T) {
	raw := `metadata:
  name: Reclaim exit
  version: "1"
  symbol_scope: any
reference_price: sell_price
entry_rules:
  - id: first-entry
    label: First
    trigger:
      drop_pct_from_reference: 2
    action:
      type: buy_percent
      buy_percent: 100
constraints:
  max_actions_per_day: 1
  prevent_duplicate_level_buys: true
exit: {}
`

	bars := []data.Bar{
		makeBar("2024-01-02", 100, 101, 99, 100),
		makeBar("2024-01-03", 100, 100, 97, 98),
		makeBar("2024-01-04", 98, 99, 96, 97),
		makeBar("2024-01-05", 97, 99, 96, 98),
		makeBar("2024-01-08", 98, 101.5, 97.5, 100.5),
		makeBar("2024-01-09", 100.5, 102, 100, 101),
	}

	result, err := sim.Run(bars, bars[0].Date, plan.MustParse(raw), sim.ExecutionPriceSameDayClose, sim.ReferencePriceClose, nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if result.Summary.EndDate != "2024-01-08" {
		t.Fatalf("expected end date to be first reference reclaim day, got %+v", result.Summary)
	}
}

func TestFixedHoldDaysOverridesReferenceReclaimExit(t *testing.T) {
	raw := `metadata:
  name: Fixed hold exit
  version: "1"
  symbol_scope: any
reference_price: sell_price
entry_rules:
  - id: first-entry
    label: First
    trigger:
      drop_pct_from_reference: 2
    action:
      type: buy_percent
      buy_percent: 100
constraints:
  max_actions_per_day: 1
  prevent_duplicate_level_buys: true
exit:
  hold_days_after_full_invest: 2
`

	bars := []data.Bar{
		makeBar("2024-01-02", 100, 101, 99, 100),
		makeBar("2024-01-03", 100, 100, 97, 98),
		makeBar("2024-01-04", 98, 99, 96, 97),
		makeBar("2024-01-05", 97, 101.5, 96, 100.5),
		makeBar("2024-01-08", 100.5, 102, 100, 101),
	}

	result, err := sim.Run(bars, bars[0].Date, plan.MustParse(raw), sim.ExecutionPriceSameDayClose, sim.ReferencePriceClose, nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if result.Summary.EndDate != "2024-01-05" {
		t.Fatalf("expected fixed hold-day override end date, got %+v", result.Summary)
	}
}

func TestReboundConsumesNextPlannedTranche(t *testing.T) {
	raw := `metadata:
  name: Rebound tranche consumption
  version: "1"
  symbol_scope: any
reference_price: sell_price
entry_rules:
  - id: first-entry
    label: First
    trigger:
      drop_pct_from_reference: 2
    action:
      type: buy_percent
      buy_percent: 20
  - id: ladder-4
    label: Ladder 4
    trigger:
      drop_pct_from_reference: 4
    action:
      type: buy_percent
      buy_percent: 15
  - id: ladder-7
    label: Ladder 7
    trigger:
      drop_pct_from_reference: 7
    action:
      type: buy_percent
      buy_percent: 15
  - id: rebound
    label: Rebound
    trigger:
      rise_pct_from_low_since_reference: 4
    action:
      type: buy_next_planned_allocation
constraints:
  max_actions_per_day: 1
  prevent_duplicate_level_buys: true
exit:
  hold_days_after_full_invest: 20
`

	bars := []data.Bar{
		makeBar("2024-01-02", 100, 101, 98, 100),
		makeBar("2024-01-03", 100, 100, 97, 98),
		makeBar("2024-01-04", 98, 99, 95, 96),
		makeBar("2024-01-05", 96, 96.5, 93.5, 94),
		makeBar("2024-01-08", 94, 98, 93.8, 97.5),
		makeBar("2024-01-09", 97.5, 98, 92.5, 93),
	}

	result, err := sim.Run(bars, bars[0].Date, plan.MustParse(raw), sim.ExecutionPriceSameDayClose, sim.ReferencePriceClose, nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	total := 0.0
	for _, action := range result.Actions {
		total += action.AllocationPct
	}

	if len(result.Actions) != 3 {
		t.Fatalf("expected exactly 3 actions, got %+v", result.Actions)
	}
	if total != 50 {
		t.Fatalf("expected total 50%% invested, got %.2f with actions %+v", total, result.Actions)
	}
	if result.Actions[2].TriggerID != "rebound" || result.Actions[2].AllocationPct != 15 {
		t.Fatalf("expected rebound to consume the next 15%% tranche, got %+v", result.Actions[2])
	}
}

func TestRunSupportsReferencePriceModes(t *testing.T) {
	raw := `metadata:
  name: Reference price mode
  version: "1"
  symbol_scope: any
reference_price: sell_price
entry_rules:
  - id: first-entry
    label: First
    trigger:
      drop_pct_from_reference: 15
    action:
      type: buy_percent
      buy_percent: 100
constraints:
  max_actions_per_day: 1
  prevent_duplicate_level_buys: true
exit:
  hold_days_after_full_invest: 20
`

	bars := []data.Bar{
		makeBar("2024-01-02", 120, 125, 95, 100),
		makeBar("2024-01-03", 100, 101, 98, 99),
	}

	result, err := sim.Run(bars, bars[0].Date, plan.MustParse(raw), sim.ExecutionPriceSameDayClose, sim.ReferencePriceHigh, nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if result.Summary.ReferencePrice != 125 {
		t.Fatalf("expected reference price 125 from high, got %.2f", result.Summary.ReferencePrice)
	}
	if len(result.Actions) != 1 {
		t.Fatalf("expected entry to trigger from high-based reference, got %+v", result.Actions)
	}
}

func TestRunSupportsReferencePriceOverride(t *testing.T) {
	raw := `metadata:
  name: Reference price override
  version: "1"
  symbol_scope: any
reference_price: sell_price
entry_rules:
  - id: first-entry
    label: First
    trigger:
      drop_pct_from_reference: 10
    action:
      type: buy_percent
      buy_percent: 100
constraints:
  max_actions_per_day: 1
  prevent_duplicate_level_buys: true
exit:
  hold_days_after_full_invest: 20
`

	bars := []data.Bar{
		makeBar("2024-01-02", 100, 102, 95, 100),
		makeBar("2024-01-03", 100, 101, 89, 90),
	}
	override := 110.0

	result, err := sim.Run(bars, bars[0].Date, plan.MustParse(raw), sim.ExecutionPriceSameDayClose, sim.ReferencePriceClose, &override)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if result.Summary.ReferencePrice != override {
		t.Fatalf("expected override reference price %.2f, got %.2f", override, result.Summary.ReferencePrice)
	}
	if len(result.Actions) != 1 {
		t.Fatalf("expected override-based trigger to fire, got %+v", result.Actions)
	}
}

func makeBar(date string, open, high, low, close float64) data.Bar {
	parsed, _ := time.Parse("2006-01-02", date)
	return data.Bar{
		Symbol: "QQQ",
		Date:   parsed,
		Open:   open,
		High:   high,
		Low:    low,
		Close:  close,
		Volume: 1000,
		VWAP:   close,
	}
}

func sampleBars() []data.Bar {
	start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	closes := []float64{
		100, 98, 96, 93, 89, 85, 88, 91, 94, 96,
		99, 102, 104, 106, 107, 108, 110, 111, 112, 113,
		114, 115, 116, 117, 118, 119, 120, 121, 122, 123,
		124, 125, 126, 127, 128, 129, 130, 131, 132, 133,
		134, 135, 136, 137, 138, 139, 140, 141, 142, 143,
		144, 145, 146, 147, 148, 149, 150, 151, 152, 153,
	}

	bars := make([]data.Bar, 0, len(closes))
	for i, closePrice := range closes {
		open := closePrice + 0.5
		if i > 0 {
			open = closes[i-1]
		}
		bars = append(bars, data.Bar{
			Symbol: "QQQ",
			Date:   start.AddDate(0, 0, i),
			Open:   open,
			High:   closePrice + 1,
			Low:    closePrice - 2,
			Close:  closePrice,
			Volume: 1000,
			VWAP:   closePrice,
		})
	}
	return bars
}
