package blueprint

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
)

func TestVerificationControlFlowMazeMatchesReference(t *testing.T) {
	graph := loadVerificationGraph(t, "02_control_flow_maze.obp")
	returns, err := NewGraph(graph).Do(EntranceIDIntParam, PortInt(1), PortInt(2), PortInt(3))
	if err != nil {
		t.Fatal(err)
	}
	assertVerificationReturns(t, returns, referenceControlFlowMaze())
}

func TestVerificationArrayDataLabMatchesReference(t *testing.T) {
	graph := loadVerificationGraph(t, "03_array_data_lab.obp")
	returns, err := NewGraph(graph).Do(EntranceIDArrayParam, PortInt(1), PortArray{})
	if err != nil {
		t.Fatal(err)
	}
	assertVerificationReturns(t, returns, referenceArrayDataLab())
}

func referenceControlFlowMaze() PortArray {
	returns := make(PortArray, 0, 20)
	for outer := PortInt(0); outer < 3; outer++ {
		for _, value := range []PortInt{2, 4, 6} {
			returns = append(returns, ArrayData{IntVal: outer + value})
		}
	}
	returns = append(returns, ArrayData{StrVal: "range-branch-true"})
	for index := PortInt(0); index <= 2; index++ {
		returns = append(returns, ArrayData{IntVal: index})
	}
	returns = append(returns,
		ArrayData{StrVal: "break-loop-complete"},
		ArrayData{StrVal: "probability-hit"},
		ArrayData{StrVal: "comparison-switch-complete"},
	)
	for _, value := range []PortString{"alpha", "beta", "gamma"} {
		returns = append(returns, ArrayData{StrVal: PortString(fmt.Sprint(ArrayData{StrVal: value}))})
	}
	return append(returns, ArrayData{StrVal: "control-flow-complete"})
}

func referenceArrayDataLab() PortArray {
	integers := []PortInt{3, 1, 4, 1, 5}
	arraySum := integers[2]
	integers = append(integers, 9)
	strings := []PortString{"red", "green", "blue"}
	strings = append(strings, "violet")
	return PortArray{
		{IntVal: arraySum},
		{IntVal: PortInt(len(integers))},
		{IntVal: arraySum},
		{StrVal: "green"},
		{StrVal: strings[3]},
		{StrVal: PortString(fmt.Sprint(ArrayData{StrVal: "north"}))},
	}
}

func TestVerificationDeterministicAlgorithmMatchesReference(t *testing.T) {
	graph := loadVerificationGraph(t, "04_deterministic_algorithm.obp")
	testVerificationDeterministicAlgorithm(t, graph, PortInt(10), PortInt(5))
}

func TestVerificationDeterministicAlgorithmRandomInputsMatchReference(t *testing.T) {
	graph := loadVerificationGraph(t, "04_deterministic_algorithm.obp")
	cases := [][2]PortInt{{0, 0}, {-4, 7}, {2, 3}, {10, 5}, {11, 12}}
	random := rand.New(rand.NewSource(20260712))
	for range 32 {
		cases = append(cases, [2]PortInt{PortInt(random.Intn(201) - 100), PortInt(random.Intn(201) - 100)})
	}
	for _, testCase := range cases {
		testVerificationDeterministicAlgorithm(t, graph, testCase[0], testCase[1])
	}
}

func testVerificationDeterministicAlgorithm(t *testing.T, graph *CompiledGraph, first, second PortInt) {
	t.Helper()
	returns, err := NewGraph(graph).Do(EntranceIDIntParam, PortInt(1), first, second)
	if err != nil {
		t.Fatalf("Do(%d, %d): %v", first, second, err)
	}
	assertVerificationReturns(t, returns, referenceDeterministicAlgorithm(first, second))
}

func referenceDeterministicAlgorithm(first, second PortInt) PortArray {
	score := ((first + second) - 3) * 2 / 3
	branch := "score-low"
	if score > 10 {
		branch = "score-high"
	}
	return PortArray{
		{IntVal: score}, {IntVal: score % 7}, {IntVal: 42},
		{StrVal: PortString(branch)}, {StrVal: "range-case-3"}, {StrVal: "switch-case-2"}, {StrVal: "5"},
	}
}

func TestVerificationFunctionOrchestratorMatchesReference(t *testing.T) {
	graphs := loadVerificationFixtureSet(t)
	returns, err := NewGraph(graphs["函数编排主图"]).Do(EntranceIDIntParam, PortInt(1), PortInt(2), PortInt(3))
	if err != nil {
		t.Fatal(err)
	}
	assertVerificationReturns(t, returns, referenceFunctionOrchestrator())
}

func TestVerificationScoreKernelRandomInputsMatchReference(t *testing.T) {
	graphs := loadVerificationFixtureSet(t)
	function := verificationFixtureFunction(t, graphs, "functions/10_score_kernel.obpf")
	cases := [][3]PortInt{{0, 0, 1}, {10, 5, 2}, {-4, 7, 3}, {13, -2, 4}}
	random := rand.New(rand.NewSource(20260713))
	for range 32 {
		cases = append(cases, [3]PortInt{
			PortInt(random.Intn(101) - 50),
			PortInt(random.Intn(101) - 50),
			PortInt(random.Intn(9) - 4),
		})
	}
	for _, testCase := range cases {
		returns, err := NewGraph(function).Do(FunctionEntranceID, testCase[0], testCase[1], testCase[2])
		if err != nil {
			t.Fatalf("Do(%d, %d, %d): %v", testCase[0], testCase[1], testCase[2], err)
		}
		score, tier := referenceScoreKernel(testCase[0], testCase[1], testCase[2])
		assertVerificationReturns(t, returns, PortArray{{IntVal: score}, {StrVal: tier}})
	}
}

func TestVerificationArrayFoldAndFormatRandomInputsMatchReference(t *testing.T) {
	graphs := loadVerificationFixtureSet(t)
	function := verificationFixtureFunction(t, graphs, "functions/11_array_fold_and_format.obpf")
	cases := []struct {
		items  PortArray
		weight PortInt
	}{
		{PortArray{{IntVal: 3}, {IntVal: 1}, {IntVal: 4}, {IntVal: 1}, {IntVal: 5}}, 2},
		{PortArray{{IntVal: -3}, {IntVal: 0}, {IntVal: 7}}, -2},
	}
	random := rand.New(rand.NewSource(20260714))
	for range 32 {
		items := make(PortArray, random.Intn(6)+2)
		for index := range items {
			items[index] = ArrayData{IntVal: PortInt(random.Intn(41) - 20)}
		}
		cases = append(cases, struct {
			items  PortArray
			weight PortInt
		}{items: items, weight: PortInt(random.Intn(11) - 5)})
	}
	for _, testCase := range cases {
		returns, err := NewGraph(function).Do(FunctionEntranceID, testCase.items, testCase.weight)
		if err != nil {
			t.Fatalf("Do(%#v, %d): %v", testCase.items, testCase.weight, err)
		}
		items := make([]PortInt, len(testCase.items))
		for index, item := range testCase.items {
			items[index] = item.IntVal
		}
		sum, summary := referenceArrayFoldAndFormat(items, testCase.weight)
		assertVerificationReturns(t, returns, PortArray{{IntVal: sum}, {StrVal: summary}})
	}
}

func referenceFunctionOrchestrator() PortArray {
	score, tier := referenceScoreKernel(10, 5, 2)
	arraySum, summary := referenceArrayFoldAndFormat([]PortInt{3, 1, 4, 1, 5}, 2)
	count, trace := referenceNestedControlFlow()
	return PortArray{
		{IntVal: score}, {StrVal: tier},
		{IntVal: arraySum}, {StrVal: summary},
		{IntVal: count}, {StrVal: trace},
		{IntVal: referenceLocalStateIsolation(7)},
		{IntVal: referenceLocalStateIsolation(7)},
	}
}

func referenceScoreKernel(base, bonus, multiplier PortInt) (PortInt, PortString) {
	return (base + bonus) * multiplier, "gold"
}

func referenceArrayFoldAndFormat(items []PortInt, weight PortInt) (PortInt, PortString) {
	var sum PortInt
	for _, item := range items {
		sum += item * weight
	}
	nestedScore, _ := referenceScoreKernel(5, 3, 2)
	return sum, PortString(strconv.FormatInt(int64(nestedScore), 10))
}

func referenceNestedControlFlow() (PortInt, PortString) {
	for outer := 0; outer <= 3; outer++ {
		for _, inner := range []int{1, 2, 3} {
			_ = outer + inner
		}
	}
	for index := 0; index <= 3; index++ {
		if index > 1 {
			break
		}
	}
	for condition := false; condition; {
	}
	return 3 + 6, "nested-control:complete"
}

func referenceLocalStateIsolation(seed PortInt) PortInt {
	var callCounter PortInt
	callCounter += seed
	return callCounter
}

type verificationMockRPCNode struct{ BaseExecNode }

type verificationMockRPCPendingCall struct {
	handle  *YieldHandle
	branch  int
	outputs []any
}

var verificationMockRPCPending struct {
	sync.Mutex
	calls []*verificationMockRPCPendingCall
}

func (n *verificationMockRPCNode) GetName() string { return "MockRpcAsync" }

func (n *verificationMockRPCNode) Exec() (int, error) {
	_, _ = n.GetInPortInt(1)
	succeed, _ := n.GetInPortBool(2)
	value, _ := n.GetInPortInt(3)
	code, _ := n.GetInPortInt(4)
	message, _ := n.GetInPortStr(5)
	branch := 1
	outputs := []any{PortInt(0), code, message}
	if succeed {
		branch = 0
		outputs = []any{value, PortInt(0), PortString("")}
	}
	handle, err := n.Yield(branch)
	if err != nil {
		return -1, err
	}
	verificationMockRPCPending.Lock()
	verificationMockRPCPending.calls = append(verificationMockRPCPending.calls, &verificationMockRPCPendingCall{handle: handle, branch: branch, outputs: outputs})
	verificationMockRPCPending.Unlock()
	return -1, ErrExecutionSuspended
}

func TestVerificationMockRPCResumeToMatchesReference(t *testing.T) {
	registry := verificationFixtureRegistry(t)
	graph := loadVerificationGraphWithRegistry(t, "07_async_rpc_resume_to.obp", registry)
	dispatcher := &manualExecutionDispatcher{}
	verificationMockRPCPending.Lock()
	verificationMockRPCPending.calls = nil
	verificationMockRPCPending.Unlock()
	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.AddCompiledGraph("定时器模拟 RPC 异步恢复", graph)
	execution, err := bp.Start(context.Background(), bp.Create("定时器模拟 RPC 异步恢复"), EntranceIDIntParam, PortInt(1), PortInt(2), PortInt(3))
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.runNext(t)
	resumeVerificationMockRPC(t)
	dispatcher.runNext(t)
	resumeVerificationMockRPC(t)
	dispatcher.runNext(t)
	returns, err := execution.Result()
	if err != nil {
		t.Fatal(err)
	}
	assertVerificationReturns(t, returns, PortArray{{IntVal: 314}, {IntVal: 503}, {StrVal: "mock rpc unavailable"}})
}

func resumeVerificationMockRPC(t *testing.T) {
	t.Helper()
	verificationMockRPCPending.Lock()
	if len(verificationMockRPCPending.calls) == 0 {
		verificationMockRPCPending.Unlock()
		t.Fatal("mock RPC has no pending call")
	}
	call := verificationMockRPCPending.calls[0]
	verificationMockRPCPending.calls = verificationMockRPCPending.calls[1:]
	verificationMockRPCPending.Unlock()
	if err := call.handle.ResumeTo(call.branch, call.outputs...); err != nil {
		t.Fatalf("mock RPC ResumeTo failed: %v", err)
	}
}

func verificationFixtureRegistry(t *testing.T) *Registry {
	return verificationFixtureRegistryWithClock(t, nil)
}

func verificationFixtureRegistryWithClock(t *testing.T, clock *verificationFakeClock) *Registry {
	t.Helper()
	registry := testSystemRegistry(t)
	registry.Register(NewNodeDefinition("MockRpcAsync", func() IExecNode { return &verificationMockRPCNode{} }, []IPort{NewPortExec(), NewPortInt(), NewPortBool(), NewPortInt(), NewPortInt(), NewPortStr()}, []IPort{NewPortExec(), NewPortExec(), NewPortInt(), NewPortInt(), NewPortStr()}))
	registry.Register(NewNodeDefinition("MockDelayAsync", func() IExecNode { return &verificationMockDelayNode{clock: clock} }, []IPort{NewPortExec(), NewPortInt(), NewPortInt(), NewPortStr()}, []IPort{NewPortExec(), NewPortInt(), NewPortStr()}))
	return registry
}

func loadVerificationGraph(t *testing.T, fileName string) *CompiledGraph {
	return loadVerificationGraphWithRegistry(t, fileName, testSystemRegistry(t))
}

func loadVerificationGraphWithRegistry(t *testing.T, fileName string, registry *Registry) *CompiledGraph {
	t.Helper()
	source := filepath.Join("..", "..", "..", "examples", "verification-blueprints", fileName)
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, fileName), data, 0644); err != nil {
		t.Fatal(err)
	}
	graphs, err := loadGraphDir(registry, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, graph := range graphs {
		return graph
	}
	t.Fatalf("%s did not load a graph", fileName)
	return nil
}

func loadVerificationFixtureSet(t *testing.T) map[string]*CompiledGraph {
	return loadVerificationFixtureSetWithRegistry(t, verificationFixtureRegistry(t))
}

func loadVerificationFixtureSetWithRegistry(t *testing.T, registry *Registry) map[string]*CompiledGraph {
	t.Helper()
	source := filepath.Join("..", "..", "..", "examples", "verification-blueprints")
	root := t.TempDir()
	for _, relative := range []string{
		"05_function_orchestrator.obp",
		"06_async_delay_resume.obp",
		"functions/10_score_kernel.obpf",
		"functions/11_array_fold_and_format.obpf",
		"functions/12_nested_control_function.obpf",
		"functions/13_local_state_isolation.obpf",
		"functions/14_async_delay_function.obpf",
	} {
		data, err := os.ReadFile(filepath.Join(source, relative))
		if err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	graphs, err := loadGraphDir(registry, root)
	if err != nil {
		t.Fatal(err)
	}
	return graphs
}

func verificationFixtureFunction(t *testing.T, graphs map[string]*CompiledGraph, functionID string) *CompiledGraph {
	t.Helper()
	for _, graph := range graphs {
		if function := graph.Functions[functionID]; function != nil {
			return function
		}
	}
	t.Fatalf("function fixture %s was not loaded", functionID)
	return nil
}

func assertVerificationReturns(t *testing.T, got, want PortArray) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("returns=%#v, want=%#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("returns[%d]=%#v, want=%#v; all=%#v", index, got[index], want[index], got)
		}
	}
}
