// Package openui is the server-side OpenUI catalog: names, props, and
// inspect/mutate policy. It does not render pixels. Clients implement the
// same names in their own UI toolkit.
package openui

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	PolicyInspect = "inspect"
	PolicyMutate  = "mutate"

	// ChartModeData is a chart backed by server-owned data.  A client may
	// inspect it, but it must not invent or mutate query parameters.
	ChartModeData = "data"
	// ChartModeTeaching is a server-declared example chart whose parameters
	// may be edited and submitted by the client.
	ChartModeTeaching = "teaching"

	StateStreaming    = "streaming"
	StateComplete     = "complete"
	StateWaitingInput = "waiting_input"
)

// Spec is one allowed component.
type Spec struct {
	Type       string
	Policy     string
	Required   []string
	ArrayProps []string
}

var catalog = map[string]Spec{
	"Table":    {Type: "Table", Policy: PolicyInspect, Required: []string{"columns", "rows"}},
	"Chart":    {Type: "Chart", Policy: PolicyInspect, Required: []string{"kind", "series"}},
	"List":     {Type: "List", Policy: PolicyInspect, Required: []string{"items"}},
	"Actions":  {Type: "Actions", Policy: PolicyMutate, Required: []string{"items"}},
	"Image":    {Type: "Image", Policy: PolicyInspect, Required: []string{"src"}},
	"Svg":      {Type: "Svg", Policy: PolicyInspect, Required: []string{"markup"}},
	"Citation": {Type: "Citation", Policy: PolicyInspect, Required: []string{"source"}},
	"Map":      {Type: "Map", Policy: PolicyInspect, Required: []string{"points"}},
	"Markdown": {Type: "Markdown", Policy: PolicyInspect, Required: []string{"text"}},
}

// Lookup returns a catalog spec.
func Lookup(typeName string) (Spec, bool) {
	spec, ok := catalog[typeName]
	return spec, ok
}

// Node is one tree node before transport.
type Node struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Policy   string         `json:"policy,omitempty"`
	Props    map[string]any `json:"props,omitempty"`
	Children []string       `json:"children,omitempty"`
}

// ChartContract is the versioned part of a Chart node.  It deliberately lives
// in the shared catalog so the Agent, app and clients validate the same fields.
// Older charts may omit these fields; ContractForNode treats those as legacy
// charts and preserves the pre-#509 apply behaviour.
type ChartContract struct {
	ChartID       string
	SchemaVersion int
	DataVersion   int64
	Mode          string
	DataSource    string
	Editable      bool
}

// ContractForNode extracts the server-declared chart lifecycle contract.  An
// omitted contract remains backwards compatible and is marked as legacy by an
// empty Mode. Explicit data charts are always read-only, even if a malicious
// client sends editable=true.
func ContractForNode(node Node) (ChartContract, error) {
	if strings.TrimSpace(node.Type) != "Chart" {
		return ChartContract{}, fmt.Errorf("node %q is not a Chart", node.ID)
	}
	props := node.Props
	contract := ChartContract{
		ChartID:       strings.TrimSpace(node.ID),
		SchemaVersion: 1,
		DataVersion:   1,
		Editable:      true,
	}
	if props == nil {
		return contract, nil
	}
	if raw, ok := props["chart_id"]; ok {
		id, ok := raw.(string)
		if !ok || strings.TrimSpace(id) == "" {
			return ChartContract{}, fmt.Errorf("chart %q has invalid chart_id", node.ID)
		}
		contract.ChartID = strings.TrimSpace(id)
	}
	if raw, ok := props["schema_version"]; ok {
		version, ok := integerValue(raw)
		if !ok || version < 1 {
			return ChartContract{}, fmt.Errorf("chart %q has invalid schema_version", node.ID)
		}
		contract.SchemaVersion = version
	}
	if raw, ok := props["data_version"]; ok {
		version, ok := int64Value(raw)
		if !ok || version < 1 {
			return ChartContract{}, fmt.Errorf("chart %q has invalid data_version", node.ID)
		}
		contract.DataVersion = version
	}
	if raw, ok := props["mode"]; ok {
		mode, ok := raw.(string)
		mode = strings.ToLower(strings.TrimSpace(mode))
		if !ok || (mode != ChartModeData && mode != ChartModeTeaching) {
			return ChartContract{}, fmt.Errorf("chart %q has invalid mode", node.ID)
		}
		contract.Mode = mode
	}
	if raw, ok := props["data_source"]; ok {
		source, ok := raw.(string)
		if !ok || strings.TrimSpace(source) == "" {
			return ChartContract{}, fmt.Errorf("chart %q has invalid data_source", node.ID)
		}
		contract.DataSource = strings.TrimSpace(source)
	}
	if raw, ok := props["editable"]; ok {
		editable, ok := raw.(bool)
		if !ok {
			return ChartContract{}, fmt.Errorf("chart %q has invalid editable", node.ID)
		}
		contract.Editable = editable
	} else if contract.Mode == "" {
		// Missing mode is a pre-#509 chart. Keep the old apply path.
		contract.Editable = true
	} else {
		// A declared chart is editable only when the server says it is a
		// teaching chart. Data charts stay read-only.
		contract.Editable = contract.Mode == ChartModeTeaching
	}
	if contract.Mode == ChartModeData {
		contract.Editable = false
	}
	if contract.Mode == ChartModeData && contract.DataSource == "" {
		return ChartContract{}, fmt.Errorf("chart %q data mode requires data_source", node.ID)
	}
	if contract.Mode == ChartModeTeaching && !contract.Editable {
		return ChartContract{}, fmt.Errorf("chart %q teaching mode cannot be read-only", node.ID)
	}
	return contract, nil
}

func integerValue(value any) (int, bool) {
	switch n := value.(type) {
	case int:
		return n, true
	case int8:
		return int(n), true
	case int16:
		return int(n), true
	case int32:
		return int(n), true
	case int64:
		return int(n), int64(int(n)) == n
	case uint:
		return int(n), uint(int(n)) == n
	case uint8:
		return int(n), true
	case uint16:
		return int(n), true
	case uint32:
		return int(n), uint32(int(n)) == n
	case uint64:
		return int(n), uint64(int(n)) == n
	case float64:
		return int(n), n == float64(int(n))
	case json.Number:
		v, err := n.Int64()
		return int(v), err == nil && int64(int(v)) == v
	default:
		return 0, false
	}
}

func int64Value(value any) (int64, bool) {
	switch n := value.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case uint:
		return int64(n), uint(int64(n)) == n
	case uint64:
		return int64(n), uint64(int64(n)) == n
	case float64:
		return int64(n), n == float64(int64(n))
	case json.Number:
		v, err := n.Int64()
		return v, err == nil
	default:
		return 0, false
	}
}

// Delta is a catalog-validated patch.
type Delta struct {
	TreeID string `json:"tree_id"`
	State  string `json:"state"`
	Nodes  []Node `json:"nodes,omitempty"`
	Patch  bool   `json:"patch,omitempty"`
}

// ValidateDelta drops unknown components and nodes missing required props.
// An empty remaining tree is not an error: callers keep the Markdown path.
func ValidateDelta(raw json.RawMessage) (Delta, []string, error) {
	var delta Delta
	if len(raw) == 0 {
		return Delta{}, nil, fmt.Errorf("empty ui delta")
	}
	if err := json.Unmarshal(raw, &delta); err != nil {
		return Delta{}, nil, err
	}
	return FilterDelta(delta)
}

// FilterDelta keeps only catalog nodes.
func FilterDelta(delta Delta) (Delta, []string, error) {
	delta.TreeID = strings.TrimSpace(delta.TreeID)
	if delta.TreeID == "" {
		return Delta{}, nil, fmt.Errorf("ui delta missing tree_id")
	}
	switch delta.State {
	case StateStreaming, StateComplete, StateWaitingInput, "":
		if delta.State == "" {
			delta.State = StateStreaming
		}
	default:
		return Delta{}, nil, fmt.Errorf("unknown ui state %q", delta.State)
	}
	var dropped []string
	kept := make([]Node, 0, len(delta.Nodes))
	for _, node := range delta.Nodes {
		node.Type = strings.TrimSpace(node.Type)
		spec, ok := catalog[node.Type]
		if !ok {
			dropped = append(dropped, node.Type)
			continue
		}
		if strings.TrimSpace(node.ID) == "" {
			dropped = append(dropped, node.Type+"(no id)")
			continue
		}
		if !hasRequired(node.Props, spec.Required) {
			dropped = append(dropped, node.Type+"/"+node.ID)
			continue
		}
		if node.Type == "Chart" {
			if _, err := ContractForNode(node); err != nil {
				dropped = append(dropped, node.Type+"/"+node.ID)
				continue
			}
		}
		node.Policy = spec.Policy
		kept = append(kept, node)
	}
	delta.Nodes = kept
	return delta, dropped, nil
}

func hasRequired(props map[string]any, required []string) bool {
	if len(required) == 0 {
		return true
	}
	if props == nil {
		return false
	}
	for _, key := range required {
		value, ok := props[key]
		if !ok || value == nil {
			return false
		}
		if text, isText := value.(string); isText && strings.TrimSpace(text) == "" {
			return false
		}
	}
	return true
}

// Action is a client intent against one widget. It is not tool execution.
// Chart fields are optional so older clients keep working. When ChartID is
// set, the version and operation must be present; the owning service still
// decides whether that version is current.
type Action struct {
	TreeID        string         `json:"tree_id"`
	WidgetID      string         `json:"widget_id"`
	Values        map[string]any `json:"values,omitempty"`
	ActionID      string         `json:"action_id,omitempty"`
	ChartID       string         `json:"chart_id,omitempty"`
	SchemaVersion int            `json:"schema_version,omitempty"`
	DataVersion   int64          `json:"data_version,omitempty"`
	Operation     string         `json:"operation,omitempty"` // apply | observe
}

// ValidateAction checks identifiers only. Policy is applied by the Agent.
func ValidateAction(action Action) error {
	if strings.TrimSpace(action.TreeID) == "" {
		return fmt.Errorf("ui action missing tree_id")
	}
	if strings.TrimSpace(action.WidgetID) == "" {
		return fmt.Errorf("ui action missing widget_id")
	}
	action.ChartID = strings.TrimSpace(action.ChartID)
	action.Operation = strings.ToLower(strings.TrimSpace(action.Operation))
	if action.ChartID == "" {
		if action.SchemaVersion != 0 || action.DataVersion != 0 || action.Operation != "" {
			return fmt.Errorf("ui action chart fields require chart_id")
		}
		return nil
	}
	if action.SchemaVersion < 1 || action.DataVersion < 1 {
		return fmt.Errorf("ui action chart %q requires schema_version and data_version", action.ChartID)
	}
	switch action.Operation {
	case "apply", "observe":
	default:
		return fmt.Errorf("ui action chart %q has invalid operation", action.ChartID)
	}
	return nil
}
