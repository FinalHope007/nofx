package store

import (
	"encoding/json"
	"testing"
)

func TestStrategyConfigDecisionContextRoundTrip(t *testing.T) {
	cfg := GetDefaultStrategyConfig("en")
	cfg.DecisionContext = &DecisionContextConfig{
		Enabled:     true,
		RecentCount: 8,
		Mode:        "structured",
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out StrategyConfig
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.DecisionContext == nil {
		t.Fatalf("decision_context not round-tripped: %s", string(data))
	}
	dc := out.DecisionContext
	if !dc.Enabled || dc.RecentCount != 8 || dc.Mode != "structured" {
		t.Fatalf("decision_context = %+v", dc)
	}
}
