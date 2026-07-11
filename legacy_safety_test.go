package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func readLegacySafetyFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "legacy", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func decodeLegacySafetyGraph(t *testing.T, data []byte) legacyGraph {
	t.Helper()
	var graph legacyGraph
	if err := json.Unmarshal(data, &graph); err != nil {
		t.Fatal(err)
	}
	return graph
}

func TestLegacyRoundTripPreservesUnmappedDefaultKeys(t *testing.T) {
	input := readLegacySafetyFixture(t, "residual-defaults.vgf")
	document, err := migrateLegacyGraph(input)
	if err != nil {
		t.Fatal(err)
	}
	output, err := exportLegacyGraph(document)
	if err != nil {
		t.Fatal(err)
	}

	want := decodeLegacySafetyGraph(t, input).Nodes[0].PortDefaults
	got := decodeLegacySafetyGraph(t, output).Nodes[0].PortDefaults
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("port_defaultv = %#v, want %#v", got, want)
	}
}

func TestExportLegacyGraphRejectsResidualDefaultsForDifferentClass(t *testing.T) {
	document := GraphDocument{
		Nodes: []GraphNode{{ID: "node", TypeID: "origin.math.add-integer", Properties: GraphNodeProperties{LegacyClass: "AddInt"}}},
		Legacy: &GraphLegacyState{ResidualNodeDefaults: map[string]GraphLegacyResidualDefaults{
			"node": {Class: "SubInt", Values: map[string]interface{}{"99": 7}},
		}},
	}
	_, err := exportLegacyGraph(document)
	if err == nil || !strings.Contains(err.Error(), "residual defaults") || !strings.Contains(err.Error(), "node") {
		t.Fatalf("error = %v, want residual class mismatch", err)
	}
}

func TestLegacyRoundTripPreservesVisibleHiddenEdgeOrder(t *testing.T) {
	input := readLegacySafetyFixture(t, "interleaved-hidden-edge.vgf")
	document, err := migrateLegacyGraph(input)
	if err != nil {
		t.Fatal(err)
	}
	output, err := exportLegacyGraph(document)
	if err != nil {
		t.Fatal(err)
	}
	graph := decodeLegacySafetyGraph(t, output)
	got := make([]string, 0, len(graph.Edges))
	for _, edge := range graph.Edges {
		got = append(got, edge.EdgeID)
	}
	want := []string{"visible-1", "hidden-1", "visible-2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("edge ids = %v, want %v", got, want)
	}
}

func TestExportLegacyGraphAppendsEdgesWithoutOrdinalsDeterministically(t *testing.T) {
	document := GraphDocument{
		Nodes: []GraphNode{
			{ID: "source", TypeID: "origin.math.add-integer"},
			{ID: "target", TypeID: "origin.math.add-integer"},
		},
		Connections: []GraphConnection{{Source: "source", SourceOutput: "result", Target: "target", TargetInput: "a", LegacyEdgeID: "visible"}},
		Legacy: &GraphLegacyState{
			HiddenNodes: []legacyNode{
				{ID: "hidden-source", Class: "UnknownSource", PortDefaults: map[string]interface{}{}},
				{ID: "hidden-target", Class: "UnknownTarget", PortDefaults: map[string]interface{}{}},
			},
			HiddenEdges: []legacyEdge{{EdgeID: "hidden", SourceNodeID: "hidden-source", TargetNodeID: "hidden-target"}},
		},
	}
	output, err := exportLegacyGraph(document)
	if err != nil {
		t.Fatal(err)
	}
	graph := decodeLegacySafetyGraph(t, output)
	got := []string{graph.Edges[0].EdgeID, graph.Edges[1].EdgeID}
	if want := []string{"visible", "hidden"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("edge ids = %v, want %v", got, want)
	}
}

func TestExportLegacyGraphRejectsUnrepresentableVisibleNode(t *testing.T) {
	_, err := exportLegacyGraph(GraphDocument{Nodes: []GraphNode{{ID: "unknown", TypeID: "origin.unknown"}}})
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("error = %v, want node id", err)
	}
}

func TestExportLegacyGraphRejectsUnrepresentableConnection(t *testing.T) {
	document := GraphDocument{
		Nodes: []GraphNode{
			{ID: "source", TypeID: "origin.math.add-integer"},
			{ID: "target", TypeID: "origin.math.add-integer"},
		},
		Connections: []GraphConnection{{Source: "source", SourceOutput: "missing", Target: "target", TargetInput: "a"}},
	}
	_, err := exportLegacyGraph(document)
	if err == nil || !strings.Contains(err.Error(), "connection 0") || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("error = %v, want connection index and port", err)
	}
}

func TestExportLegacyGraphRejectsDuplicateFinalNodeID(t *testing.T) {
	document := GraphDocument{
		Nodes: []GraphNode{{ID: "duplicate", TypeID: "origin.math.add-integer"}},
		Legacy: &GraphLegacyState{HiddenNodes: []legacyNode{{
			ID: "duplicate", Class: "UnknownSource", Module: "old.module", PortDefaults: map[string]interface{}{},
		}}},
	}
	_, err := exportLegacyGraph(document)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("error = %v, want duplicate node id", err)
	}
}

func TestExportLegacyGraphRejectsDanglingHiddenEdge(t *testing.T) {
	document := GraphDocument{Legacy: &GraphLegacyState{HiddenEdges: []legacyEdge{{
		EdgeID: "dangling", SourceNodeID: "missing-source", TargetNodeID: "missing-target",
	}}}}
	_, err := exportLegacyGraph(document)
	if err == nil || !strings.Contains(err.Error(), "dangling") {
		t.Fatalf("error = %v, want hidden edge id", err)
	}
}

func TestExportLegacyGraphRejectsMismatchedHiddenEdgeOrdinals(t *testing.T) {
	document := GraphDocument{Legacy: &GraphLegacyState{
		HiddenNodes: []legacyNode{
			{ID: "left", Class: "UnknownLeft", Module: "old.module", PortDefaults: map[string]interface{}{}},
			{ID: "right", Class: "UnknownRight", Module: "old.module", PortDefaults: map[string]interface{}{}},
		},
		HiddenEdges:        []legacyEdge{{EdgeID: "hidden", SourceNodeID: "left", TargetNodeID: "right"}},
		HiddenEdgeOrdinals: []int{0, 1},
	}}
	_, err := exportLegacyGraph(document)
	if err == nil || !strings.Contains(err.Error(), "ordinals length") {
		t.Fatalf("error = %v, want ordinal length mismatch", err)
	}
}

func TestExportLegacyGraphRejectsDuplicateEdgeOrdinal(t *testing.T) {
	ordinal := 0
	document := GraphDocument{
		Nodes: []GraphNode{
			{ID: "source", TypeID: "origin.math.add-integer"},
			{ID: "target", TypeID: "origin.math.add-integer"},
		},
		Connections: []GraphConnection{
			{Source: "source", SourceOutput: "result", Target: "target", TargetInput: "a", LegacyOrdinal: &ordinal},
			{Source: "source", SourceOutput: "result", Target: "target", TargetInput: "b", LegacyOrdinal: &ordinal},
		},
	}
	_, err := exportLegacyGraph(document)
	if err == nil || !strings.Contains(err.Error(), "ordinal 0") {
		t.Fatalf("error = %v, want duplicate ordinal", err)
	}
}

func TestExportLegacyGraphRejectsNegativeEdgeOrdinal(t *testing.T) {
	ordinal := -1
	document := GraphDocument{
		Nodes: []GraphNode{
			{ID: "source", TypeID: "origin.math.add-integer"},
			{ID: "target", TypeID: "origin.math.add-integer"},
		},
		Connections: []GraphConnection{{Source: "source", SourceOutput: "result", Target: "target", TargetInput: "a", LegacyOrdinal: &ordinal}},
	}
	_, err := exportLegacyGraph(document)
	if err == nil || !strings.Contains(err.Error(), "negative ordinal") {
		t.Fatalf("error = %v, want negative ordinal", err)
	}
}

func TestLegacySparsePortKeysUseDeclaredPortIDs(t *testing.T) {
	keys := []string{"in2", "in14"}
	if got, ok := legacyKeyIndex(keys, "in14", "in"); !ok || got != 14 {
		t.Fatalf("legacyKeyIndex(in14) = %d,%v, want 14,true", got, ok)
	}
	if _, ok := legacyKeyIndex(keys, "in999", "in"); ok {
		t.Fatalf("undeclared in999 should not be accepted")
	}
	if got := indexedKey(keys, 14, "in"); got != "in14" {
		t.Fatalf("indexedKey(14) = %q, want in14", got)
	}
	if got := indexedKey(keys, 0, "in"); got != "in0" {
		t.Fatalf("indexedKey(0) = %q, want in0", got)
	}
}

func TestMapLegacyNodeDefaultsUsesDeclaredSparsePortIDs(t *testing.T) {
	values, residual := mapLegacyNodeDefaults(
		map[string]interface{}{"14": float64(7), "99": "keep"},
		[]string{"in2", "in14"},
	)
	if !reflect.DeepEqual(values, map[string]interface{}{"in14": float64(7)}) {
		t.Fatalf("values = %#v, want sparse in14 value", values)
	}
	if !reflect.DeepEqual(residual, map[string]interface{}{"99": "keep"}) {
		t.Fatalf("residual = %#v, want unmapped default", residual)
	}
}

func TestExportLegacyGraphRejectsIncompatiblePortTypes(t *testing.T) {
	tests := []struct {
		name       string
		targetType string
		targetPort string
	}{
		{name: "integer to string", targetType: "origin.literal.string", targetPort: "value"},
		{name: "data to exec", targetType: "origin.action.print", targetPort: "exec"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := GraphDocument{
				Nodes: []GraphNode{
					{ID: "source", TypeID: "origin.math.add-integer"},
					{ID: "target", TypeID: test.targetType},
				},
				Connections: []GraphConnection{{Source: "source", SourceOutput: "result", Target: "target", TargetInput: test.targetPort}},
			}
			_, err := exportLegacyGraph(document)
			if err == nil || !strings.Contains(err.Error(), "incompatible") {
				t.Fatalf("error = %v, want incompatible port types", err)
			}
		})
	}
}

func TestExportLegacyGraphRejectsVariableTypeMismatch(t *testing.T) {
	document := GraphDocument{
		Variables: []GraphVariable{{ID: "score", Name: "Score", Type: "integer", DefaultValue: 0}},
		Nodes: []GraphNode{
			{ID: "source", TypeID: "origin.variable.get", Properties: GraphNodeProperties{VariableID: "score", VariableAccess: "get"}},
			{ID: "target", TypeID: "origin.literal.string"},
		},
		Connections: []GraphConnection{{Source: "source", SourceOutput: "value", Target: "target", TargetInput: "value"}},
	}
	_, err := exportLegacyGraph(document)
	if err == nil || !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("error = %v, want variable type mismatch", err)
	}
}

func TestLegacyAnyPortTypeIsCompatibleWithExec(t *testing.T) {
	if !legacyPortTypesCompatible("any", "exec") {
		t.Fatal("any output should be compatible with an exec input")
	}
	if !legacyPortTypesCompatible("exec", "any") {
		t.Fatal("exec output should be compatible with an any input")
	}
}

func TestLegacyRoundTripPreservesOriginalIncompatibleConnection(t *testing.T) {
	input := []byte(`{"graph_name":"Malformed Legacy Edge","nodes":[{"id":"source","class":"EqualInteger","port_defaultv":{}},{"id":"target","class":"ModInt","port_defaultv":{}}],"edges":[{"edge_id":"legacy-mismatch","source_node_id":"source","source_port_id":0,"des_node_id":"target","des_port_id":0}],"groups":[],"variables":[]}`)
	document, err := migrateLegacyGraph(input)
	if err != nil {
		t.Fatal(err)
	}
	output, err := exportLegacyGraph(document)
	if err != nil {
		t.Fatal(err)
	}
	graph := decodeLegacySafetyGraph(t, output)
	if len(graph.Edges) != 1 || graph.Edges[0].EdgeID != "legacy-mismatch" {
		t.Fatalf("edges = %#v, want original incompatible edge", graph.Edges)
	}
}

func TestLegacyRoundTripPreservesNodeWithMixedValidAndInvalidPortIDs(t *testing.T) {
	input := []byte(`{"graph_name":"Mixed Port IDs","nodes":[{"id":"source","class":"AddInt","port_defaultv":{}},{"id":"target","class":"AddInt","port_defaultv":{}}],"edges":[{"edge_id":"invalid","source_node_id":"source","source_port_id":0,"des_node_id":"target","des_port_id":-1},{"edge_id":"valid-max","source_node_id":"source","source_port_id":0,"des_node_id":"target","des_port_id":1}],"groups":[],"variables":[]}`)
	document, err := migrateLegacyGraph(input)
	if err != nil {
		t.Fatal(err)
	}
	output, err := exportLegacyGraph(document)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := decodeLegacySafetyGraph(t, output).Edges, decodeLegacySafetyGraph(t, input).Edges; !reflect.DeepEqual(got, want) {
		t.Fatalf("edges = %#v, want %#v", got, want)
	}
}

func TestLegacyRoundTripPreservesMissingEdgeID(t *testing.T) {
	input := []byte(`{"graph_name":"No Edge ID","nodes":[{"id":"left","class":"AddInt","port_defaultv":{}},{"id":"right","class":"AddInt","port_defaultv":{}}],"edges":[{"source_node_id":"left","source_port_id":0,"des_node_id":"right","des_port_id":0}],"groups":[],"variables":[]}`)
	document, err := migrateLegacyGraph(input)
	if err != nil {
		t.Fatal(err)
	}
	output, err := exportLegacyGraph(document)
	if err != nil {
		t.Fatal(err)
	}
	graph := decodeLegacySafetyGraph(t, output)
	if len(graph.Edges) != 1 || graph.Edges[0].EdgeID != "" {
		t.Fatalf("edge id = %q, want empty", graph.Edges[0].EdgeID)
	}
}

func TestPreferredLegacyExportClassesCoverDuplicateTypeIDs(t *testing.T) {
	want := map[string]string{
		"origin.flow.for-loop":       "Foreach",
		"origin.flow.branch":         "BoolIf",
		"origin.cast.integer-string": "Integer2String",
		"origin.math.add-integer":    "AddInt",
		"origin.array.length":        "GetArrayLen",
		"origin.array.create-string": "CreateStringArray",
		"origin.cast.any-string":     "Cast To",
	}
	for typeID, class := range want {
		if got := preferredLegacyExportClassByType[typeID]; got != class {
			t.Errorf("preferred class for %s = %q, want %q", typeID, got, class)
		}
	}
}
