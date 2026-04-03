package plan

const DefaultQQQPlanYAML = `metadata:
  name: QQQ re-entry
  version: "1"
  symbol_scope: any
reference_price: sell_price
entry_rules:
  - id: first-entry
    label: First entry
    trigger:
      any_of:
        - drop_pct_from_reference: 2
        - trading_days_since_reference: 10
    action:
      type: buy_percent
      buy_percent: 20
    priority: 10
  - id: ladder-4
    label: Ladder 4%
    trigger:
      drop_pct_from_reference: 4
    action:
      type: buy_percent
      buy_percent: 15
  - id: ladder-7
    label: Ladder 7%
    trigger:
      drop_pct_from_reference: 7
    action:
      type: buy_percent
      buy_percent: 15
  - id: ladder-11
    label: Ladder 11%
    trigger:
      drop_pct_from_reference: 11
    action:
      type: buy_percent
      buy_percent: 15
  - id: ladder-15
    label: Ladder 15%
    trigger:
      drop_pct_from_reference: 15
    action:
      type: buy_percent
      buy_percent: 15
  - id: ladder-20
    label: Ladder 20%
    trigger:
      drop_pct_from_reference: 20
    action:
      type: buy_percent
      buy_percent: 20
  - id: rebound
    label: Rebound from low
    trigger:
      rise_pct_from_low_since_reference: 4
    action:
      type: buy_next_planned_allocation
  - id: strong-recovery
    label: Strong recovery
    trigger:
      close_above_sma: [20, 50]
    action:
      type: invest_remaining
constraints:
  max_actions_per_day: 1
  prevent_duplicate_level_buys: true
exit: {}
`
