package plan_test

import (
	"strings"
	"testing"

	"stock-sim/internal/plan"
)

func TestValidateDefaultPlan(t *testing.T) {
	p := plan.MustParse(plan.DefaultQQQPlanYAML)
	result := plan.Validate(p, 500)
	if !result.Valid {
		t.Fatalf("expected valid plan, got errors: %v", result.Errors)
	}
}

func TestValidateRejectsOverAllocation(t *testing.T) {
	raw := strings.Replace(plan.DefaultQQQPlanYAML, "buy_percent: 20", "buy_percent: 55", 1)
	p := plan.MustParse(raw)
	result := plan.Validate(p, 500)
	if result.Valid {
		t.Fatal("expected validation failure")
	}
}

func TestValidateRejectsDuplicateIDs(t *testing.T) {
	raw := strings.Replace(plan.DefaultQQQPlanYAML, "id: ladder-7", "id: ladder-4", 1)
	p := plan.MustParse(raw)
	result := plan.Validate(p, 500)
	if result.Valid {
		t.Fatal("expected duplicate id validation failure")
	}
}

func TestValidateRejectsInvalidIndicator(t *testing.T) {
	raw := strings.Replace(plan.DefaultQQQPlanYAML, "close_above_sma: [20, 50]", "close_above_sma: [1, 50]", 1)
	p := plan.MustParse(raw)
	result := plan.Validate(p, 500)
	if result.Valid {
		t.Fatal("expected invalid SMA validation failure")
	}
}
