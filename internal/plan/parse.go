package plan

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

func Parse(raw string) (StrategyPlan, error) {
	var p StrategyPlan
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return p, fmt.Errorf("plan is empty")
	}

	if strings.HasPrefix(trimmed, "{") {
		if err := json.Unmarshal([]byte(trimmed), &p); err != nil {
			return p, fmt.Errorf("parse json plan: %w", err)
		}
		return p, nil
	}

	if err := yaml.Unmarshal([]byte(raw), &p); err != nil {
		return p, fmt.Errorf("parse yaml plan: %w", err)
	}
	return p, nil
}

func MustParse(raw string) StrategyPlan {
	p, err := Parse(raw)
	if err != nil {
		panic(err)
	}
	return p
}
