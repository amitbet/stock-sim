package sim

import (
	"fmt"
	"slices"
	"time"

	"stock-sim/internal/data"
	"stock-sim/internal/plan"
)

func Run(bars []data.Bar, referenceSellDate time.Time, strategy plan.StrategyPlan, mode ExecutionPriceMode) (Result, error) {
	var result Result
	if len(bars) == 0 {
		return result, fmt.Errorf("no bars provided")
	}
	if mode != ExecutionPriceSameDayClose && mode != ExecutionPriceNextDayOpen {
		return result, fmt.Errorf("unsupported execution mode %q", mode)
	}

	referenceIndex := slices.IndexFunc(bars, func(bar data.Bar) bool {
		return sameDay(bar.Date, referenceSellDate)
	})
	if referenceIndex < 0 {
		return result, fmt.Errorf("reference sell date %s not found in bar set", referenceSellDate.Format("2006-01-02"))
	}

	referencePrice := bars[referenceIndex].Close
	executedRules := map[string]bool{}
	var executions []Execution
	plannedBuys, plannedRuleIDs := fixedAllocations(strategy)
	nextAllocationIndex := 0
	totalInvestedPct := 0.0
	lowestLow := bars[referenceIndex].Low
	fullInvestIndex := -1
	maxDrawdownPct := 0.0

	sortedRules := append([]plan.EntryRule(nil), strategy.EntryRules...)
	slices.SortFunc(sortedRules, func(a, b plan.EntryRule) int {
		return b.Priority - a.Priority
	})

	for barIndex := referenceIndex; barIndex < len(bars); barIndex++ {
		bar := bars[barIndex]
		if bar.Low < lowestLow {
			lowestLow = bar.Low
		}

		if totalInvestedPct > 0 {
			avgPrice := weightedAverage(executions)
			drawdown := ((bar.Low - avgPrice) / avgPrice) * 100
			if drawdown < maxDrawdownPct {
				maxDrawdownPct = drawdown
			}
		}

		actionsToday := 0
		for _, rule := range sortedRules {
			if strategy.Constraints.PreventDuplicateLevelBuys && executedRules[rule.ID] {
				continue
			}
			if actionsToday >= strategy.Constraints.MaxActionsPerDay {
				break
			}
			if !ruleEligible(rule, totalInvestedPct) {
				continue
			}
			if !triggerMatches(rule.Trigger, bars, referenceIndex, barIndex, referencePrice, lowestLow) {
				continue
			}

			allocation := allocationForRule(rule, plannedBuys, nextAllocationIndex, totalInvestedPct)
			if allocation <= 0 {
				continue
			}

			fillIndex := barIndex
			fillPrice := bar.Close
			if mode == ExecutionPriceNextDayOpen {
				if barIndex+1 >= len(bars) {
					continue
				}
				fillIndex = barIndex + 1
				fillPrice = bars[fillIndex].Open
			}

			executions = append(executions, Execution{
				Date:      bars[fillIndex].Date,
				FillPrice: fillPrice,
				Percent:   allocation,
			})
			result.Actions = append(result.Actions, Action{
				Date:          bars[fillIndex].Date.Format("2006-01-02"),
				TriggerID:     rule.ID,
				Label:         rule.Label,
				ActionType:    rule.Action.Type,
				AllocationPct: allocation,
				FillPrice:     fillPrice,
				Notes:         explainExecution(rule, mode, bar.Date, bars[fillIndex].Date),
			})

			totalInvestedPct += allocation
			executedRules[rule.ID] = true
			actionsToday++

			if rule.Action.Type == "buy_percent" {
				nextAllocationIndex++
			} else if rule.Action.Type == "buy_next_planned_allocation" {
				if nextAllocationIndex < len(plannedRuleIDs) {
					executedRules[plannedRuleIDs[nextAllocationIndex]] = true
				}
				nextAllocationIndex++
			} else if rule.Action.Type == "invest_remaining" {
				nextAllocationIndex = len(plannedBuys)
			}

			if fullInvestIndex < 0 && totalInvestedPct >= 99.999 {
				fullInvestIndex = fillIndex
			}
		}
	}

	endIndex := len(bars) - 1
	if fullInvestIndex >= 0 {
		endIndex = determineEndIndex(bars, fullInvestIndex, referencePrice, strategy.Exit)
	}

	finalPrice := bars[endIndex].Close
	avgBuyPrice := weightedAverage(executions)
	gainPct := 0.0
	if avgBuyPrice > 0 {
		gainPct = ((finalPrice - avgBuyPrice) / avgBuyPrice) * 100
	}

	result.Summary = Summary{
		ReferenceSellDate: referenceSellDate.Format("2006-01-02"),
		EndDate:           bars[endIndex].Date.Format("2006-01-02"),
		GainPct:           gainPct,
		TotalInvestedPct:  totalInvestedPct,
		ExecutionMode:     mode,
	}
	if fullInvestIndex >= 0 {
		result.Summary.FullInvestDate = bars[fullInvestIndex].Date.Format("2006-01-02")
		result.Stats.BarsToFullInvest = fullInvestIndex - referenceIndex
	}
	result.Stats.BarsToEnd = endIndex - referenceIndex
	result.Stats.MaxDrawdownPct = maxDrawdownPct

	return result, nil
}

func fixedAllocations(strategy plan.StrategyPlan) ([]float64, []string) {
	var allocations []float64
	var ruleIDs []string
	for _, rule := range strategy.EntryRules {
		if rule.Action.Type == "buy_percent" && rule.Action.BuyPercent != nil {
			allocations = append(allocations, *rule.Action.BuyPercent)
			ruleIDs = append(ruleIDs, rule.ID)
		}
	}
	return allocations, ruleIDs
}

func allocationForRule(rule plan.EntryRule, planned []float64, nextIndex int, invested float64) float64 {
	remaining := 100.0 - invested
	if remaining <= 0 {
		return 0
	}

	switch rule.Action.Type {
	case "buy_percent":
		if rule.Action.BuyPercent == nil {
			return 0
		}
		return minFloat(*rule.Action.BuyPercent, remaining)
	case "buy_next_planned_allocation":
		if nextIndex >= len(planned) {
			return 0
		}
		return minFloat(planned[nextIndex], remaining)
	case "invest_remaining":
		return remaining
	default:
		return 0
	}
}

func ruleEligible(rule plan.EntryRule, invested float64) bool {
	switch rule.Action.Type {
	case "buy_next_planned_allocation", "invest_remaining":
		return invested > 0
	default:
		return true
	}
}

func triggerMatches(trigger plan.Trigger, bars []data.Bar, referenceIndex, currentIndex int, referencePrice, lowestLow float64) bool {
	if len(trigger.AnyOf) > 0 {
		for _, child := range trigger.AnyOf {
			if triggerMatches(child, bars, referenceIndex, currentIndex, referencePrice, lowestLow) {
				return true
			}
		}
		return false
	}

	bar := bars[currentIndex]
	ok := true
	if trigger.DropPctFromReference != nil {
		dropPct := ((referencePrice - bar.Low) / referencePrice) * 100
		ok = ok && dropPct >= *trigger.DropPctFromReference
	}
	if trigger.TradingDaysSinceReference != nil {
		ok = ok && (currentIndex-referenceIndex) >= *trigger.TradingDaysSinceReference
	}
	if trigger.RisePctFromLowSinceRef != nil {
		risePct := ((bar.Close - lowestLow) / lowestLow) * 100
		ok = ok && risePct >= *trigger.RisePctFromLowSinceRef
	}
	if len(trigger.CloseAboveSMA) > 0 {
		crossedAbove, canEvaluate := crossedAboveAllSMAs(bars, currentIndex, trigger.CloseAboveSMA)
		if !canEvaluate || !crossedAbove {
			return false
		}
	}
	return ok
}

func crossedAboveAllSMAs(bars []data.Bar, currentIndex int, periods []int) (bool, bool) {
	if currentIndex <= 0 {
		return false, false
	}

	currentBar := bars[currentIndex]
	previousBar := bars[currentIndex-1]
	crossedAny := false

	for _, period := range periods {
		currentSMA, currentCan := smaAt(bars, currentIndex, period)
		previousSMA, previousCan := smaAt(bars, currentIndex-1, period)
		if !currentCan || !previousCan {
			return false, false
		}
		if currentBar.Close <= currentSMA {
			return false, true
		}
		if previousBar.Close <= previousSMA {
			crossedAny = true
		}
	}

	return crossedAny, true
}

func smaAt(bars []data.Bar, index, period int) (float64, bool) {
	if index+1 < period {
		return 0, false
	}
	sum := 0.0
	for i := index - period + 1; i <= index; i++ {
		sum += bars[i].Close
	}
	return sum / float64(period), true
}

func weightedAverage(executions []Execution) float64 {
	totalPct := 0.0
	totalCost := 0.0
	for _, execution := range executions {
		totalPct += execution.Percent
		totalCost += execution.FillPrice * execution.Percent
	}
	if totalPct == 0 {
		return 0
	}
	return totalCost / totalPct
}

func sameDay(left, right time.Time) bool {
	return left.Format("2006-01-02") == right.Format("2006-01-02")
}

func explainExecution(rule plan.EntryRule, mode ExecutionPriceMode, triggerDay, fillDay time.Time) string {
	if mode == ExecutionPriceSameDayClose {
		return fmt.Sprintf("%s executed on %s close", rule.ID, triggerDay.Format("2006-01-02"))
	}
	return fmt.Sprintf("%s triggered on %s and filled on next open %s", rule.ID, triggerDay.Format("2006-01-02"), fillDay.Format("2006-01-02"))
}

func minFloat(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}

func determineEndIndex(bars []data.Bar, fullInvestIndex int, referencePrice float64, exitRule plan.ExitRule) int {
	if fullInvestIndex < 0 {
		return len(bars) - 1
	}

	if exitRule.HoldDaysAfterFullInvest != nil && *exitRule.HoldDaysAfterFullInvest > 0 {
		return min(fullInvestIndex+*exitRule.HoldDaysAfterFullInvest, len(bars)-1)
	}

	for index := fullInvestIndex; index < len(bars); index++ {
		if bars[index].High > referencePrice {
			return index
		}
	}

	return len(bars) - 1
}
