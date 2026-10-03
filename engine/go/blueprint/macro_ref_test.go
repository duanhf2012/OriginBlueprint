package blueprint

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// 宏引用（macroRefs）编译期内联测试：引用图只保存 macroId + 边界连线，
// 宏内容以 .obpm 源文件为准，由 loadGraphDir 在编译前展开。
// 文档形态与编辑器 v2 引用模型一致（frame 为编辑器元数据，引擎忽略）。

// macroRefEntranceNode 生成一个 legacy 入口节点（class 决定入口身份与编号）。
func macroRefEntranceNode(id, class string) string {
	return fmt.Sprintf(`{"id":%q,"typeId":"legacy.entry","values":{},"properties":{"legacyClass":%q,"legacyOutputs":[{"key":"exec","type":"exec"}]}}`, id, class)
}

func macroRefRecorderNode(id string) string {
	return fmt.Sprintf(`{"id":%q,"typeId":"legacy.record","values":{},"properties":{"legacyClass":"TestRecorder","legacyInputs":[{"key":"exec","type":"exec"},{"key":"integer","type":"int"}]}}`, id)
}

// macroRefAddNode 生成 AddInt 节点：result = a + b。
func macroRefAddNode(id string, a int) string {
	return fmt.Sprintf(`{"id":%q,"typeId":"origin.math.add-integer","values":{"a":%d,"b":1}}`, id, a)
}

// macroRefDocument 拼装一份带宏引用的引用图文档。
func macroRefDocument(macroID string, boundary string) string {
	boundaryField := ""
	if boundary != "" {
		boundaryField = fmt.Sprintf(`,"boundary":[%s]`, boundary)
	}
	return fmt.Sprintf(`{
		"schemaVersion":1,
		"graphName":"Host",
		"nodes":[%s],
		"connections":[],
		"variables":[],
		"macroRefs":[{"macroId":%q,
			"frame":{"x":0,"y":0,"width":400,"height":300}%s}]
	}`, macroRefEntranceNode("ext", "Entrance_IntParam_1"), macroID, boundaryField)
}

// macroRefMacroDocument 拼装宏源文档：入口(entryClass) → record ← add(a)。
func macroRefMacroDocument(macroID, entryClass string, addValue int, extraNodes, extraConnections string) string {
	nodes := fmt.Sprintf(`%s,%s,%s`, macroRefEntranceNode("entry", entryClass), macroRefAddNode("add", addValue), macroRefRecorderNode("record"))
	if extraNodes != "" {
		nodes = nodes + "," + extraNodes
	}
	connections := `{"source":"entry","sourceOutput":"exec","target":"record","targetInput":"exec"},
		{"source":"add","sourceOutput":"result","target":"record","targetInput":"integer"}`
	if extraConnections != "" {
		connections = connections + "," + extraConnections
	}
	return fmt.Sprintf(`{
		"schemaVersion":1,
		"graphName":"宏_加成",
		"macroId":%q,
		"nodes":[%s],
		"connections":[%s],
		"variables":[]
	}`, macroID, nodes, connections)
}

// writeMacroRefWorkspace 写入宏源 .obpm 与引用图 .obp，返回图目录。
// record 在宏文档 nodes 数组中的下标（boundary 的 macroNodeIndex 引用它）。
const macroRefRecordIndex = 2

func writeMacroRefWorkspace(t *testing.T, macroID, hostDocument, macroDocument string) string {
	t.Helper()
	graphDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(graphDir, "宏_加成.obpm"), []byte(macroDocument), 0644); err != nil {
		t.Fatalf("write macro failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(graphDir, "host.obp"), []byte(hostDocument), 0644); err != nil {
		t.Fatalf("write host failed: %v", err)
	}
	return graphDir
}

func macroRefBoundary(intoMacro bool) string {
	return fmt.Sprintf(`{"externalNode":"ext","externalPort":"exec","macroNodeIndex":%d,"macroPort":"exec","intoMacro":%t}`, macroRefRecordIndex, intoMacro)
}

// TestLoadGraphDirInlinesMacroRefs 验证编译期内联：
// 宏入口（entrance 2）可被外部触发执行宏逻辑；外部入口（entrance 1）经边界连线驱动同一逻辑。
func TestLoadGraphDirInlinesMacroRefs(t *testing.T) {
	var recorder *testRecorder
	registry := NewRegistry()
	registerFunctionTestNodes(registry, func() IExecNode { recorder = &testRecorder{}; return recorder })

	host := macroRefDocument("m1", macroRefBoundary(true))
	macro := macroRefMacroDocument("m1", "Entrance_IntParam_2", 31, "", "")
	graphs, err := loadGraphDir(registry, writeMacroRefWorkspace(t, "m1", host, macro))
	if err != nil {
		t.Fatalf("loadGraphDir failed: %v", err)
	}
	hostGraph, ok := graphs["host"]
	if !ok {
		t.Fatalf("host graph missing; graphs = %#v", graphs)
	}
	if len(hostGraph.Variables) != 0 || hostGraph.NodeCount != 4 {
		t.Fatalf("inlined host node count = %d, want 4 (ext + 宏入口 + add + record)", hostGraph.NodeCount)
	}

	if _, err := NewGraph(hostGraph).Do(2); err != nil {
		t.Fatalf("Do(macro entrance) failed: %v", err)
	}
	if values := recorder.snapshot(); len(values) != 1 || values[0] != 32 {
		t.Fatalf("macro entrance recorder values = %#v, want [32]", values)
	}

	recorder = nil
	if _, err := NewGraph(hostGraph).Do(1); err != nil {
		t.Fatalf("Do(external entrance) failed: %v", err)
	}
	if values := recorder.snapshot(); len(values) != 1 || values[0] != 32 {
		t.Fatalf("external entrance recorder values = %#v, want [32]（边界连线必须驱动宏内节点）", values)
	}
}

// TestMacroRefHotReloadTakesEffect 热更验证：只改 .obpm 宏源文件并 HotReload，
// 已有实例的引用图执行立即采用新宏内容（引用不落盘内容，天然热更）。
func TestMacroRefHotReloadTakesEffect(t *testing.T) {
	root := t.TempDir()
	execDir := filepath.Join(root, "json")
	graphDir := filepath.Join(root, "vgf")
	if err := os.MkdirAll(execDir, 0755); err != nil {
		t.Fatalf("Mkdir execDir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(execDir, "nodes.json"), []byte(`[
		{"name":"Entrance_IntParam","inputs":[],"outputs":[{"type":"exec","port_id":0}]},
		{"name":"AddInt","inputs":[{"type":"data","data_type":"int","port_id":0},{"type":"data","data_type":"int","port_id":1}],"outputs":[{"type":"data","data_type":"int","port_id":0}]},
		{"name":"TestRecorder","inputs":[{"type":"exec","port_id":0},{"type":"data","data_type":"int","port_id":1}],"outputs":[]}
	]`), 0644); err != nil {
		t.Fatalf("write nodes.json failed: %v", err)
	}

	var recorder *testRecorder
	writeWorkspace := func(addValue int) {
		t.Helper()
		if err := os.MkdirAll(graphDir, 0755); err != nil {
			t.Fatalf("Mkdir graphDir failed: %v", err)
		}
		if err := os.WriteFile(filepath.Join(graphDir, "宏_加成.obpm"), []byte(macroRefMacroDocument("m1", "Entrance_IntParam_2", addValue, "", "")), 0644); err != nil {
			t.Fatalf("write macro failed: %v", err)
		}
		if err := os.WriteFile(filepath.Join(graphDir, "host.obp"), []byte(macroRefDocument("m1", macroRefBoundary(true))), 0644); err != nil {
			t.Fatalf("write host failed: %v", err)
		}
	}
	writeWorkspace(31)

	var bp Blueprint
	bp.RegisterExecNode(func() IExecNode { return &EntranceIntParam{} })
	bp.RegisterExecNode(func() IExecNode { return &AddInt{} })
	bp.RegisterExecNode(func() IExecNode { recorder = &testRecorder{}; return recorder })
	if err := bp.Init(execDir, graphDir, nil); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	graphID := bp.Create("host")
	if _, err := bp.Do(graphID, 2); err != nil {
		t.Fatalf("first Do failed: %v", err)
	}
	if values := recorder.snapshot(); len(values) != 1 || values[0] != 32 {
		t.Fatalf("first recorder values = %#v, want [32]", values)
	}

	// 宏内容修改：add 常量 31 → 77。引用图 host.obp 未动，仅重载。
	writeWorkspace(77)
	if _, err := bp.HotReload(); err != nil {
		t.Fatalf("HotReload failed: %v", err)
	}
	recorder = nil
	if _, err := bp.Do(graphID, 2); err != nil {
		t.Fatalf("second Do failed: %v", err)
	}
	if values := recorder.snapshot(); len(values) != 1 || values[0] != 78 {
		t.Fatalf("after HotReload recorder values = %#v, want [78]（宏修改必须热更生效）", values)
	}
}

// TestLoadGraphDirMacroRefMissingFails 引用的宏不存在时报错并指明 macroId。
func TestLoadGraphDirMacroRefMissingFails(t *testing.T) {
	registry := NewRegistry()
	registerFunctionTestNodes(registry, func() IExecNode { return &testRecorder{} })
	host := macroRefDocument("missing-macro", macroRefBoundary(true))
	macro := macroRefMacroDocument("m1", "Entrance_IntParam_2", 31, "", "")
	_, err := loadGraphDir(registry, writeMacroRefWorkspace(t, "m1", host, macro))
	if err == nil || !strings.Contains(err.Error(), "missing-macro") || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %v, want macro missing error mentioning missing-macro", err)
	}
}

// TestLoadGraphDirDuplicateMacroIDFails 两个 .obpm 声明同一 macroId 时报冲突。
func TestLoadGraphDirDuplicateMacroIDFails(t *testing.T) {
	registry := NewRegistry()
	registerFunctionTestNodes(registry, func() IExecNode { return &testRecorder{} })
	graphDir := t.TempDir()
	macro := macroRefMacroDocument("m1", "Entrance_IntParam_2", 31, "", "")
	if err := os.WriteFile(filepath.Join(graphDir, "a.obpm"), []byte(macro), 0644); err != nil {
		t.Fatalf("write a.obpm failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(graphDir, "b.obpm"), []byte(macro), 0644); err != nil {
		t.Fatalf("write b.obpm failed: %v", err)
	}
	if _, err := loadGraphDir(registry, graphDir); err == nil || !strings.Contains(err.Error(), `macro id "m1"`) {
		t.Fatalf("error = %v, want duplicate macro id conflict", err)
	}
}

// TestLoadGraphDirMacroRefEntranceDedup 宏入口与宿主入口同身份（同 class）时合并：
// 宏入口不重复创建，其下游逻辑接到宿主已有入口上。
func TestLoadGraphDirMacroRefEntranceDedup(t *testing.T) {
	var recorder *testRecorder
	registry := NewRegistry()
	registerFunctionTestNodes(registry, func() IExecNode { recorder = &testRecorder{}; return recorder })

	// 宿主与宏都用 Entrance_IntParam_1（entrance 1）。
	host := macroRefDocument("m1", "")
	macro := macroRefMacroDocument("m1", "Entrance_IntParam_1", 31, "", "")
	graphs, err := loadGraphDir(registry, writeMacroRefWorkspace(t, "m1", host, macro))
	if err != nil {
		t.Fatalf("loadGraphDir failed: %v", err)
	}
	hostGraph := graphs["host"]
	if hostGraph.NodeCount != 3 {
		t.Fatalf("node count = %d, want 3（宏入口必须合并到宿主入口，不重复创建）", hostGraph.NodeCount)
	}
	if len(hostGraph.Entrances) != 1 || hostGraph.Entrances[1] == nil {
		t.Fatalf("entrances = %#v, want single entrance 1", hostGraph.Entrances)
	}
	recorder = nil
	if _, err := NewGraph(hostGraph).Do(1); err != nil {
		t.Fatalf("Do(1) failed: %v", err)
	}
	if values := recorder.snapshot(); len(values) != 1 || values[0] != 32 {
		t.Fatalf("recorder values = %#v, want [32]（合并入口驱动宏内逻辑）", values)
	}
}

// TestLoadGraphDirMacroRefVariableMerge 宏变量同名同类型复用宿主定义，缺失补建。
func TestLoadGraphDirMacroRefVariableMerge(t *testing.T) {
	registry := NewRegistry()
	registerFunctionTestNodes(registry, func() IExecNode { return &testRecorder{} })
	host := `{
		"schemaVersion":1,
		"graphName":"Host",
		"nodes":[` + macroRefEntranceNode("ext", "Entrance_IntParam_1") + `],
		"connections":[],
		"variables":[{"id":"host-shared","name":"等级","type":"integer","defaultValue":5}],
		"macroRefs":[{"macroId":"m1","frame":{"x":0,"y":0,"width":10,"height":10}}]
	}`
	macro := `{
		"schemaVersion":1,
		"graphName":"宏_加成",
		"macroId":"m1",
		"nodes":[` + macroRefEntranceNode("entry", "Entrance_IntParam_2") + `,
			{"id":"read","typeId":"origin.variable.get","values":{},"properties":{"variableId":"macro-shared"}},
			{"id":"private","typeId":"origin.variable.get","values":{},"properties":{"variableId":"macro-private"}}],
		"connections":[],
		"variables":[
			{"id":"macro-shared","name":"等级","type":"integer","defaultValue":9},
			{"id":"macro-private","name":"宏_私有","type":"integer","defaultValue":3}]
	}`
	graphs, err := loadGraphDir(registry, writeMacroRefWorkspace(t, "m1", host, macro))
	if err != nil {
		t.Fatalf("loadGraphDir failed: %v", err)
	}
	variables := graphs["host"].Variables
	if len(variables) != 2 {
		t.Fatalf("variables = %#v, want 2（同名同类型复用 + 私有变量补建）", variables)
	}
	if shared, exists := variables["等级"]; !exists || shared.ID != "host-shared" {
		t.Fatalf("variables = %#v, want 等级 reuse host-shared definition", variables)
	}
	private, privateAdded := variables["宏_私有"]
	if !privateAdded || private.ID != "macro:0:m1:macro-private" {
		t.Fatalf("variables = %#v, want 宏_私有 added with prefixed id", variables)
	}
}

// TestLoadGraphDirNestedMacroRef 嵌套宏：宏 M2 引用宏 M1，引用图只引用 M2，
// 递归展开后 M1 的逻辑同样出现在引用图中。
func TestLoadGraphDirNestedMacroRef(t *testing.T) {
	var recorder *testRecorder
	registry := NewRegistry()
	registerFunctionTestNodes(registry, func() IExecNode { recorder = &testRecorder{}; return recorder })

	inner := macroRefMacroDocument("m-inner", "Entrance_IntParam_2", 31, "", "")
	// 外层宏 M2：入口(entry, entrance 3) 经边界连线直接驱动内层宏 M1 的 record（下标 2）。
	outer := `{
		"schemaVersion":1,
		"graphName":"宏_外层",
		"macroId":"m-outer",
		"nodes":[` + macroRefEntranceNode("entry", "Entrance_IntParam_3") + `],
		"connections":[],
		"variables":[],
		"macroRefs":[{"macroId":"m-inner","frame":{"x":0,"y":0,"width":10,"height":10},
			"boundary":[{"externalNode":"entry","externalPort":"exec","macroNodeIndex":2,"macroPort":"exec","intoMacro":true}]}]
	}`
	host := `{
		"schemaVersion":1,
		"graphName":"Host",
		"nodes":[],
		"connections":[],
		"variables":[],
		"macroRefs":[{"macroId":"m-outer","frame":{"x":0,"y":0,"width":10,"height":10}}]
	}`
	graphDir := t.TempDir()
	for name, data := range map[string]string{"宏_加成.obpm": inner, "宏_外层.obpm": outer, "host.obp": host} {
		if err := os.WriteFile(filepath.Join(graphDir, name), []byte(data), 0644); err != nil {
			t.Fatalf("write %s failed: %v", name, err)
		}
	}
	graphs, err := loadGraphDir(registry, graphDir)
	if err != nil {
		t.Fatalf("loadGraphDir failed: %v", err)
	}
	hostGraph := graphs["host"]
	// 外层入口(entrance 3) → 边界 → 内层 record ← 内层 add(31+1)。
	if _, err := NewGraph(hostGraph).Do(3); err != nil {
		t.Fatalf("Do(outer entrance) failed: %v", err)
	}
	if values := recorder.snapshot(); len(values) != 1 || values[0] != 32 {
		t.Fatalf("nested macro recorder values = %#v, want [32]（嵌套宏必须递归展开）", values)
	}
}

// TestLoadGraphDirCyclicMacroRef 宏自引用（直接或间接）必须报循环错误，不能死循环。
func TestLoadGraphDirCyclicMacroRef(t *testing.T) {
	registry := NewRegistry()
	registerFunctionTestNodes(registry, func() IExecNode { return &testRecorder{} })
	self := `{
		"schemaVersion":1,
		"graphName":"宏_自引用",
		"macroId":"m-cycle",
		"nodes":[` + macroRefEntranceNode("entry", "Entrance_IntParam_2") + `],
		"connections":[],
		"variables":[],
		"macroRefs":[{"macroId":"m-cycle","frame":{"x":0,"y":0,"width":10,"height":10}}]
	}`
	graphDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(graphDir, "宏_自引用.obpm"), []byte(self), 0644); err != nil {
		t.Fatalf("write macro failed: %v", err)
	}
	_, err := loadGraphDir(registry, graphDir)
	if err == nil || !strings.Contains(err.Error(), "cyclic") {
		t.Fatalf("error = %v, want cyclic macro reference error", err)
	}
}

// 共享记录器：多入口/多引用场景下每个 TestRecorder 节点都把输入追加到同一列表，便于断言。
type macroSharedRecorder struct {
	BaseExecNode
	mu     *sync.Mutex
	values *[]PortInt
}

func (n *macroSharedRecorder) GetName() string { return "TestRecorder" }
func (n *macroSharedRecorder) Exec() (int, error) {
	if value, ok := n.GetInPortInt(1); ok {
		n.mu.Lock()
		*n.values = append(*n.values, value)
		n.mu.Unlock()
	}
	return -1, nil
}

func macroSharedRecorderFactory(mu *sync.Mutex, values *[]PortInt) func() IExecNode {
	return func() IExecNode { return &macroSharedRecorder{mu: mu, values: values} }
}

// macroRefMultiEntranceMacroDocument：宏含 entrance 2 与 entrance 3 两个入口，各驱动一个 record。
// 节点顺序（boundary 的 macroNodeIndex 依赖）：0=entry2, 1=recordA, 2=entry3, 3=recordB, 4=add。
func macroRefMultiEntranceMacroDocument(macroID string, addValue int) string {
	return fmt.Sprintf(`{
		"schemaVersion":1,
		"graphName":"宏_多入口",
		"macroId":%q,
		"nodes":[%s,%s,%s,%s,%s],
		"connections":[
			{"source":"entry2","sourceOutput":"exec","target":"recordA","targetInput":"exec"},
			{"source":"entry3","sourceOutput":"exec","target":"recordB","targetInput":"exec"},
			{"source":"add","sourceOutput":"result","target":"recordA","targetInput":"integer"},
			{"source":"add","sourceOutput":"result","target":"recordB","targetInput":"integer"}
		],
		"variables":[]
	}`,
		macroID,
		macroRefEntranceNode("entry2", "Entrance_IntParam_2"),
		macroRefRecorderNode("recordA"),
		macroRefEntranceNode("entry3", "Entrance_IntParam_3"),
		macroRefRecorderNode("recordB"),
		macroRefAddNode("add", addValue))
}

// TestMacroRefMultipleEntrancesAllTrigger 宏的每个入口都必须能从宿主图触发：
// entrance 2 / entrance 3 各自驱动独立逻辑；宿主入口（entrance 1）经边界连线驱动宏内节点。
func TestMacroRefMultipleEntrancesAllTrigger(t *testing.T) {
	var mu sync.Mutex
	var values []PortInt
	registry := NewRegistry()
	registerFunctionTestNodes(registry, macroSharedRecorderFactory(&mu, &values))

	host := `{
		"schemaVersion":1,
		"graphName":"Host",
		"nodes":[` + macroRefEntranceNode("ext", "Entrance_IntParam_1") + `],
		"connections":[],
		"variables":[],
		"macroRefs":[{"macroId":"m-multi",
			"frame":{"x":0,"y":0,"width":10,"height":10},
			"boundary":[{"externalNode":"ext","externalPort":"exec","macroNodeIndex":1,"macroPort":"exec","intoMacro":true}]}]
	}`
	graphDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(graphDir, "宏_多入口.obpm"), []byte(macroRefMultiEntranceMacroDocument("m-multi", 31)), 0644); err != nil {
		t.Fatalf("write macro failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(graphDir, "host.obp"), []byte(host), 0644); err != nil {
		t.Fatalf("write host failed: %v", err)
	}
	graphs, err := loadGraphDir(registry, graphDir)
	if err != nil {
		t.Fatalf("loadGraphDir failed: %v", err)
	}
	hostGraph := graphs["host"]

	mu.Lock()
	values = nil
	mu.Unlock()
	if _, err := NewGraph(hostGraph).Do(2); err != nil {
		t.Fatalf("Do(entrance 2) failed: %v", err)
	}
	mu.Lock()
	got := append([]PortInt(nil), values...)
	values = nil
	mu.Unlock()
	if len(got) != 1 || got[0] != 32 {
		t.Fatalf("entrance 2 values = %v, want [32]", got)
	}

	if _, err := NewGraph(hostGraph).Do(3); err != nil {
		t.Fatalf("Do(entrance 3) failed: %v", err)
	}
	mu.Lock()
	got = append([]PortInt(nil), values...)
	values = nil
	mu.Unlock()
	if len(got) != 1 || got[0] != 32 {
		t.Fatalf("entrance 3 values = %v, want [32]", got)
	}

	if _, err := NewGraph(hostGraph).Do(1); err != nil {
		t.Fatalf("Do(host entrance) failed: %v", err)
	}
	mu.Lock()
	got = append([]PortInt(nil), values...)
	mu.Unlock()
	if len(got) != 1 || got[0] != 32 {
		t.Fatalf("host entrance (boundary) values = %v, want [32]", got)
	}
}

// TestMacroRefTwoReferencesOfSameMacro 同一宏被引用两次：入口合并为一份（触发一次全图执行），
// 但每次引用各有一份宏内节点副本，边界连线各自指向自己的副本。
func TestMacroRefTwoReferencesOfSameMacro(t *testing.T) {
	var mu sync.Mutex
	var values []PortInt
	registry := NewRegistry()
	registerFunctionTestNodes(registry, macroSharedRecorderFactory(&mu, &values))

	host := `{
		"schemaVersion":1,
		"graphName":"Host",
		"nodes":[],
		"connections":[],
		"variables":[],
		"macroRefs":[
			{"macroId":"m-multi","frame":{"x":0,"y":0,"width":10,"height":10}},
			{"macroId":"m-multi","frame":{"x":500,"y":0,"width":10,"height":10},
			 "boundary":[{"externalNode":"ext","externalPort":"exec","macroNodeIndex":1,"macroPort":"exec","intoMacro":true}]}
		]
	}`
	// 第二个引用的边界指向 ext，但宿主没有 ext 节点——展开应报错而不是静默丢失。
	graphDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(graphDir, "宏_多入口.obpm"), []byte(macroRefMultiEntranceMacroDocument("m-multi", 31)), 0644); err != nil {
		t.Fatalf("write macro failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(graphDir, "host.obp"), []byte(host), 0644); err != nil {
		t.Fatalf("write host failed: %v", err)
	}
	if _, err := loadGraphDir(registry, graphDir); err == nil {
		t.Fatal("boundary referencing missing external node must fail loudly")
	}

	// 同一宏双重引用必须被明确拒绝（入口合并会让共享入口的 exec 出口出现多个后继），
	// 报错要指明"每个宏只能引用一次"，而不是抛出晦涩的底层编译错误。
	hostValid := `{
		"schemaVersion":1,
		"graphName":"Host2",
		"nodes":[],
		"connections":[],
		"variables":[],
		"macroRefs":[
			{"macroId":"m-multi","frame":{"x":0,"y":0,"width":10,"height":10}},
			{"macroId":"m-multi","frame":{"x":500,"y":0,"width":10,"height":10}}
		]
	}`
	graphDir2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(graphDir2, "宏_多入口.obpm"), []byte(macroRefMultiEntranceMacroDocument("m-multi", 31)), 0644); err != nil {
		t.Fatalf("write macro failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(graphDir2, "host.obp"), []byte(hostValid), 0644); err != nil {
		t.Fatalf("write host failed: %v", err)
	}
	_, err := loadGraphDir(registry, graphDir2)
	if err == nil || !strings.Contains(err.Error(), "referenced multiple times") {
		t.Fatalf("error = %v, want explicit duplicate macro reference error", err)
	}
}

// TestMacroRefHotReloadEntranceLifecycle 运行期热更：宏新增入口立即可调用；删除的入口不再可调用；
// 宏源被删后 HotReload 失败但旧图保留（可继续用旧逻辑执行）。
func TestMacroRefHotReloadEntranceLifecycle(t *testing.T) {
	root := t.TempDir()
	execDir := filepath.Join(root, "json")
	graphDir := filepath.Join(root, "vgf")
	if err := os.MkdirAll(execDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(graphDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(execDir, "nodes.json"), []byte(`[
		{"name":"Entrance_IntParam","inputs":[],"outputs":[{"type":"exec","port_id":0}]},
		{"name":"AddInt","inputs":[{"type":"data","data_type":"int","port_id":0},{"type":"data","data_type":"int","port_id":1}],"outputs":[{"type":"data","data_type":"int","port_id":0}]},
		{"name":"TestRecorder","inputs":[{"type":"exec","port_id":0},{"type":"data","data_type":"int","port_id":1}],"outputs":[]}
	]`), 0644); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var values []PortInt
	// v1：只有 entrance 2。
	macroV1 := fmt.Sprintf(`{
		"schemaVersion":1,
		"graphName":"宏_多入口",
		"macroId":"m-life",
		"nodes":[%s,%s,%s],
		"connections":[
			{"source":"entry2","sourceOutput":"exec","target":"recordA","targetInput":"exec"},
			{"source":"add","sourceOutput":"result","target":"recordA","targetInput":"integer"}
		],
		"variables":[]
	}`, macroRefEntranceNode("entry2", "Entrance_IntParam_2"), macroRefRecorderNode("recordA"), macroRefAddNode("add", 10))
	host := `{
		"schemaVersion":1,
		"graphName":"host",
		"nodes":[],
		"connections":[],
		"variables":[],
		"macroRefs":[{"macroId":"m-life","frame":{"x":0,"y":0,"width":10,"height":10}}]
	}`
	writeMacro := func(doc string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(graphDir, "宏_多入口.obpm"), []byte(doc), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeMacro(macroV1)
	if err := os.WriteFile(filepath.Join(graphDir, "host.obp"), []byte(host), 0644); err != nil {
		t.Fatal(err)
	}

	var bp Blueprint
	bp.RegisterExecNode(func() IExecNode { return &EntranceIntParam{} })
	bp.RegisterExecNode(func() IExecNode { return &AddInt{} })

	bp.RegisterExecNode(macroSharedRecorderFactory(&mu, &values))
	if err := bp.Init(execDir, graphDir, nil); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	graphID := bp.Create("host")

	// v1：entrance 3 不存在，调用必须失败。
	if _, err := bp.Do(graphID, 3); err == nil {
		t.Fatal("Do(3) on v1 must fail: entrance not compiled yet")
	}

	// v2：宏新增 entrance 3 → 热更后立即可调用。
	writeMacro(macroRefMultiEntranceMacroDocument("m-life", 20))
	if _, err := bp.HotReload(); err != nil {
		t.Fatalf("HotReload(add entrance) failed: %v", err)
	}
	mu.Lock()
	values = nil
	mu.Unlock()
	if _, err := bp.Do(graphID, 3); err != nil {
		t.Fatalf("Do(3) after hot reload failed: %v", err)
	}
	mu.Lock()
	got := append([]PortInt(nil), values...)
	mu.Unlock()
	if len(got) != 1 || got[0] != 21 {
		t.Fatalf("after add-entrance values = %v, want [21]", got)
	}

	// v3：宏删掉全部入口 → 热更后宿主图没有入口，调用失败。
	broken := fmt.Sprintf(`{
		"schemaVersion":1,
		"graphName":"宏_多入口",
		"macroId":"m-life",
		"nodes":[%s,%s],
		"connections":[{"source":"add","sourceOutput":"result","target":"recordA","targetInput":"integer"}],
		"variables":[]
	}`, macroRefAddNode("add", 1), macroRefRecorderNode("recordA"))
	writeMacro(broken)
	if _, err := bp.HotReload(); err == nil {
		t.Fatal("hot reload must be rejected when the macro loses all entrances")
	}
	// 旧图保留：已有实例继续按旧宏逻辑执行（降级不崩）。
	mu.Lock()
	values = nil
	mu.Unlock()
	if _, err := bp.Do(graphID, 2); err != nil {
		t.Fatalf("Do(2) on retained old graph failed: %v", err)
	}
	mu.Lock()
	got = append([]PortInt(nil), values...)
	mu.Unlock()
	if len(got) != 1 || got[0] != 21 {
		t.Fatalf("retained old graph values = %v, want [21]（旧宏逻辑继续可用）", got)
	}

	// v4：宏源文件删除 → HotReload 报错，旧图保留（降级不崩）。
	if err := os.Remove(filepath.Join(graphDir, "宏_多入口.obpm")); err != nil {
		t.Fatal(err)
	}
	if _, err := bp.HotReload(); err == nil {
		t.Fatal("HotReload with missing macro source must fail")
	}
	mu.Lock()
	values = nil
	mu.Unlock()
	if _, err := bp.Do(graphID, 2); err != nil {
		t.Fatalf("Do(2) after failed reload failed: %v", err)
	}
	mu.Lock()
	got = append([]PortInt(nil), values...)
	mu.Unlock()
	if len(got) != 1 || got[0] != 21 {
		t.Fatalf("after failed reload values = %v, want [21]", got)
	}
	// 恢复宏源并热更，功能恢复（证明失败的热更没有破坏状态）。
	writeMacro(macroV1)
	if _, err := bp.HotReload(); err != nil {
		t.Fatalf("HotReload after restore failed: %v", err)
	}
	mu.Lock()
	values = nil
	mu.Unlock()
	if _, err := bp.Do(graphID, 2); err != nil {
		t.Fatalf("Do(2) after restore failed: %v", err)
	}
	mu.Lock()
	got = append([]PortInt(nil), values...)
	mu.Unlock()
	if len(got) != 1 || got[0] != 11 {
		t.Fatalf("after restore values = %v, want [11]", got)
	}
}

// TestMacroRefHotReloadConcurrentWithExecutions 并发安全：热更期间持续执行引用图，
// -race 下验证无数据竞争；热更完成后执行采用新宏内容。
func TestMacroRefHotReloadConcurrentWithExecutions(t *testing.T) {
	root := t.TempDir()
	execDir := filepath.Join(root, "json")
	graphDir := filepath.Join(root, "vgf")
	if err := os.MkdirAll(execDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(graphDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(execDir, "nodes.json"), []byte(`[
		{"name":"Entrance_IntParam","inputs":[],"outputs":[{"type":"exec","port_id":0}]},
		{"name":"AddInt","inputs":[{"type":"data","data_type":"int","port_id":0},{"type":"data","data_type":"int","port_id":1}],"outputs":[{"type":"data","data_type":"int","port_id":0}]},
		{"name":"TestRecorder","inputs":[{"type":"exec","port_id":0},{"type":"data","data_type":"int","port_id":1}],"outputs":[]}
	]`), 0644); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var values []PortInt
	singleEntranceMacro := func(addValue int) string {
		return fmt.Sprintf(`{
			"schemaVersion":1,
			"graphName":"宏_单入口",
			"macroId":"m-race",
			"nodes":[%s,%s,%s],
			"connections":[
				{"source":"entry","sourceOutput":"exec","target":"record","targetInput":"exec"},
				{"source":"add","sourceOutput":"result","target":"record","targetInput":"integer"}
			],
			"variables":[]
		}`, macroRefEntranceNode("entry", "Entrance_IntParam_2"), macroRefAddNode("add", addValue), macroRefRecorderNode("record"))
	}
	host := `{
		"schemaVersion":1,
		"graphName":"host",
		"nodes":[],
		"connections":[],
		"variables":[],
		"macroRefs":[{"macroId":"m-race","frame":{"x":0,"y":0,"width":10,"height":10}}]
	}`
	if err := os.WriteFile(filepath.Join(graphDir, "宏_单入口.obpm"), []byte(singleEntranceMacro(1)), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(graphDir, "host.obp"), []byte(host), 0644); err != nil {
		t.Fatal(err)
	}

	var bp Blueprint
	bp.RegisterExecNode(func() IExecNode { return &EntranceIntParam{} })
	bp.RegisterExecNode(func() IExecNode { return &AddInt{} })
	bp.RegisterExecNode(macroSharedRecorderFactory(&mu, &values))
	if err := bp.Init(execDir, graphDir, nil); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	graphID := bp.Create("host")

	var wg sync.WaitGroup
	stop := make(chan struct{})
	execErr := make(chan error, 1)
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := bp.Do(graphID, 2); err != nil {
					select {
					case execErr <- err:
					default:
					}
					return
				}
			}
		}()
	}
	if err := os.WriteFile(filepath.Join(graphDir, "宏_单入口.obpm"), []byte(singleEntranceMacro(99)), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := bp.HotReload(); err != nil {
		t.Fatalf("HotReload failed: %v", err)
	}
	close(stop)
	wg.Wait()
	select {
	case err := <-execErr:
		t.Fatalf("concurrent Do failed during hot reload: %v", err)
	default:
	}
	// 热更完成后：新实例执行采用新宏内容（100），旧值（2）不再出现。
	mu.Lock()
	values = nil
	mu.Unlock()
	if _, err := bp.Do(graphID, 2); err != nil {
		t.Fatalf("Do after reload failed: %v", err)
	}
	mu.Lock()
	got := append([]PortInt(nil), values...)
	mu.Unlock()
	if len(got) != 1 || got[0] != 100 {
		t.Fatalf("after concurrent reload values = %v, want [100]", got)
	}
}
