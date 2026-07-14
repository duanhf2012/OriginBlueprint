package blueprint

import (
	"sync"
	"testing"
)

func TestCompileVMProgramClassifiesControlAndNativeNodes(t *testing.T) {
	registry := testSystemRegistry(t)
	compiled, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "Entrance_IntParam_1"},
			{ID: "sequence", Class: "Sequence"},
			{ID: "loop", Class: "ForeachIntArray"},
			{ID: "result", Class: "AppendIntReturn", PortDefault: map[int]any{1: 7}},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "sequence", DesPortID: 0},
			{SourceNodeID: "sequence", SourcePortID: 0, DesNodeID: "loop", DesPortID: 0},
			{SourceNodeID: "loop", SourcePortID: 0, DesNodeID: "result", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	if compiled.Program == nil {
		t.Fatal("CompileGraph did not produce VM Program")
	}
	if got, want := len(compiled.Program.Nodes), 4; got != want {
		t.Fatalf("Program nodes = %d, want %d", got, want)
	}
	if got := compiled.Program.Instructions[1].Op; got != OpSequence {
		t.Fatalf("Sequence opcode = %v, want %v", got, OpSequence)
	}
	if got := compiled.Program.Instructions[2].Op; got != OpArrayLoop {
		t.Fatalf("ForeachIntArray opcode = %v, want %v", got, OpArrayLoop)
	}
	if got := compiled.Program.Instructions[3].Op; got != OpCallNative {
		t.Fatalf("AppendIntReturn opcode = %v, want %v", got, OpCallNative)
	}
	if got := compiled.Program.Entrances[1]; got != PC(0) {
		t.Fatalf("entry PC = %d, want 0", got)
	}
}

func TestEnsureVMProgramIsSafeForConcurrentCompatibilityCompilation(t *testing.T) {
	entry := NewExecNode("entry", NewNodeDefinition("VMEntry", func() IExecNode { return &EntranceIntParam{} }, nil, []IPort{NewPortExec()}))
	current := entry
	for index := 0; index < 256; index++ {
		next := NewExecNode("step", NewNodeDefinition("VMPass", func() IExecNode { return &vmPassNode{} }, []IPort{NewPortExec()}, []IPort{NewPortExec()}))
		current.Next = []*ExecNode{next}
		current = next
	}
	compiled := &CompiledGraph{Entrances: map[int64]*ExecNode{1: entry}, NodeCount: 257}
	const workers = 16
	ready := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(workers)
	for range workers {
		go func() {
			defer wait.Done()
			<-ready
			ensureVMProgram(compiled)
		}()
	}
	close(ready)
	wait.Wait()
	if compiled.Program == nil || len(compiled.Program.Instructions) != 257 {
		got := 0
		if compiled.Program != nil {
			got = len(compiled.Program.Instructions)
		}
		t.Fatalf("compiled Program instructions = %d, want 257", got)
	}
}

func TestCompileVMProgramPreservesLegacyFanoutOrder(t *testing.T) {
	registry := testSystemRegistry(t)
	compiled, err := CompileGraph(registry, GraphConfig{
		Legacy: true,
		Nodes: []NodeConfig{
			{ID: "entry", Class: "Entrance_IntParam_1"},
			{ID: "first", Class: "AppendIntReturn", PortDefault: map[int]any{1: 1}},
			{ID: "second", Class: "AppendIntReturn", PortDefault: map[int]any{1: 2}},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "first", DesPortID: 0},
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "second", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	targets := compiled.Program.Nodes[0].Successors[0]
	if len(targets) != 2 || targets[0].PC != 1 || targets[1].PC != 2 {
		t.Fatalf("fanout targets = %#v, want PCs [1 2]", targets)
	}
}
