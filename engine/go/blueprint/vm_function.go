package blueprint

import "fmt"

// SlotBinding 描述函数返回值从 callee 数据槽到 caller 输出槽的固定映射。
type SlotBinding struct {
	Source int
	Target int
}

// CallFrame 保存函数调用前的完整 VM 上下文。
type CallFrame struct {
	CallerProgram *Program
	CallerGraph   *Graph
	ReturnPC      PC
	CallerNode    *ExecNode
	CallerContext *ExecContext
	OutputMap     []SlotBinding
	FlowBase      int
	LoopBase      int
	FunctionName  string
}

func (m *vmMachine) callFunction(nodeIndex int) error {
	if len(m.callStack) >= MaxFunctionCallDepth {
		return fmt.Errorf("maximum function call depth %d exceeded", MaxFunctionCallDepth)
	}
	plan, ctx, err := m.prepareControlNode(nodeIndex)
	if err != nil {
		return err
	}
	node := plan.Node
	functionGraph := node.FunctionGraph
	if functionGraph == nil && node.FunctionID != "" {
		functionGraph = m.graph.compiled.Functions[node.FunctionID]
	}
	if functionGraph == nil && node.FunctionName != "" {
		functionGraph = m.graph.compiled.Functions[node.FunctionName]
	}
	if functionGraph == nil || functionGraph.Program == nil {
		m.graph.releaseContext(node, ctx)
		return fmt.Errorf("function %s not found", vmFunctionLabel(node))
	}
	entryPC, ok := functionGraph.Program.Entrances[FunctionEntranceID]
	if !ok {
		m.graph.releaseContext(node, ctx)
		return fmt.Errorf("function %s entrance not found", vmFunctionLabel(node))
	}

	args := make([]any, 0, len(ctx.InputPorts)-1)
	for index := 1; index < len(ctx.InputPorts); index++ {
		args = append(args, portAnyValue(ctx.InputPorts[index]))
	}
	outputMap := make([]SlotBinding, 0, len(ctx.OutputPorts)-1)
	for index := 1; index < len(ctx.OutputPorts); index++ {
		outputMap = append(outputMap, SlotBinding{Source: index - 1, Target: index})
	}
	frame := CallFrame{
		CallerProgram: m.program,
		CallerGraph:   m.graph,
		ReturnPC:      m.pc,
		CallerNode:    node,
		CallerContext: ctx,
		OutputMap:     outputMap,
		FlowBase:      len(m.flowStack),
		LoopBase:      len(m.loopStack),
		FunctionName:  vmFunctionLabel(node),
	}
	m.callStack = append(m.callStack, frame)

	child := NewGraph(functionGraph)
	child.name = frame.FunctionName
	child.graphID = m.graph.graphID
	child.module = m.graph.module
	child.instance = m.graph.instance
	child.logger = m.graph.logger
	child.trace = m.graph.trace
	child.execution = m.graph.execution
	child.budget = m.graph.budget
	child.initializeVMRun()
	child.vm = m
	m.program = functionGraph.Program
	m.graph = child
	m.pc = entryPC
	m.inputPortID = 0
	m.outputArgs = args
	return nil
}

func (m *vmMachine) returnFunction(nodeIndex int) error {
	plan, ctx, err := m.prepareControlNode(nodeIndex)
	if err != nil {
		return err
	}
	values := make([]any, 0, len(ctx.InputPorts)-1)
	for index := 1; index < len(ctx.InputPorts); index++ {
		values = append(values, portAnyValue(ctx.InputPorts[index]))
	}
	m.graph.releaseContext(plan.Node, ctx)
	if len(m.callStack) == 0 {
		m.graph.returns = m.graph.returns[:0]
		for _, value := range values {
			m.graph.appendReturn(arrayDataFromAny(value))
		}
		m.pc = InvalidPC
		return nil
	}

	frameIndex := len(m.callStack) - 1
	frame := m.callStack[frameIndex]
	m.callStack = m.callStack[:frameIndex]
	if len(m.flowStack) > frame.FlowBase {
		m.flowStack = m.flowStack[:frame.FlowBase]
	}
	for len(m.loopStack) > frame.LoopBase {
		loop := m.loopStack[len(m.loopStack)-1]
		m.loopStack = m.loopStack[:len(m.loopStack)-1]
		m.graph.releaseContext(loop.node, loop.ctx)
	}
	callee := m.graph
	m.program = frame.CallerProgram
	m.graph = frame.CallerGraph
	m.graph.vm = m
	for _, binding := range frame.OutputMap {
		if binding.Source >= len(values) || binding.Target >= len(frame.CallerContext.OutputPorts) {
			continue
		}
		if err := frame.CallerContext.OutputPorts[binding.Target].setAnyValue(values[binding.Source]); err != nil {
			return fmt.Errorf("function %s output %d: %w", frame.FunctionName, binding.Source, err)
		}
	}
	m.graph.releaseContext(frame.CallerNode, frame.CallerContext)
	callee.releaseContextReferences()
	m.pc = frame.ReturnPC
	m.inputPortID = 0
	m.outputArgs = nil
	m.advanceFromPort(0)
	return nil
}

func vmFunctionLabel(node *ExecNode) string {
	if node == nil {
		return ""
	}
	if node.FunctionID != "" {
		return node.FunctionID
	}
	return node.FunctionName
}
