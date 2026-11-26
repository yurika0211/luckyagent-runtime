package openui

import (
	"encoding/json"
	"testing"
)

func TestFilterDeltaKeepsCatalogAndDropsUnknown(t *testing.T) {
	raw, _ := json.Marshal(Delta{
		TreeID: "t1",
		State:  StateStreaming,
		Nodes: []Node{
			{ID: "a", Type: "Table", Props: map[string]any{"columns": []string{"n"}, "rows": []any{[]any{"1"}}}},
			{ID: "b", Type: "Carousel", Props: map[string]any{"items": []any{}}},
			{ID: "c", Type: "Chart"},
		},
	})
	delta, dropped, err := ValidateDelta(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(delta.Nodes) != 1 || delta.Nodes[0].Type != "Table" {
		t.Fatalf("kept %#v", delta.Nodes)
	}
	if delta.Nodes[0].Policy != PolicyInspect {
		t.Fatalf("policy %s", delta.Nodes[0].Policy)
	}
	if len(dropped) != 2 {
		t.Fatalf("dropped %#v", dropped)
	}
}

func TestFilterDeltaRejectsEmptyTreeID(t *testing.T) {
	_, _, err := ValidateDelta([]byte(`{"nodes":[]}`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestActionsAreMutate(t *testing.T) {
	delta, _, err := FilterDelta(Delta{
		TreeID: "t1",
		Nodes: []Node{{
			ID: "go", Type: "Actions",
			Props: map[string]any{"items": []any{map[string]any{"id": "retry", "label": "重试"}}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if delta.Nodes[0].Policy != PolicyMutate {
		t.Fatalf("got %s", delta.Nodes[0].Policy)
	}
}

func TestValidateAction(t *testing.T) {
	if err := ValidateAction(Action{TreeID: "t", WidgetID: "w"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAction(Action{TreeID: "t"}); err == nil {
		t.Fatal("expected missing widget")
	}
}

func TestChartContractDefaultsLegacyAndRejectsInvalidDeclarations(t *testing.T) {
	legacy, err := ContractForNode(Node{ID: "chart", Type: "Chart"})
	if err != nil || legacy.ChartID != "chart" || !legacy.Editable || legacy.Mode != "" {
		t.Fatalf("legacy contract=%#v err=%v", legacy, err)
	}
	data, err := ContractForNode(Node{ID: "chart-node", Type: "Chart", Props: map[string]any{
		"chart_id": "chart-1", "schema_version": float64(2), "data_version": float64(7),
		"mode": "data", "data_source": "metrics.api", "editable": true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if data.ChartID != "chart-1" || data.SchemaVersion != 2 || data.DataVersion != 7 || data.Mode != ChartModeData || data.Editable || data.DataSource != "metrics.api" {
		t.Fatalf("data contract=%#v", data)
	}
	teaching, err := ContractForNode(Node{ID: "lesson", Type: "Chart", Props: map[string]any{"mode": ChartModeTeaching}})
	if err != nil || !teaching.Editable || teaching.Mode != ChartModeTeaching {
		t.Fatalf("teaching contract=%#v err=%v", teaching, err)
	}
	if _, err := ContractForNode(Node{ID: "bad", Type: "Chart", Props: map[string]any{"mode": "other"}}); err == nil {
		t.Fatal("invalid chart mode was accepted")
	}
	if _, err := ContractForNode(Node{ID: "nodata", Type: "Chart", Props: map[string]any{"mode": ChartModeData}}); err == nil {
		t.Fatal("data chart without data_source was accepted")
	}
	if _, err := ContractForNode(Node{ID: "locked-lesson", Type: "Chart", Props: map[string]any{"mode": ChartModeTeaching, "editable": false}}); err == nil {
		t.Fatal("read-only teaching chart was accepted")
	}
}

func TestValidateChartActionRequiresVersion(t *testing.T) {
	if err := ValidateAction(Action{TreeID: "t", WidgetID: "w"}); err != nil {
		t.Fatal(err)
	}
	err := ValidateAction(Action{TreeID: "t", WidgetID: "w", ChartID: "c1", Operation: "apply"})
	if err == nil {
		t.Fatal("chart action without versions was accepted")
	}
	if err := ValidateAction(Action{
		TreeID: "t", WidgetID: "w", ChartID: "c1",
		SchemaVersion: 1, DataVersion: 3, Operation: "observe",
	}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAction(Action{TreeID: "t", WidgetID: "w", Operation: "apply"}); err == nil {
		t.Fatal("operation without chart_id was accepted")
	}
}

func TestFilterDeltaDropsInvalidChartContract(t *testing.T) {
	delta, dropped, err := FilterDelta(Delta{TreeID: "t", State: StateComplete, Nodes: []Node{
		{ID: "ok", Type: "Chart", Props: map[string]any{
			"kind": "line", "series": []any{}, "mode": ChartModeTeaching,
		}},
		{ID: "bad", Type: "Chart", Props: map[string]any{
			"kind": "line", "series": []any{}, "schema_version": "two",
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(delta.Nodes) != 1 || delta.Nodes[0].ID != "ok" {
		t.Fatalf("nodes=%#v", delta.Nodes)
	}
	if len(dropped) != 1 || dropped[0] != "Chart/bad" {
		t.Fatalf("dropped=%#v", dropped)
	}
}
