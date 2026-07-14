package blueprint

import (
	"sync"
	"sync/atomic"
)

var vmProgramVersion atomic.Uint64
var vmCompatibilityCompileMu sync.Mutex

func compileVMProgram(compiled *CompiledGraph) *Program {
	if compiled == nil {
		return nil
	}
	program := &Program{
		Version:      vmProgramVersion.Add(1),
		Instructions: make([]Instruction, len(compiled.Nodes)),
		Nodes:        make([]NodePlan, len(compiled.Nodes)),
		Entrances:    make(map[int64]PC, len(compiled.Entrances)),
		Functions:    make(map[string]*Program, len(compiled.Functions)),
		Variables:    compiled.Variables,
	}
	for index, node := range compiled.Nodes {
		kind := ControlNative
		if node != nil && node.Definition != nil {
			kind = node.Definition.ControlKind
		}
		program.Instructions[index] = Instruction{Op: opcodeForControl(kind), A: int32(index)}
		program.Nodes[index] = NodePlan{Node: node, Control: kind, Successors: compileVMSuccessors(node)}
	}
	program.FlowStackHint, program.LoopStackHint = compileVMStackHints(program.Nodes)
	for entranceID, node := range compiled.Entrances {
		if node != nil && node.Index >= 0 {
			program.Entrances[entranceID] = PC(node.Index)
		}
	}
	for key, function := range compiled.Functions {
		if function != nil {
			program.Functions[key] = function.Program
		}
	}
	return program
}

func compileVMStackHints(nodes []NodePlan) (flowHint, loopHint int) {
	for index := range nodes {
		plan := &nodes[index]
		switch plan.Control {
		case ControlRangeLoop, ControlArrayLoop, ControlWhileLoop, ControlBreakableLoop:
			loopHint++
			flowHint++
		case ControlSequence:
			branches := 0
			for _, successors := range plan.Successors {
				if len(successors) != 0 {
					branches++
				}
			}
			if branches > 1 {
				flowHint += branches - 1
			}
		}
		for _, successors := range plan.Successors {
			if len(successors) > 1 {
				flowHint += len(successors) - 1
			}
		}
	}
	return min(flowHint, 64), min(loopHint, 16)
}

func compileVMSuccessors(node *ExecNode) [][]VMTarget {
	if node == nil || node.Definition == nil {
		return nil
	}
	targets := make([][]VMTarget, len(node.Definition.OutPorts))
	for output := range targets {
		if output < len(node.legacyFanout) && len(node.legacyFanout[output]) != 0 {
			targets[output] = make([]VMTarget, 0, len(node.legacyFanout[output]))
			for _, target := range node.legacyFanout[output] {
				if target.node != nil {
					targets[output] = append(targets[output], VMTarget{PC: PC(target.node.Index), InputPortID: target.inputPortID})
				}
			}
			continue
		}
		if output < len(node.Next) && node.Next[output] != nil {
			inputPortID := 0
			if output < len(node.NextInPort) {
				inputPortID = node.NextInPort[output]
			}
			targets[output] = []VMTarget{{PC: PC(node.Next[output].Index), InputPortID: inputPortID}}
		}
	}
	return targets
}

// ensureVMProgram 仅兼容通过 AddCompiledGraph 手工构造的旧宿主图。
// 正常加载路径始终由 CompileGraph 直接生成 Program。
func ensureVMProgram(compiled *CompiledGraph) {
	vmCompatibilityCompileMu.Lock()
	defer vmCompatibilityCompileMu.Unlock()
	ensureVMProgramRecursive(compiled, map[*CompiledGraph]bool{})
}

func ensureVMProgramRecursive(compiled *CompiledGraph, visiting map[*CompiledGraph]bool) {
	if compiled == nil || compiled.Program != nil || visiting[compiled] {
		return
	}
	visiting[compiled] = true
	for _, function := range compiled.Functions {
		ensureVMProgramRecursive(function, visiting)
	}
	visited := map[*ExecNode]bool{}
	nodes := make([]*ExecNode, 0, compiled.NodeCount)
	var visit func(*ExecNode)
	visit = func(node *ExecNode) {
		if node == nil || node.Definition == nil || visited[node] {
			return
		}
		visited[node] = true
		node.Index = len(nodes)
		nodes = append(nodes, node)
		for _, pre := range node.PreInPort {
			if pre != nil {
				visit(pre.Node)
			}
		}
		for _, next := range node.Next {
			visit(next)
		}
		for _, targets := range node.legacyFanout {
			for _, target := range targets {
				visit(target.node)
			}
		}
	}
	for _, entrance := range compiled.Entrances {
		visit(entrance)
	}
	compiled.Nodes = nodes
	compiled.NodeCount = len(nodes)
	compiled.Program = compileVMProgram(compiled)
	delete(visiting, compiled)
}
