package blueprint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepresentativeLegacyVGFFilesAreHandledByGoLoader(t *testing.T) {
	fixtures := []string{
		filepath.Join("testdata", "legacy", "sequence-return.vgf"),
		filepath.Join("testdata", "legacy", "foreach-int-array.vgf"),
		filepath.Join("testdata", "legacy", "business-stub.vgf"),
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(filepath.ToSlash(fixture), func(t *testing.T) {
			data, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatalf("ReadFile failed: %v", err)
			}
			config, err := ParseGraphConfigJSON(data)
			if err != nil {
				t.Fatalf("ParseGraphConfigJSON failed: %v", err)
			}

			registry := testSystemRegistry(t)
			registerLegacyCompatStubs(t, registry, config)

			root := t.TempDir()
			target := filepath.Join(root, filepath.Base(fixture))
			if err := os.WriteFile(target, data, 0644); err != nil {
				t.Fatalf("WriteFile failed: %v", err)
			}
			graphs, err := loadGraphDir(registry, root)
			if err != nil {
				t.Fatalf("loadGraphDir failed: %v", err)
			}

			graphName := strings.TrimSuffix(filepath.Base(fixture), filepath.Ext(fixture))
			compiled := graphs[graphName]
			if compiled == nil {
				t.Fatalf("compiled graph %s not found in %#v", graphName, graphs)
			}
			entranceID, ok := firstLegacyEntranceID(config)
			if !ok {
				t.Fatalf("legacy graph has no entrance node")
			}
			if _, err := NewGraph(compiled).Do(entranceID); err != nil {
				t.Fatalf("Do entrance %d failed: %v", entranceID, err)
			}
		})
	}
}

type legacyCompatStub struct {
	BaseExecNode
	name string
}

type legacyCompatPortShape struct {
	inputs  map[int]IPort
	outputs map[int]IPort
}

func (n *legacyCompatStub) GetName() string {
	return n.name
}

func (n *legacyCompatStub) Exec() (int, error) {
	return 0, nil
}

func registerLegacyCompatStubs(t *testing.T, registry *Registry, config GraphConfig) {
	t.Helper()
	shapes := map[string]*legacyCompatPortShape{}
	nodeClass := map[string]string{}
	for _, node := range config.Nodes {
		class, _, _ := parseEntranceClass(node.Class)
		nodeClass[node.ID] = class
		if registry.Get(class) != nil {
			continue
		}
		shape := ensureLegacyCompatShape(shapes, class)
		for key, value := range node.RawDefault {
			var portID int
			if err := json.Unmarshal([]byte(key), &portID); err != nil {
				continue
			}
			shape.inputs[portID] = legacyCompatPortForDefault(value)
		}
	}

	for _, edge := range config.Edges {
		sourceClass := nodeClass[edge.SourceNodeID]
		destClass := nodeClass[edge.DesNodeID]
		sourceKnown := registry.Get(sourceClass)
		destKnown := registry.Get(destClass)
		sourceExec := legacyCompatKnownOutputIsExec(sourceKnown, edge.SourcePortID)
		destExec := legacyCompatKnownInputIsExec(destKnown, edge.DesPortID)
		if sourceKnown == nil {
			sourceExec = destExec && edge.SourcePortID == 0
		}
		if sourceKnown == nil && destKnown == nil {
			destExec = edge.DesPortID == 0
			sourceExec = destExec && edge.SourcePortID == 0
		}

		if sourceKnown == nil {
			shape := ensureLegacyCompatShape(shapes, sourceClass)
			if destKnown != nil && edge.DesPortID >= 0 && edge.DesPortID < len(destKnown.InPorts) && destKnown.InPorts[edge.DesPortID] != nil && !destKnown.InPorts[edge.DesPortID].IsPortExec() {
				shape.outputs[edge.SourcePortID] = destKnown.InPorts[edge.DesPortID].Clone()
			} else if sourceExec {
				shape.outputs[edge.SourcePortID] = NewPortExec()
			} else {
				shape.outputs[edge.SourcePortID] = NewPortInt()
			}
		}
		if destKnown == nil {
			shape := ensureLegacyCompatShape(shapes, destClass)
			if sourceKnown != nil && edge.SourcePortID >= 0 && edge.SourcePortID < len(sourceKnown.OutPorts) && sourceKnown.OutPorts[edge.SourcePortID] != nil && !sourceKnown.OutPorts[edge.SourcePortID].IsPortExec() {
				shape.inputs[edge.DesPortID] = sourceKnown.OutPorts[edge.SourcePortID].Clone()
			} else if destExec {
				shape.inputs[edge.DesPortID] = NewPortExec()
			} else {
				shape.inputs[edge.DesPortID] = NewPortInt()
			}
		}
	}

	for class, shape := range shapes {
		class := class
		if registry.Get(class) != nil {
			continue
		}
		inputs := compactLegacyCompatPorts(shape.inputs, false)
		outputs := compactLegacyCompatPorts(shape.outputs, strings.HasPrefix(class, "Entrance_"))
		if !registry.Register(NewNodeDefinition(class, func() IExecNode { return &legacyCompatStub{name: class} }, inputs, outputs)) {
			t.Fatalf("failed to register legacy stub %s", class)
		}
	}
}

func ensureLegacyCompatShape(shapes map[string]*legacyCompatPortShape, class string) *legacyCompatPortShape {
	shape := shapes[class]
	if shape == nil {
		shape = &legacyCompatPortShape{inputs: map[int]IPort{}, outputs: map[int]IPort{}}
		shapes[class] = shape
	}
	return shape
}

func legacyCompatKnownOutputIsExec(definition *NodeDefinition, portID int) bool {
	return definition != nil && portID >= 0 && portID < len(definition.OutPorts) && definition.OutPorts[portID] != nil && definition.OutPorts[portID].IsPortExec()
}

func legacyCompatKnownInputIsExec(definition *NodeDefinition, portID int) bool {
	return definition != nil && portID >= 0 && portID < len(definition.InPorts) && definition.InPorts[portID] != nil && definition.InPorts[portID].IsPortExec()
}

func legacyCompatPortForDefault(value any) IPort {
	switch value.(type) {
	case []any, []int, []int64, PortArray:
		return NewPortArray()
	case bool:
		return NewPortBool()
	case string:
		return NewPortStr()
	default:
		return NewPortInt()
	}
}

func compactLegacyCompatPorts(ports map[int]IPort, entrance bool) []IPort {
	maxPort := -1
	for portID := range ports {
		if portID > maxPort {
			maxPort = portID
		}
	}
	if entrance && maxPort < 0 {
		maxPort = 0
	}
	if maxPort < 0 {
		return nil
	}
	result := make([]IPort, maxPort+1)
	for index := range result {
		if port := ports[index]; port != nil {
			result[index] = port
			continue
		}
		if index == 0 && entrance {
			result[index] = NewPortExec()
		} else if index == 0 && !entrance {
			result[index] = NewPortExec()
		} else {
			result[index] = NewPortInt()
		}
	}
	return result
}

func firstLegacyEntranceID(config GraphConfig) (int64, bool) {
	for _, node := range config.Nodes {
		if _, entranceID, ok := parseEntranceClass(node.Class); ok {
			return entranceID, true
		}
	}
	return 0, false
}
