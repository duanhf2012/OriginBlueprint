package blueprint

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseGraphDocumentRejectsUnknownExecutionField(t *testing.T) {
	data := []byte(`{
		"schemaVersion":1,
		"graphName":"Typo",
		"nodes":[],
		"connetions":[],
		"variables":[]
	}`)

	_, err := ParseGraphConfigJSON(data)
	if err == nil || !strings.Contains(err.Error(), "connetions") {
		t.Fatalf("ParseGraphConfigJSON error = %v, want unknown field connetions", err)
	}
	var structured *BlueprintError
	if !errors.As(err, &structured) || structured.Stage != BlueprintStageParse {
		t.Fatalf("ParseGraphConfigJSON error = %#v, want parse BlueprintError", err)
	}
}

func TestCompileGraphReturnsStructuredCompileError(t *testing.T) {
	_, err := CompileGraph(testSystemRegistry(t), GraphConfig{Nodes: []NodeConfig{
		{ID: "duplicate", Class: "LiteralInt"},
		{ID: "duplicate", Class: "LiteralInt"},
	}})
	var structured *BlueprintError
	if err == nil || !errors.As(err, &structured) || structured.Stage != BlueprintStageCompile {
		t.Fatalf("CompileGraph error = %#v, want compile BlueprintError", err)
	}
}

func TestCompileGraphMissingDefinitionCarriesNodeID(t *testing.T) {
	_, err := CompileGraph(NewRegistry(), GraphConfig{Nodes: []NodeConfig{{ID: "missing", Class: "MissingNode"}}})
	var structured *BlueprintError
	if err == nil || !errors.As(err, &structured) || structured.Stage != BlueprintStageCompile || structured.NodeID != "missing" {
		t.Fatalf("CompileGraph error = %#v, want compile BlueprintError for node missing", err)
	}
}

func TestParseGraphDocumentMissingDefinitionCarriesNodeID(t *testing.T) {
	_, err := ParseGraphConfigJSON([]byte(`{
		"schemaVersion":1,
		"graphName":"Missing",
		"nodes":[{"id":"missing","typeId":"origin.missing","values":{}}],
		"connections":[],
		"variables":[]
	}`))
	var structured *BlueprintError
	if err == nil || !errors.As(err, &structured) || structured.Stage != BlueprintStageParse || structured.NodeID != "missing" {
		t.Fatalf("ParseGraphConfigJSON error = %#v, want parse BlueprintError for node missing", err)
	}
}

func TestParseGraphDocumentAllowsKnownEditorMetadata(t *testing.T) {
	data := []byte(`{
		"schemaVersion":1,
		"graphName":"Metadata",
		"functionCategory":"Category",
		"nodes":[{
			"id":"literal",
			"typeId":"origin.literal.string",
			"position":{"x":1,"y":2},
			"values":{"value":"ok"},
			"properties":{"label":"Literal"}
		}],
		"connections":[],
		"groups":[],
		"variables":[],
		"variableGroups":[],
		"view":{"x":0,"y":0,"zoom":1}
	}`)

	if _, err := ParseGraphConfigJSON(data); err != nil {
		t.Fatalf("ParseGraphConfigJSON rejected editor metadata: %v", err)
	}
}

// 编辑器会给函数签名端口写入 ref（关联数据集 key）供画布引用选择控件使用；
// 引擎不消费该字段，但严格 JSON 解析必须接受它，否则带数据集绑定的函数文档无法编译。
// 宏引用字段（macroId/macroRefs）同样必须通过严格解析，
// 但 macroRefs 只能由目录加载层（Init/HotReload → expandMacroRefs）展开，
// 单文档入口 ParseGraphConfigJSON 对未展开的宏引用显式报错而不是静默丢逻辑。
func TestParseGraphDocumentAllowsSignaturePortRef(t *testing.T) {
	data := []byte(`{
		"schemaVersion":1,
		"graphName":"RefBinding",
		"functionSignature":{
			"inputs":[{"id":"base","name":"基础分","type":"integer","ref":"数据集/key"}],
			"outputs":[{"id":"out","name":"结果","type":"integer"}]
		},
		"comments":[{"id":"c1","text":"备注","x":1,"y":2,"width":100,"height":50}],
		"macroId":"m_a3f8",
		"macroRefs":[{"macroId":"m_a3f8",
			"frame":{"x":1,"y":2,"width":100,"height":50},
			"boundary":[{"externalNode":"n9","externalPort":"exec","macroNodeIndex":0,"macroPort":"exec","intoMacro":true}]}],
		"nodes":[{
			"id":"entry",
			"typeId":"origin.function.entry",
			"position":{"x":1,"y":2},
			"values":{},
			"properties":{
				"label":"Entry",
				"functionRole":"entry",
				"functionSignature":{
					"inputs":[{"id":"base","name":"基础分","type":"integer","ref":"数据集/key"}],
					"outputs":[{"id":"out","name":"结果","type":"integer"}]
				}
			}
		}],
		"connections":[],
		"groups":[],
		"variables":[],
		"variableGroups":[],
		"view":{"x":0,"y":0,"zoom":1}
	}`)

	var document graphDocument
	if err := decodeGraphDocument(data, &document); err != nil {
		t.Fatalf("decodeGraphDocument rejected editor metadata: %v", err)
	}
	if len(document.MacroRefs) != 1 || document.MacroRefs[0].MacroID != "m_a3f8" || len(document.MacroRefs[0].Boundary) != 1 {
		t.Fatalf("macroRefs decoded = %#v", document.MacroRefs)
	}
	if _, err := ParseGraphConfigJSON(data); err == nil {
		t.Fatal("ParseGraphConfigJSON accepted unexpanded macro refs; they must fail loudly instead of silently dropping macro logic")
	}
	document.MacroRefs = nil
	withoutRefs, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("re-encode document: %v", err)
	}
	if _, err := ParseGraphConfigJSON(withoutRefs); err != nil {
		t.Fatalf("ParseGraphConfigJSON rejected signature port ref: %v", err)
	}
}

func TestParseGraphDocumentRejectsUnknownNodeValue(t *testing.T) {
	data := []byte(`{
		"schemaVersion":1,
		"graphName":"DefaultTypo",
		"nodes":[{
			"id":"literal",
			"typeId":"origin.literal.string",
			"values":{"vale":"wrong"}
		}],
		"connections":[],
		"variables":[]
	}`)

	_, err := ParseGraphConfigJSON(data)
	if err == nil || !strings.Contains(err.Error(), "literal") || !strings.Contains(err.Error(), "vale") {
		t.Fatalf("ParseGraphConfigJSON error = %v, want node and unknown value key", err)
	}
}

func TestParseGraphDocumentRejectsDuplicateVariableID(t *testing.T) {
	data := []byte(`{
		"schemaVersion":1,
		"graphName":"Variables",
		"nodes":[],
		"connections":[],
		"variables":[
			{"id":"shared","name":"A","type":"Integer","defaultValue":1},
			{"id":"shared","name":"B","type":"Integer","defaultValue":2}
		]
	}`)

	_, err := ParseGraphConfigJSON(data)
	if err == nil || !strings.Contains(err.Error(), "duplicate variable id") || !strings.Contains(err.Error(), "shared") {
		t.Fatalf("ParseGraphConfigJSON error = %v, want duplicate variable id", err)
	}
}

func TestLoadGraphDirRejectsGraphWithoutEntranceWithSourcePath(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "missing-entrance.obp")
	writeTestFile(t, path, `{
		"schemaVersion":1,
		"graphName":"MissingEntrance",
		"nodes":[{"id":"literal","typeId":"origin.literal.string","values":{"value":"ok"}}],
		"connections":[],
		"variables":[]
	}`)

	_, err := loadGraphDir(testSystemRegistry(t), root)
	if err == nil || !strings.Contains(filepath.ToSlash(err.Error()), "missing-entrance.obp") || !strings.Contains(err.Error(), "entrance") {
		t.Fatalf("loadGraphDir error = %v, want source path and missing entrance", err)
	}
}

func TestLoadGraphDirAllowsEmptyLegacyPlaceholderGraph(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "empty-placeholder.vgf"), `{
		"graph_name":"",
		"time":"",
		"nodes":[],
		"edges":[],
		"groups":[],
		"variables":[]
	}`)

	graphs, err := loadGraphDir(testSystemRegistry(t), root)
	if err != nil {
		t.Fatalf("loadGraphDir rejected legacy placeholder: %v", err)
	}
	if graphs["empty-placeholder"] == nil {
		t.Fatal("empty legacy placeholder graph was not loaded")
	}
}

func TestLoadGraphDirRejectsFunctionWithoutReturnWithSourcePath(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "missing-return.obpf")
	if err := os.WriteFile(path, []byte(`{
		"schemaVersion":1,
		"graphName":"MissingReturn",
		"functionId":"missing-return",
		"nodes":[{"id":"entry","typeId":"origin.function.entry","values":{},"properties":{"functionSignature":{"inputs":[],"outputs":[]}}}],
		"connections":[],
		"variables":[],
		"functionSignature":{"inputs":[],"outputs":[]}
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := loadGraphDir(testSystemRegistry(t), root)
	if err == nil || !strings.Contains(filepath.ToSlash(err.Error()), "missing-return.obpf") || !strings.Contains(err.Error(), "FunctionReturn") {
		t.Fatalf("loadGraphDir error = %v, want source path and missing FunctionReturn", err)
	}
	var structured *BlueprintError
	if !errors.As(err, &structured) || structured.Stage != BlueprintStageCompile || filepath.Clean(structured.SourcePath) != filepath.Clean(path) {
		t.Fatalf("loadGraphDir error = %#v, want compile BlueprintError with source path %s", err, path)
	}
}
