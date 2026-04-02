package sim

import "time"

type ExecutionPriceMode string

const (
	ExecutionPriceSameDayClose ExecutionPriceMode = "same_day_close"
	ExecutionPriceNextDayOpen  ExecutionPriceMode = "next_day_open"
)

type Action struct {
	Date          string  `json:"date"`
	TriggerID     string  `json:"trigger_id"`
	Label         string  `json:"label"`
	ActionType    string  `json:"action_type"`
	AllocationPct float64 `json:"allocation_pct"`
	FillPrice     float64 `json:"fill_price"`
	Notes         string  `json:"notes"`
}

type Summary struct {
	ReferenceSellDate string             `json:"reference_sell_date"`
	FullInvestDate    string             `json:"full_invest_date,omitempty"`
	EndDate           string             `json:"end_date"`
	GainPct           float64            `json:"gain_pct"`
	TotalInvestedPct  float64            `json:"total_invested_pct"`
	ExecutionMode     ExecutionPriceMode `json:"execution_mode"`
}

type Stats struct {
	MaxDrawdownPct   float64 `json:"max_drawdown_pct"`
	BarsToFullInvest int     `json:"bars_to_full_invest"`
	BarsToEnd        int     `json:"bars_to_end"`
}

type Result struct {
	Summary Summary  `json:"summary"`
	Actions []Action `json:"actions"`
	Stats   Stats    `json:"stats"`
}

type BatchResult struct {
	Runs []Result `json:"runs"`
}

type Execution struct {
	Date      time.Time
	FillPrice float64
	Percent   float64
}
