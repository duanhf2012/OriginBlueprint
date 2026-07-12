package blueprint

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const verificationMatrixReportEnv = "WRITE_BLUEPRINT_VERIFICATION_REPORT"

type verificationMatrixRow struct {
	input         string
	blueprint     PortArray
	reference     PortArray
	functionCalls string
}

func TestWriteVerificationMatrixReport(t *testing.T) {
	if os.Getenv(verificationMatrixReportEnv) != "1" {
		t.Skipf("set %s=1 to generate the verification matrix", verificationMatrixReportEnv)
	}

	var report bytes.Buffer
	report.WriteString("# 蓝图执行对比矩阵\n\n")
	report.WriteString("本报告由 Go 测试实际执行生成。每行均已断言蓝图输出与独立 Go 参考逻辑一致；输入采用可复现的零值、正负值和分支边界值。`01_legacy_all_nodes_showcase.vgf` 仅用于 legacy 导入与显示验证，不包含可执行结果契约。\n")

	intInputs := verificationMatrixIntInputs()
	appendVerificationMatrixSection(&report, "02_control_flow_maze.obp", "入口的三个整数端口当前未接入执行流，因此十组输入用于确认结果不受未使用入口值污染。", verificationControlFlowMatrixRows(t, intInputs))
	appendVerificationMatrixSection(&report, "03_array_data_lab.obp", "入口对象 ID 与数组端口当前未接入执行流，因此十组输入用于确认固定数组与局部变量流程稳定。", verificationArrayMatrixRows(t, intInputs))
	appendVerificationMatrixSection(&report, "04_deterministic_algorithm.obp", "参数 1、参数 2 参与整数评分、除法、取模和分支。", verificationAlgorithmMatrixRows(t, intInputs))
	appendVerificationMatrixSection(&report, "05_function_orchestrator.obp", "主图入口当前未接入后续函数调用；每行同时列出四个内部函数调用的实际参数和返回值。", verificationFunctionMatrixRows(t, intInputs))
	appendVerificationMatrixSection(&report, "06_timer_lifecycle.obp", "入口参数当前未接入定时器生命周期；每行执行创建、暂停、恢复、查询和清理，并列出定时器回调函数参数。", verificationTimerMatrixRows(t, intInputs))
	appendVerificationMatrixSection(&report, "07_async_rpc_resume_to.obp", "入口参数当前未接入示例 RPC；每行依次执行成功与失败两次异步恢复。", verificationRPCMatrixRows(t, intInputs))

	path := filepath.Join("..", "..", "..", "docs", "BLUEPRINT_VERIFICATION_MATRIX_ZH.md")
	if err := os.WriteFile(path, report.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", path)
}

func verificationMatrixIntInputs() [][3]PortInt {
	return [][3]PortInt{
		{0, 0, 0},
		{1, 1, 1},
		{1, 10, 5},
		{2, -1, 1},
		{7, -10, -5},
		{42, 2, 3},
		{99, 11, 12},
		{100, 100, -100},
		{-1, 4, 8},
		{214, -50, 50},
	}
}

func verificationControlFlowMatrixRows(t *testing.T, inputs [][3]PortInt) []verificationMatrixRow {
	graph := loadVerificationGraph(t, "02_control_flow_maze.obp")
	rows := make([]verificationMatrixRow, 0, len(inputs))
	for _, input := range inputs {
		actual := verificationMatrixRun(t, NewGraph(graph), EntranceIDIntParam, referenceControlFlowMaze(), input[0], input[1], input[2])
		rows = append(rows, verificationMatrixRow{input: formatIntInputs(input), blueprint: actual, reference: referenceControlFlowMaze(), functionCalls: "无"})
	}
	return rows
}

func verificationArrayMatrixRows(t *testing.T, inputs [][3]PortInt) []verificationMatrixRow {
	graph := loadVerificationGraph(t, "03_array_data_lab.obp")
	arrays := []PortArray{
		{},
		{{IntVal: 1}},
		{{IntVal: -1}, {IntVal: 2}},
		{{IntVal: 3}, {IntVal: 1}, {IntVal: 4}},
		{{IntVal: 9}, {IntVal: 8}, {IntVal: 7}, {IntVal: 6}},
	}
	rows := make([]verificationMatrixRow, 0, len(inputs))
	for index, input := range inputs {
		array := arrays[index%len(arrays)]
		actual := verificationMatrixRun(t, NewGraph(graph), EntranceIDArrayParam, referenceArrayDataLab(), input[0], array)
		rows = append(rows, verificationMatrixRow{input: fmt.Sprintf("对象ID=%d, 数组=%s", input[0], formatPortArray(array)), blueprint: actual, reference: referenceArrayDataLab(), functionCalls: "无"})
	}
	return rows
}

func verificationAlgorithmMatrixRows(t *testing.T, inputs [][3]PortInt) []verificationMatrixRow {
	graph := loadVerificationGraph(t, "04_deterministic_algorithm.obp")
	rows := make([]verificationMatrixRow, 0, len(inputs))
	for _, input := range inputs {
		want := referenceDeterministicAlgorithm(input[1], input[2])
		actual := verificationMatrixRun(t, NewGraph(graph), EntranceIDIntParam, want, input[0], input[1], input[2])
		rows = append(rows, verificationMatrixRow{input: formatIntInputs(input), blueprint: actual, reference: want, functionCalls: "无"})
	}
	return rows
}

func verificationFunctionMatrixRows(t *testing.T, inputs [][3]PortInt) []verificationMatrixRow {
	graphs := loadVerificationFixtureSet(t)
	main := graphs["函数编排主图"]
	if main == nil {
		t.Fatal("function orchestrator fixture was not loaded")
	}
	functionCalls := verificationFunctionCallSummary(t, graphs)
	want := referenceFunctionOrchestrator()
	rows := make([]verificationMatrixRow, 0, len(inputs))
	for _, input := range inputs {
		actual := verificationMatrixRun(t, NewGraph(main), EntranceIDIntParam, want, input[0], input[1], input[2])
		rows = append(rows, verificationMatrixRow{input: formatIntInputs(input), blueprint: actual, reference: want, functionCalls: functionCalls})
	}
	return rows
}

func verificationTimerMatrixRows(t *testing.T, inputs [][3]PortInt) []verificationMatrixRow {
	rows := make([]verificationMatrixRow, 0, len(inputs))
	want := PortArray{{StrVal: "timer-lifecycle-complete"}}
	for _, input := range inputs {
		graphs := loadVerificationFixtureSet(t)
		callback := verificationFixtureFunction(t, graphs, "functions/13_local_state_isolation.obpf")
		callbackReturns := verificationMatrixRun(t, NewGraph(callback), FunctionEntranceID, PortArray{{IntVal: 11}}, PortInt(11))
		bp := &Blueprint{}
		for name, graph := range graphs {
			bp.AddCompiledGraph(name, graph)
		}
		graphID := bp.Create("新定时器生命周期")
		actual, err := bp.Do(graphID, EntranceIDIntParam, input[0], input[1], input[2])
		if err != nil {
			t.Fatalf("timer Do(%s): %v", formatIntInputs(input), err)
		}
		assertVerificationReturns(t, actual, want)
		rows = append(rows, verificationMatrixRow{
			input:         formatIntInputs(input),
			blueprint:     actual,
			reference:     want,
			functionCalls: "Set Timer by Function: 局部状态隔离(种子=11) => " + formatPortArray(callbackReturns) + "（循环回调，主图返回不包含回调结果）",
		})
	}
	return rows
}

func verificationRPCMatrixRows(t *testing.T, inputs [][3]PortInt) []verificationMatrixRow {
	rows := make([]verificationMatrixRow, 0, len(inputs))
	want := PortArray{{IntVal: 314}, {IntVal: 503}, {StrVal: "mock rpc unavailable"}}
	for _, input := range inputs {
		actual := verificationMatrixRunMockRPC(t, input)
		assertVerificationReturns(t, actual, want)
		rows = append(rows, verificationMatrixRow{
			input:         formatIntInputs(input),
			blueprint:     actual,
			reference:     want,
			functionCalls: "MockRpcAsync(80ms, true, 314, 0, \"\") => 成功[value=314]<br>MockRpcAsync(80ms, false, 0, 503, \"mock rpc unavailable\") => 失败[code=503, message=\"mock rpc unavailable\"]",
		})
	}
	return rows
}

func verificationMatrixRun(t *testing.T, graph *Graph, entranceID int64, want PortArray, args ...any) PortArray {
	t.Helper()
	actual, err := graph.Do(entranceID, args...)
	if err != nil {
		t.Fatal(err)
	}
	assertVerificationReturns(t, actual, want)
	return actual
}

func verificationMatrixRunMockRPC(t *testing.T, input [3]PortInt) PortArray {
	t.Helper()
	registry := verificationFixtureRegistry(t)
	graph := loadVerificationGraphWithRegistry(t, "07_async_rpc_resume_to.obp", registry)
	dispatcher := &manualExecutionDispatcher{}
	scheduler := newManualTimerScheduler()
	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.SetTimerScheduler(scheduler)
	bp.AddCompiledGraph("定时器模拟 RPC 异步恢复", graph)
	execution, err := bp.Start(context.Background(), bp.Create("定时器模拟 RPC 异步恢复"), EntranceIDIntParam, input[0], input[1], input[2])
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.runNext(t)
	scheduler.fire(t, scheduler.onlyHandle(t))
	dispatcher.runNext(t)
	scheduler.fire(t, scheduler.onlyHandle(t))
	dispatcher.runNext(t)
	returns, err := execution.Result()
	if err != nil {
		t.Fatal(err)
	}
	return returns
}

func verificationFunctionCallSummary(t *testing.T, graphs map[string]*CompiledGraph) string {
	t.Helper()
	call := func(functionID string, args ...any) PortArray {
		function := verificationFixtureFunction(t, graphs, functionID)
		returns, err := NewGraph(function).Do(FunctionEntranceID, args...)
		if err != nil {
			t.Fatalf("%s: %v", functionID, err)
		}
		return returns
	}
	score := call("functions/10_score_kernel.obpf", PortInt(10), PortInt(5), PortInt(2))
	fold := call("functions/11_array_fold_and_format.obpf", PortArray{{IntVal: 3}, {IntVal: 1}, {IntVal: 4}, {IntVal: 1}, {IntVal: 5}}, PortInt(2))
	nested := call("functions/12_nested_control_function.obpf", PortInt(0), PortInt(4))
	localFirst := call("functions/13_local_state_isolation.obpf", PortInt(7))
	localSecond := call("functions/13_local_state_isolation.obpf", PortInt(7))
	return strings.Join([]string{
		"评分核心(10, 5, 2) => " + formatPortArray(score),
		"数组折叠与格式化([3, 1, 4, 1, 5], 2) => " + formatPortArray(fold),
		"嵌套控制流(0, 4) => " + formatPortArray(nested),
		"局部状态隔离(7) => " + formatPortArray(localFirst),
		"局部状态隔离(7) => " + formatPortArray(localSecond),
	}, "<br>")
}

func appendVerificationMatrixSection(report *bytes.Buffer, title, note string, rows []verificationMatrixRow) {
	fmt.Fprintf(report, "\n## %s\n\n%s\n\n", title, note)
	report.WriteString("| 组 | 入口输入 | 蓝图输出 | Go 参考输出 | 一致 | 内部函数调用（输入 => 返回） |\n")
	report.WriteString("| --- | --- | --- | --- | --- | --- |\n")
	for index, row := range rows {
		fmt.Fprintf(report, "| %d | %s | %s | %s | 是 | %s |\n", index+1, row.input, formatPortArray(row.blueprint), formatPortArray(row.reference), row.functionCalls)
	}
}

func formatIntInputs(input [3]PortInt) string {
	return fmt.Sprintf("对象ID=%d, 参数1=%d, 参数2=%d", input[0], input[1], input[2])
}

func formatPortArray(values PortArray) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		switch {
		case value.StrVal != "":
			parts = append(parts, fmt.Sprintf("%q", value.StrVal))
		case value.BoolVal:
			parts = append(parts, "true")
		case value.FloatVal != 0:
			parts = append(parts, fmt.Sprintf("%g", value.FloatVal))
		default:
			parts = append(parts, fmt.Sprintf("%d", value.IntVal))
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
