package sim

import (
	"fmt"
	"hash/fnv"
	"time"

	"stock-sim/internal/data"
	"stock-sim/internal/plan"
)

func Run(bars []data.Bar, referenceSellDate time.Time, strategy plan.StrategyPlan, mode ExecutionPriceMode, referencePriceMode ReferencePriceMode, referencePriceOverride *float64) (Result, error) {
	result := Result{
		Actions: []Action{},
	}
	if len(bars) == 0 {
		return result, fmt.Errorf("no bars provided")
	}
	if mode != ExecutionPriceSameDayClose &&
		mode != ExecutionPriceNextDayOpen &&
		mode != ExecutionPriceRandomInDay &&
		mode != ExecutionPriceAverageOfDay {
		return result, fmt.Errorf("unsupported execution mode %q", mode)
	}
	if referencePriceMode == "" {
		referencePriceMode = ReferencePriceClose
	}
	if referencePriceMode != ReferencePriceClose &&
		referencePriceMode != ReferencePriceOpen &&
		referencePriceMode != ReferencePriceHigh &&
		referencePriceMode != ReferencePriceLow {
		return result, fmt.Errorf("unsupported reference price mode %q", referencePriceMode)
	}

	referenceIndex := indexBarByDate(bars, referenceSellDate)
	if referenceIndex < 0 {
		return result, fmt.Errorf("reference sell date %s not found in bar set", referenceSellDate.Format("2006-01-02"))
	}

	referencePrice := referencePriceForBar(bars[referenceIndex], referencePriceMode)
	if referencePriceOverride != nil {
		referencePrice = *referencePriceOverride
	}
	executedRules := map[string]bool{}
	var executions []Execution
	plannedBuys, plannedRuleIDs := fixedAllocations(strategy)
	nextAllocationIndex := 0
	totalInvestedPct := 0.0
	lowestLow := bars[referenceIndex].Low
	fullInvestIndex := -1
	maxDrawdownPct := 0.0

	sortedRules := append([]plan.EntryRule(nil), strategy.EntryRules...)
	sortRulesByPriority(sortedRules)

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

			fillIndex, fillPrice, ok := resolveExecution(mode, bars, barIndex, rule)
			if !ok {
				continue
			}

			executions = append(executions, Execution{
				Date:      bars[fillIndex].Date,
				FillPrice: fillPrice,
				Percent:   allocation,
			})
			result.Actions = append(result.Actions, Action{
				Date:          bars[fillIndex].Date.Format("2006-01-02"),
				TriggerDate:   bar.Date.Format("2006-01-02"),
				TriggerPrice:  triggerPriceForMatch(rule.Trigger, bars, referenceIndex, barIndex, referencePrice, lowestLow),
				TriggerReason: triggerReasonForMatch(rule.Trigger, bars, referenceIndex, barIndex, referencePrice, lowestLow),
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
		ReferencePrice:    referencePrice,
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

func referencePriceForBar(bar data.Bar, mode ReferencePriceMode) float64 {
	switch mode {
	case ReferencePriceOpen:
		return bar.Open
	case ReferencePriceHigh:
		return bar.High
	case ReferencePriceLow:
		return bar.Low
	default:
		return bar.Close
	}
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

func triggerPriceForMatch(trigger plan.Trigger, bars []data.Bar, referenceIndex, currentIndex int, referencePrice, lowestLow float64) *float64 {
	if len(trigger.AnyOf) > 0 {
		for _, child := range trigger.AnyOf {
			if triggerMatches(child, bars, referenceIndex, currentIndex, referencePrice, lowestLow) {
				return triggerPriceForMatch(child, bars, referenceIndex, currentIndex, referencePrice, lowestLow)
			}
		}
		return nil
	}

	bar := bars[currentIndex]
	switch {
	case trigger.DropPctFromReference != nil:
		return float64Ptr(bar.Low)
	case trigger.RisePctFromLowSinceRef != nil:
		return float64Ptr(bar.Close)
	case len(trigger.CloseAboveSMA) > 0:
		return float64Ptr(bar.Close)
	case trigger.TradingDaysSinceReference != nil:
		return nil
	default:
		return nil
	}
}

func triggerReasonForMatch(trigger plan.Trigger, bars []data.Bar, referenceIndex, currentIndex int, referencePrice, lowestLow float64) string {
	if len(trigger.AnyOf) > 0 {
		for _, child := range trigger.AnyOf {
			if triggerMatches(child, bars, referenceIndex, currentIndex, referencePrice, lowestLow) {
				return triggerReasonForMatch(child, bars, referenceIndex, currentIndex, referencePrice, lowestLow)
			}
		}
		return "matched any_of"
	}

	switch {
	case trigger.DropPctFromReference != nil:
		return fmt.Sprintf("drop %.2f%% from S", *trigger.DropPctFromReference)
	case trigger.TradingDaysSinceReference != nil:
		return fmt.Sprintf("%d trading days since S", *trigger.TradingDaysSinceReference)
	case trigger.RisePctFromLowSinceRef != nil:
		return fmt.Sprintf("rise %.2f%% from low since S", *trigger.RisePctFromLowSinceRef)
	case len(trigger.CloseAboveSMA) > 0:
		return fmt.Sprintf("close above SMA %v", trigger.CloseAboveSMA)
	default:
		return "rule trigger"
	}
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
	switch mode {
	case ExecutionPriceSameDayClose:
		return fmt.Sprintf("%s executed on %s close", rule.ID, triggerDay.Format("2006-01-02"))
	case ExecutionPriceNextDayOpen:
		return fmt.Sprintf("%s triggered on %s and filled on next open %s", rule.ID, triggerDay.Format("2006-01-02"), fillDay.Format("2006-01-02"))
	case ExecutionPriceRandomInDay:
		return fmt.Sprintf("%s executed at a simulated intraday price on %s", rule.ID, triggerDay.Format("2006-01-02"))
	case ExecutionPriceAverageOfDay:
		return fmt.Sprintf("%s executed at the OHLC average on %s", rule.ID, triggerDay.Format("2006-01-02"))
	default:
		return fmt.Sprintf("%s executed on %s", rule.ID, fillDay.Format("2006-01-02"))
	}
}

func resolveExecution(mode ExecutionPriceMode, bars []data.Bar, barIndex int, rule plan.EntryRule) (int, float64, bool) {
	bar := bars[barIndex]

	switch mode {
	case ExecutionPriceSameDayClose:
		return barIndex, bar.Close, true
	case ExecutionPriceNextDayOpen:
		if barIndex+1 >= len(bars) {
			return 0, 0, false
		}
		return barIndex + 1, bars[barIndex+1].Open, true
	case ExecutionPriceRandomInDay:
		return barIndex, intradayRandomPrice(bar, rule.ID), true
	case ExecutionPriceAverageOfDay:
		return barIndex, (bar.Open + bar.High + bar.Low + bar.Close) / 4, true
	default:
		return 0, 0, false
	}
}

func intradayRandomPrice(bar data.Bar, ruleID string) float64 {
	if bar.High <= bar.Low {
		return bar.Close
	}

	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(bar.Date.Format("2006-01-02")))
	_, _ = hasher.Write([]byte(":"))
	_, _ = hasher.Write([]byte(ruleID))

	fraction := float64(hasher.Sum64()%1000000) / 1000000.0
	return bar.Low + ((bar.High - bar.Low) * fraction)
}

func minFloat(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}

func float64Ptr(value float64) *float64 {
	return &value
}

func determineEndIndex(bars []data.Bar, fullInvestIndex int, referencePrice float64, exitRule plan.ExitRule) int {
	if fullInvestIndex < 0 {
		return len(bars) - 1
	}

	if exitRule.HoldDaysAfterFullInvest != nil && *exitRule.HoldDaysAfterFullInvest > 0 {
		return minInt(fullInvestIndex+*exitRule.HoldDaysAfterFullInvest, len(bars)-1)
	}

	for index := fullInvestIndex; index < len(bars); index++ {
		if bars[index].High > referencePrice {
			return index
		}
	}

	return len(bars) - 1
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func indexBarByDate(bars []data.Bar, target time.Time) int {
	for index, bar := range bars {
		if sameDay(bar.Date, target) {
			return index
		}
	}
	return -1
}

func sortRulesByPriority(rules []plan.EntryRule) {
	for i := 0; i < len(rules); i++ {
		for j := i + 1; j < len(rules); j++ {
			if rules[j].Priority > rules[i].Priority {
				rules[i], rules[j] = rules[j], rules[i]
			}
		}
	}
}
