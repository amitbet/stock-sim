package plan

import "fmt"

func Validate(p StrategyPlan, availableBars int) ValidationResult {
	result := ValidationResult{Valid: true}

	if p.Metadata.Name == "" {
		result.Errors = append(result.Errors, "metadata.name is required")
	}
	if p.ReferencePrice != "sell_price" {
		result.Errors = append(result.Errors, "reference_price must be sell_price")
	}
	if p.Constraints.MaxActionsPerDay <= 0 {
		result.Errors = append(result.Errors, "constraints.max_actions_per_day must be positive")
	}
	if p.Exit.HoldDaysAfterFullInvest != nil && *p.Exit.HoldDaysAfterFullInvest < 0 {
		result.Errors = append(result.Errors, "exit.hold_days_after_full_invest must be zero or positive")
	}
	if len(p.EntryRules) == 0 {
		result.Errors = append(result.Errors, "at least one entry rule is required")
	}

	ids := map[string]struct{}{}
	totalPercent := 0.0
	maxSMA := 0

	for index, rule := range p.EntryRules {
		if rule.ID == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("entry_rules[%d].id is required", index))
		}
		if _, exists := ids[rule.ID]; exists {
			result.Errors = append(result.Errors, fmt.Sprintf("duplicate rule id: %s", rule.ID))
		}
		ids[rule.ID] = struct{}{}

		switch rule.Action.Type {
		case "buy_percent":
			if rule.Action.BuyPercent == nil {
				result.Errors = append(result.Errors, fmt.Sprintf("rule %s requires action.buy_percent", rule.ID))
			} else {
				totalPercent += *rule.Action.BuyPercent
			}
		case "buy_next_planned_allocation", "invest_remaining":
		default:
			result.Errors = append(result.Errors, fmt.Sprintf("rule %s uses unsupported action type %q", rule.ID, rule.Action.Type))
		}

		maxSMA = max(maxSMA, maxPeriod(rule.Trigger))
		validateTrigger(rule.ID, rule.Trigger, &result)
	}

	if totalPercent > 100.000001 {
		result.Errors = append(result.Errors, fmt.Sprintf("fixed buy_percent allocations exceed 100%%: %.2f%%", totalPercent))
	}
	if maxSMA > 0 && availableBars > 0 && availableBars < maxSMA {
		result.Errors = append(result.Errors, fmt.Sprintf("plan requires at least %d bars to evaluate SMA rules, only %d available", maxSMA, availableBars))
	}

	result.Valid = len(result.Errors) == 0
	return result
}

func validateTrigger(ruleID string, trigger Trigger, result *ValidationResult) {
	count := 0
	if trigger.DropPctFromReference != nil {
		count++
	}
	if trigger.TradingDaysSinceReference != nil {
		count++
	}
	if trigger.RisePctFromLowSinceRef != nil {
		count++
	}
	if len(trigger.CloseAboveSMA) > 0 {
		count++
		for _, period := range trigger.CloseAboveSMA {
			if period <= 1 {
				result.Errors = append(result.Errors, fmt.Sprintf("rule %s close_above_sma periods must be > 1", ruleID))
			}
		}
	}
	if len(trigger.AnyOf) > 0 {
		count++
		for _, child := range trigger.AnyOf {
			validateTrigger(ruleID, child, result)
		}
	}

	if count == 0 {
		result.Errors = append(result.Errors, fmt.Sprintf("rule %s has no trigger", ruleID))
	}
}

func maxPeriod(trigger Trigger) int {
	best := 0
	for _, period := range trigger.CloseAboveSMA {
		if period > best {
			best = period
		}
	}
	for _, child := range trigger.AnyOf {
		if childBest := maxPeriod(child); childBest > best {
			best = childBest
		}
	}
	return best
}
