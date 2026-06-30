package golang

import "testing"

func TestParseGraphConfigJSONUsesLegacyFieldNames(t *testing.T) {
	config, err := ParseGraphConfigJSON([]byte(`{
		"nodes": [
			{"id":"entrance","class":"TestEntrance_1"},
			{"id":"record","class":"TestRecorder","port_defaultv":{"1":9}}
		],
		"edges": [
			{"source_node_id":"entrance","des_node_id":"record","source_port_id":0,"des_port_id":0}
		]
	}`))
	if err != nil {
		t.Fatalf("ParseGraphConfigJSON failed: %v", err)
	}
	if len(config.Nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(config.Nodes))
	}
	if config.Nodes[1].PortDefault[1] != float64(9) {
		t.Fatalf("port default = %#v, want 9", config.Nodes[1].PortDefault[1])
	}
	if len(config.Edges) != 1 || config.Edges[0].SourceNodeID != "entrance" || config.Edges[0].DesNodeID != "record" {
		t.Fatalf("edges = %#v", config.Edges)
	}
}
