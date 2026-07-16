package blueprint

import (
	"errors"
	"fmt"
)

var ErrControlNodeRequiresVM = errors.New("blueprint control node requires VM execution")

// vmMachine 保存一次执行的全部可恢复状态。Program 是不可变共享对象，其他字段只属于当前执行。
type vmMachine struct {
	program        *Program
	graph          *Graph
	pc             PC
	inputPortID    int
	outputArgs     []any
	flowStack      []vmFlowFrame
	loopStack      []vmLoopFrame
	callStack      []CallFrame
	pendingYield   *vmYieldState
	yieldSeq       uint64
	loopSeq        uint64
	err            error
	activeDataNode *ExecNode
}

func newVMMachine(graph *Graph, program *Program) *vmMachine {
	machine := &vmMachine{graph: graph, program: program, pc: InvalidPC}
	if program != nil {
		if program.FlowStackHint > 0 {
			machine.flowStack = make([]vmFlowFrame, 0, program.FlowStackHint)
		}
		if program.LoopStackHint > 0 {
			machine.loopStack = make([]vmLoopFrame, 0, program.LoopStackHint)
		}
	}
	return machine
}

func (g *Graph) runVMEntrance(entranceID int64, args ...any) (result PortArray, runErr error) {
	machine, found, err := g.newVMMachineForEntrance(entranceID, args...)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrEntranceNotFound
	}
	// Graph.Do 是不经过 Execution 的底层入口，需要在边界兜住用户结点 panic。
	// Blueprint/Execution 路径在更外层已有同等保护，VM 热循环本身不承担 defer 成本。
	defer func() {
		if recovered := recover(); recovered != nil {
			result = g.resultSnapshot()
			runErr = machine.recoveredPanicError(BlueprintStageExecute, recovered)
		}
	}()
	if err := machine.run(); err != nil {
		return g.resultSnapshot(), err
	}
	return g.resultSnapshot(), nil
}

func (g *Graph) newVMMachineForEntrance(entranceID int64, args ...any) (*vmMachine, bool, error) {
	if g == nil || g.compiled == nil {
		return nil, false, nil
	}
	program := g.compiled.Program
	if program == nil {
		return nil, false, fmt.Errorf("blueprint VM program is nil")
	}
	pc, ok := program.Entrances[entranceID]
	if !ok {
		return nil, false, nil
	}

	g.initializeVMRun()

	machine := newVMMachine(g, program)
	g.vm = machine
	machine.pc = pc
	machine.outputArgs = args
	return machine, true, nil
}

func (g *Graph) initializeVMRun() {
	g.resetContext()
	clear(g.returns)
	g.returns = g.returns[:0]
	g.returnPort = nil
	g.variables = g.initialVariables()
}

func (m *vmMachine) run() error {
	for m.pc != InvalidPC {
		if m.graph != nil && m.graph.execution != nil {
			if err := m.graph.execution.cancellationError(); err != nil {
				return err
			}
		}
		if m.pc < 0 || int(m.pc) >= len(m.program.Instructions) {
			return fmt.Errorf("blueprint VM pc %d out of range", m.pc)
		}
		instructionPC := m.pc
		instructionGraph := m.graph
		instruction := m.program.Instructions[instructionPC]
		var instructionNode *ExecNode
		if instruction.A >= 0 && int(instruction.A) < len(m.program.Nodes) {
			instructionNode = m.program.Nodes[instruction.A].Node
		}
		switch instruction.Op {
		case OpCallNative:
			nextPort, err := m.callNative(int(instruction.A))
			if err != nil {
				return wrapVMInstructionError(instructionGraph, instructionNode, instructionPC, err)
			}
			m.advanceFromPort(nextPort)
		case OpSequence:
			if err := m.runSequence(int(instruction.A)); err != nil {
				return wrapVMInstructionError(instructionGraph, instructionNode, instructionPC, err)
			}
		case OpRangeLoop, OpArrayLoop, OpWhileLoop, OpBreakableLoop:
			if err := m.runLoop(instruction.Op, int(instruction.A)); err != nil {
				return wrapVMInstructionError(instructionGraph, instructionNode, instructionPC, err)
			}
		case OpCallFunction:
			if err := m.callFunction(int(instruction.A)); err != nil {
				return wrapVMInstructionError(instructionGraph, instructionNode, instructionPC, err)
			}
		case OpReturnFunction:
			if err := m.returnFunction(int(instruction.A)); err != nil {
				return wrapVMInstructionError(instructionGraph, instructionNode, instructionPC, err)
			}
		case OpHalt:
			m.pc = InvalidPC
		default:
			return wrapVMInstructionError(instructionGraph, instructionNode, instructionPC, fmt.Errorf("blueprint VM unsupported opcode %d", instruction.Op))
		}
		if m.err != nil {
			return wrapVMInstructionError(instructionGraph, instructionNode, instructionPC, m.err)
		}
	}
	return m.err
}

func wrapVMInstructionError(graph *Graph, node *ExecNode, pc PC, err error) error {
	if err == nil {
		return nil
	}
	var structured *BlueprintError
	if errors.As(err, &structured) && structured != nil {
		result := *structured
		if result.GraphName == "" && graph != nil {
			result.GraphName = graph.name
		}
		if result.GraphID == 0 && graph != nil {
			result.GraphID = graph.graphID
		}
		if result.NodeID == "" && node != nil {
			result.NodeID = node.ID
			if node.Definition != nil {
				result.NodeName = node.Definition.Name
			}
		}
		if result.PC == InvalidPC {
			result.PC = pc
		}
		return &result
	}
	return newBlueprintNodeError(BlueprintStageExecute, graph, node, pc, err)
}

func (m *vmMachine) recoveredPanicError(stage BlueprintStage, recovered any) error {
	node := m.activeDataNode
	pc := m.pc
	message := "VM panic"
	if node != nil {
		pc = PC(node.Index)
		message = "data producer panic"
	} else if m != nil && m.program != nil && pc >= 0 && int(pc) < len(m.program.Nodes) {
		node = m.program.Nodes[pc].Node
	}
	m.activeDataNode = nil
	return newBlueprintNodeError(stage, m.graph, node, pc, fmt.Errorf("%s: %v", message, recovered))
}

func (m *vmMachine) advanceFromPort(portIndex int) {
	if portIndex >= 0 && int(m.pc) < len(m.program.Nodes) {
		successors := m.program.Nodes[m.pc].Successors
		if portIndex < len(successors) && len(successors[portIndex]) != 0 {
			targets := successors[portIndex]
			m.scheduleTargets(targets)
			return
		}
	}
	m.resumeFlow()
}

func (m *vmMachine) resumeFlow() {
	if len(m.callStack) != 0 {
		frame := &m.callStack[len(m.callStack)-1]
		if len(m.flowStack) == frame.FlowBase {
			m.err = fmt.Errorf("function %s completed without FunctionReturn", frame.FunctionName)
			m.pc = InvalidPC
			return
		}
	}
	if len(m.flowStack) == 0 {
		m.pc = InvalidPC
		m.inputPortID = 0
		m.outputArgs = nil
		return
	}
	last := len(m.flowStack) - 1
	frame := m.flowStack[last]
	m.flowStack = m.flowStack[:last]
	switch frame.kind {
	case vmFlowTarget:
		m.moveTo(frame.target)
	case vmFlowLoopContinue:
		if err := m.continueLoop(frame.loopID); err != nil {
			m.pc = InvalidPC
			m.err = err
		}
	}
}

func (m *vmMachine) scheduleTargets(targets []VMTarget) {
	if len(targets) == 0 {
		m.resumeFlow()
		return
	}
	for index := len(targets) - 1; index >= 1; index-- {
		m.flowStack = append(m.flowStack, vmFlowFrame{kind: vmFlowTarget, target: targets[index]})
	}
	m.moveTo(targets[0])
}

func (m *vmMachine) moveTo(target VMTarget) {
	m.pc = target.PC
	m.inputPortID = target.InputPortID
	m.outputArgs = nil
}

func (m *vmMachine) nodePlan(nodeIndex int) (*NodePlan, error) {
	if nodeIndex < 0 || nodeIndex >= len(m.program.Nodes) {
		return nil, fmt.Errorf("blueprint VM node index %d out of range at pc %d", nodeIndex, m.pc)
	}
	return &m.program.Nodes[nodeIndex], nil
}

func (m *vmMachine) callNative(nodeIndex int) (nextPort int, err error) {
	plan, err := m.nodePlan(nodeIndex)
	if err != nil {
		return -1, err
	}
	node := plan.Node
	defer func() {
		if recovered := recover(); recovered != nil {
			panicNode := node
			panicPC := m.pc
			message := "native panic"
			if m.activeDataNode != nil {
				panicNode = m.activeDataNode
				panicPC = PC(panicNode.Index)
				message = "data producer panic"
				m.activeDataNode = nil
			}
			err = newBlueprintNodeError(BlueprintStageExecute, m.graph, panicNode, panicPC, fmt.Errorf("%s: %v", message, recovered))
			nextPort = -1
		}
	}()
	pendingBefore := m.pendingYield
	nextPort, err = node.executeWithInput(m.graph, m.inputPortID, true, m.outputArgs...)
	pending := m.pendingYield
	yielded := pending != nil && pending != pendingBefore && pending.node == node
	suspended := errors.Is(err, ErrExecutionSuspended)
	if suspended && !yielded {
		return -1, fmt.Errorf("blueprint native node %s failed at pc %d: %w: suspension returned without Yield", node.ID, m.pc, ErrYieldInvalid)
	}
	if yielded && !suspended {
		m.pendingYield = nil
		if err != nil {
			return -1, fmt.Errorf("blueprint native node %s failed at pc %d: %w: Yield returned with %v", node.ID, m.pc, ErrYieldInvalid, err)
		}
		return -1, fmt.Errorf("blueprint native node %s failed at pc %d: %w: Yield returned without suspension", node.ID, m.pc, ErrYieldInvalid)
	}
	if err != nil {
		var structured *BlueprintError
		if errors.As(err, &structured) {
			return -1, structured
		}
		return -1, newBlueprintNodeError(BlueprintStageExecute, m.graph, node, m.pc, err)
	}
	legacyTerminal := nextPort == 0 && len(plan.ExecOutputs) == 0
	validExecOutput := nextPort >= 0 && nextPort < len(plan.ExecOutputs) && plan.ExecOutputs[nextPort]
	if nextPort < -1 || (nextPort >= 0 && !legacyTerminal && !validExecOutput) {
		return -1, newBlueprintNodeError(BlueprintStageExecute, m.graph, node, m.pc, fmt.Errorf("selected invalid exec output %d", nextPort))
	}
	return nextPort, nil
}

func (m *vmMachine) release() {
	if m == nil {
		return
	}
	seen := map[*Graph]struct{}{}
	releaseGraph := func(graph *Graph) {
		if graph == nil {
			return
		}
		if _, ok := seen[graph]; ok {
			return
		}
		seen[graph] = struct{}{}
		graph.releaseContextReferences()
		graph.vm = nil
	}
	releaseGraph(m.graph)
	for index := range m.callStack {
		releaseGraph(m.callStack[index].CallerGraph)
	}
	m.flowStack = nil
	m.loopStack = nil
	m.callStack = nil
	m.pendingYield = nil
}
