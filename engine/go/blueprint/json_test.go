package blueprint

import (
	"path/filepath"
	"testing"
)

func TestSchemaVersionValidationIsSharedByJSONAndFileParsers(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantErr bool
	}{
		{name: "missing legacy", version: ""},
		{name: "supported", version: `"schemaVersion":1,`},
		{name: "zero", version: `"schemaVersion":0,`, wantErr: true},
		{name: "future", version: `"schemaVersion":2,`, wantErr: true},
		{name: "fraction", version: `"schemaVersion":1.5,`, wantErr: true},
		{name: "string", version: `"schemaVersion":"1",`, wantErr: true},
		{name: "null", version: `"schemaVersion":null,`, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := []byte(`{` + test.version + `"graphName":"Version Test","nodes":[],"edges":[],"variables":[]}`)
			_, jsonErr := ParseGraphConfigJSON(data)
			_, _, _, _, fileErr := parseGraphFile(data, t.TempDir(), filepath.Join(t.TempDir(), "test.obp"))
			if (jsonErr != nil) != test.wantErr {
				t.Fatalf("ParseGraphConfigJSON error = %v, wantErr %v", jsonErr, test.wantErr)
			}
			if (fileErr != nil) != test.wantErr {
				t.Fatalf("parseGraphFile error = %v, wantErr %v", fileErr, test.wantErr)
			}
		})
	}
}

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
