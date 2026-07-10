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
