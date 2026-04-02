package plan

type StrategyPlan struct {
	Metadata       Metadata    `json:"metadata" yaml:"metadata"`
	ReferencePrice string      `json:"reference_price" yaml:"reference_price"`
	EntryRules     []EntryRule `json:"entry_rules" yaml:"entry_rules"`
	Constraints    Constraints `json:"constraints" yaml:"constraints"`
	Exit           ExitRule    `json:"exit" yaml:"exit"`
}

type Metadata struct {
	Name        string `json:"name" yaml:"name"`
	Version     string `json:"version" yaml:"version"`
	SymbolScope string `json:"symbol_scope" yaml:"symbol_scope"`
}

type EntryRule struct {
	ID       string  `json:"id" yaml:"id"`
	Label    string  `json:"label" yaml:"label"`
	Trigger  Trigger `json:"trigger" yaml:"trigger"`
	Action   Action  `json:"action" yaml:"action"`
	Priority int     `json:"priority,omitempty" yaml:"priority,omitempty"`
}

type Trigger struct {
	DropPctFromReference      *float64  `json:"drop_pct_from_reference,omitempty" yaml:"drop_pct_from_reference,omitempty"`
	TradingDaysSinceReference *int      `json:"trading_days_since_reference,omitempty" yaml:"trading_days_since_reference,omitempty"`
	RisePctFromLowSinceRef    *float64  `json:"rise_pct_from_low_since_reference,omitempty" yaml:"rise_pct_from_low_since_reference,omitempty"`
	CloseAboveSMA             []int     `json:"close_above_sma,omitempty" yaml:"close_above_sma,omitempty"`
	AnyOf                     []Trigger `json:"any_of,omitempty" yaml:"any_of,omitempty"`
}

type Action struct {
	Type       string   `json:"type" yaml:"type"`
	BuyPercent *float64 `json:"buy_percent,omitempty" yaml:"buy_percent,omitempty"`
}

type Constraints struct {
	MaxActionsPerDay          int  `json:"max_actions_per_day" yaml:"max_actions_per_day"`
	PreventDuplicateLevelBuys bool `json:"prevent_duplicate_level_buys" yaml:"prevent_duplicate_level_buys"`
}

type ExitRule struct {
	HoldDaysAfterFullInvest *int `json:"hold_days_after_full_invest,omitempty" yaml:"hold_days_after_full_invest,omitempty"`
}

type ValidationResult struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}
