package blueprint

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

// ErrExecutionSuspended 表示节点主动暂停，等待异步回调恢复。
var ErrExecutionSuspended = errors.New("golang blueprint execution suspended")

// ErrFunctionReturned 表示函数图已经执行到返回节点。
var ErrFunctionReturned = errors.New("golang blueprint function returned")

// ErrLoopBreak 表示循环节点收到 break 控制流。
var ErrLoopBreak = errors.New("golang blueprint loop break")

// IExecNode 是所有执行节点需要实现的接口。
type IExecNode interface {
	GetName() string
	Exec() (int, error)
}

// ExecContext 保存单个节点本次执行的输入、输出端口。
type ExecContext struct {
	InputPorts      []IPort
	OutputPorts     []IPort
	ExecInputPortID int
	state           uint64
}

const (
	execContextActiveBit      = uint64(1) << 63
	execContextGenerationMask = execContextActiveBit - 1
)

// BaseExecNode 提供节点实现常用的端口访问和执行辅助方法。
type BaseExecNode struct {
	graph *Graph
	node  *ExecNode
	ctx   *ExecContext
}

func (n *BaseExecNode) bind(graph *Graph, node *ExecNode, ctx *ExecContext) {
	n.graph = graph
	n.node = node
	n.ctx = ctx
}

func (n *BaseExecNode) GetInPort(index int) IPort {
	if n == nil || n.ctx == nil || index < 0 || index >= len(n.ctx.InputPorts) {
		return nil
	}
	return n.ctx.InputPorts[index]
}

func (n *BaseExecNode) GetInPortInt(index int) (PortInt, bool) {
	port := n.GetInPort(index)
	if port == nil {
		return 0, false
	}
	return port.GetInt()
}

func (n *BaseExecNode) GetInPortFloat(index int) (PortFloat, bool) {
	port := n.GetInPort(index)
	if port == nil {
		return 0, false
	}
	return port.GetFloat()
}

func (n *BaseExecNode) GetInPortStr(index int) (PortString, bool) {
	port := n.GetInPort(index)
	if port == nil {
		return "", false
	}
	return port.GetStr()
}

func (n *BaseExecNode) GetInPortBool(index int) (PortBool, bool) {
	port := n.GetInPort(index)
	if port == nil {
		return false, false
	}
	return port.GetBool()
}

func (n *BaseExecNode) GetInPortArray(index int) (PortArray, bool) {
	port := n.GetInPort(index)
	if port == nil {
		return nil, false
	}
	return port.GetArray()
}

func (n *BaseExecNode) GetInPortTimerHandle(index int) (TimerHandle, bool) {
	port := n.GetInPort(index)
	if port == nil {
		return TimerHandle{}, false
	}
	return port.GetTimerHandle()
}

func (n *BaseExecNode) GetOutPort(index int) IPort {
	if n == nil || n.ctx == nil || index < 0 || index >= len(n.ctx.OutputPorts) {
		return nil
	}
	return n.ctx.OutputPorts[index]
}

func (n *BaseExecNode) SetOutPortInt(index int, value PortInt) bool {
	port := n.GetOutPort(index)
	if port == nil {
		return false
	}
	return port.setAnyValue(value) == nil
}

func (n *BaseExecNode) SetOutPortFloat(index int, value PortFloat) bool {
	port := n.GetOutPort(index)
	if port == nil {
		return false
	}
	return port.SetFloat(value)
}

func (n *BaseExecNode) SetOutPortStr(index int, value PortString) bool {
	port := n.GetOutPort(index)
	if port == nil {
		return false
	}
	return port.SetStr(value)
}

func (n *BaseExecNode) SetOutPortBool(index int, value PortBool) bool {
	port := n.GetOutPort(index)
	if port == nil {
		return false
	}
	return port.SetBool(value)
}

func (n *BaseExecNode) SetOutPortTimerHandle(index int, value TimerHandle) bool {
	port := n.GetOutPort(index)
	if port == nil {
		return false
	}
	return port.SetTimerHandle(value)
}

func (n *BaseExecNode) SetOutPort(index int, value IPort) bool {
	port := n.GetOutPort(index)
	if port == nil || value == nil {
		return false
	}
	port.SetValue(value)
	return true
}

func (n *BaseExecNode) GetOutPortInt(index int) (PortInt, bool) {
	port := n.GetOutPort(index)
	if port == nil {
		return 0, false
	}
	return port.GetInt()
}

func (n *BaseExecNode) GetOutPortFloat(index int) (PortFloat, bool) {
	port := n.GetOutPort(index)
	if port == nil {
		return 0, false
	}
	return port.GetFloat()
}

func (n *BaseExecNode) GetOutPortStr(index int) (PortString, bool) {
	port := n.GetOutPort(index)
	if port == nil {
		return "", false
	}
	return port.GetStr()
}

func (n *BaseExecNode) GetOutPortBool(index int) (PortBool, bool) {
	port := n.GetOutPort(index)
	if port == nil {
		return false, false
	}
	return port.GetBool()
}

func (n *BaseExecNode) AppendOutPortArrayValInt(index int, value PortInt) bool {
	port := n.GetOutPort(index)
	if port == nil {
		return false
	}
	return port.AppendArrayValInt(value)
}

func (n *BaseExecNode) AppendOutPortArrayValStr(index int, value PortString) bool {
	port := n.GetOutPort(index)
	if port == nil {
		return false
	}
	return port.AppendArrayValStr(value)
}

func (n *BaseExecNode) AppendInPortArrayValInt(index int, value PortInt) bool {
	port := n.GetInPort(index)
	if port == nil {
		return false
	}
	return port.AppendArrayValInt(value)
}

func (n *BaseExecNode) AppendInPortArrayValStr(index int, value PortString) bool {
	port := n.GetInPort(index)
	if port == nil {
		return false
	}
	return port.AppendArrayValStr(value)
}

func (n *BaseExecNode) GetOutPortCount() int {
	if n == nil || n.ctx == nil {
		return 0
	}
	return len(n.ctx.OutputPorts)
}

func (n *BaseExecNode) DoNext(index int) error {
	if n == nil || n.node == nil || n.graph == nil {
		return fmt.Errorf("node is not executing")
	}
	return n.node.doNext(n.graph, index)
}

func (n *BaseExecNode) GetVariableName() string {
	if n == nil || n.node == nil {
		return ""
	}
	return n.node.VariableName
}

func (n *BaseExecNode) GetBlueprintModule() IBlueprintModule {
	if n == nil || n.graph == nil {
		return nil
	}
	return n.graph.module
}

func (n *BaseExecNode) GetBluePrintModule() IBlueprintModule {
	return n.GetBlueprintModule()
}

func (n *BaseExecNode) GetAndCreateReturnPort() IPort {
	if n == nil || n.graph == nil {
		return nil
	}
	return n.graph.getAndCreateReturnPort()
}

// NodeDefinition 是节点的只读定义。
//
// 编译阶段会预计算数据输入端口等索引，执行期直接复用。
type NodeDefinition struct {
	Name                   string
	New                    func() IExecNode
	InPorts                []IPort
	OutPorts               []IPort
	OutPortParamStartIndex int
	DataInPortIndexes      []int
}

// NewNodeDefinition 创建节点定义，并复制端口模板。
func NewNodeDefinition(name string, newExec func() IExecNode, inPorts []IPort, outPorts []IPort) *NodeDefinition {
	clonedInPorts := clonePorts(inPorts)
	clonedOutPorts := clonePorts(outPorts)
	return &NodeDefinition{
		Name:                   name,
		New:                    newExec,
		InPorts:                clonedInPorts,
		OutPorts:               clonedOutPorts,
		OutPortParamStartIndex: firstDataOutPort(outPorts),
		DataInPortIndexes:      dataInPortIndexes(clonedInPorts),
	}
}

func (d *NodeDefinition) cloneContext() *ExecContext {
	return &ExecContext{
		InputPorts:  clonePorts(d.InPorts),
		OutputPorts: clonePorts(d.OutPorts),
	}
}

func (c *ExecContext) reset(definition *NodeDefinition) {
	if c == nil || definition == nil {
		return
	}
	c.ExecInputPortID = 0
	c.InputPorts = resetContextPorts(c.InputPorts, definition.InPorts)
	c.OutputPorts = resetContextPorts(c.OutputPorts, definition.OutPorts)
}

func resetContextPorts(targets []IPort, templates []IPort) []IPort {
	if cap(targets) < len(templates) {
		targets = make([]IPort, len(templates))
	} else {
		targets = targets[:len(templates)]
	}
	for index, template := range templates {
		if template == nil {
			targets[index] = nil
			continue
		}
		target, targetBuiltin := targets[index].(*Port)
		source, sourceBuiltin := template.(*Port)
		if targetBuiltin && sourceBuiltin && target != nil && source != nil {
			arrayStorage := target.arrv[:0]
			*target = *source
			target.arrv = append(arrayStorage, source.arrv...)
			target.anyv = cloneAnyValue(source.anyv)
			targets[index] = target
			continue
		}
		targets[index] = template.Clone()
	}
	return targets
}

// PrePortNode 记录某个输入端口连接的上游数据节点。
type PrePortNode struct {
	Node      *ExecNode
	OutPortID int
}

// InputBindingKind 表示输入端口取值来源。
type InputBindingKind uint8

const (
	// InputBindingDefault 表示输入端口使用默认值。
	InputBindingDefault InputBindingKind = iota + 1
	// InputBindingProducer 表示输入端口从上游输出端口取值。
	InputBindingProducer
)

// InputBinding 是编译阶段预处理出的输入端口绑定。
//
// 执行期按绑定信息直接取值，减少临时 map 查找。
type InputBinding struct {
	Kind              InputBindingKind
	InputPortID       int
	DefaultPort       IPort
	Producer          *ExecNode
	ProducerOutPortID int
	RecomputeProducer bool
}

type execTarget struct {
	node        *ExecNode
	inputPortID int
}

// ExecNode 是编译后的可执行节点。
//
// 它只保存只读连接信息；每次执行的端口值放在 ExecContext 中。
type ExecNode struct {
	ID              string
	Index           int
	Definition      *NodeDefinition
	Next            []*ExecNode
	NextInPort      []int
	legacyFanout    [][]execTarget
	PreInPort       []*PrePortNode
	DefaultIn       map[int]any
	DefaultInputs   []IPort
	DefaultInputSet []bool
	InputBindings   []InputBinding
	VariableName    string
	FunctionID      string
	FunctionName    string
	FunctionGraph   *CompiledGraph
	BeConnect       bool
	IsEntrance      bool
}

// NewExecNode 创建编译期节点对象。
func NewExecNode(id string, definition *NodeDefinition) *ExecNode {
	return &ExecNode{
		ID:         id,
		Index:      -1,
		Definition: definition,
		PreInPort:  make([]*PrePortNode, len(definition.InPorts)),
		DefaultIn:  map[int]any{},
	}
}

// Do 从节点默认执行输入口开始执行。
//
// outPortArgs 会写入本节点输出数据端口，用于入口和异步恢复场景。
func (n *ExecNode) Do(graph *Graph, outPortArgs ...any) error {
	return n.doWithInput(graph, 0, outPortArgs...)
}

func (n *ExecNode) doWithInput(graph *Graph, execInputPortID int, outPortArgs ...any) error {
	nextIndex, err := n.executeWithInput(graph, execInputPortID, true, outPortArgs...)
	if err != nil {
		return err
	}
	return n.doNext(graph, nextIndex)
}

func (n *ExecNode) executeWithInput(graph *Graph, execInputPortID int, recomputeInputs bool, outPortArgs ...any) (int, error) {
	if graph != nil && graph.execution != nil {
		if err := graph.execution.cancellationError(); err != nil {
			return -1, err
		}
	}
	if n == nil || n.Definition == nil {
		return -1, fmt.Errorf("exec node is invalid")
	}
	if err := graph.enterStep(); err != nil {
		return -1, fmt.Errorf("node %s: %w", n.ID, err)
	}
	defer graph.leaveStep()
	ctx := graph.acquireContext(n)
	releaseContext := true
	defer func() {
		if releaseContext {
			graph.releaseContext(n, ctx)
		}
	}()
	ctx.ExecInputPortID = execInputPortID
	if n.Index < 0 {
		graph.setContext(n, ctx)
	}
	graph.clearSuspendedContinuation()

	if err := n.applyOutputArgs(ctx, outPortArgs...); err != nil {
		return -1, err
	}
	if len(n.InputBindings) != 0 {
		for _, binding := range n.InputBindings {
			if err := n.applyInputBinding(graph, ctx, binding, recomputeInputs); err != nil {
				return -1, err
			}
		}
	} else {
		for _, index := range n.Definition.DataInPortIndexes {
			inPort := ctx.InputPorts[index]
			if err := n.setInPort(graph, ctx, index, inPort, recomputeInputs); err != nil {
				return -1, err
			}
		}
	}

	exec := n.Definition.New()
	if binder, ok := exec.(interface {
		bind(*Graph, *ExecNode, *ExecContext)
	}); ok {
		binder.bind(graph, n, ctx)
	}

	nextIndex, err := exec.Exec()
	graph.logLegacyNode(n, ctx, nextIndex, err)
	graph.traceNode(n, ctx, nextIndex, err)
	if err != nil {
		if errors.Is(err, ErrExecutionSuspended) {
			releaseContext = false
		}
		return -1, err
	}
	return nextIndex, nil
}

func (n *ExecNode) doNext(graph *Graph, index int) error {
	return n.doNextWithPending(graph, index, nil)
}

func (n *ExecNode) doNextWithPending(graph *Graph, index int, inherited []execTarget) error {
	current := n
	pending := append([]execTarget(nil), inherited...)
	for {
		if graph != nil && graph.execution != nil {
			if err := graph.execution.cancellationError(); err != nil {
				return err
			}
		}
		if index < -1 {
			return fmt.Errorf("next index %d not found", index)
		}

		target, remaining, found := current.nextExecTarget(index)
		if !found {
			if len(pending) == 0 {
				return nil
			}
			target = pending[len(pending)-1]
			pending = pending[:len(pending)-1]
		} else {
			for remainingIndex := len(remaining) - 1; remainingIndex >= 0; remainingIndex-- {
				pending = append(pending, remaining[remainingIndex])
			}
		}

		current = target.node
		nextIndex, err := current.executeWithInput(graph, target.inputPortID, true)
		if err != nil {
			if errors.Is(err, ErrExecutionSuspended) && len(pending) != 0 && graph != nil {
				if continuation := graph.currentSuspendedContinuation(); continuation != nil {
					continuation.prependPending(pending)
				}
			}
			return err
		}
		index = nextIndex
	}
}

func (n *ExecNode) nextExecTarget(index int) (execTarget, []execTarget, bool) {
	if index < 0 {
		return execTarget{}, nil, false
	}
	if index < len(n.legacyFanout) && len(n.legacyFanout[index]) != 0 {
		targets := n.legacyFanout[index]
		return targets[0], targets[1:], true
	}
	if index >= len(n.Next) || n.Next[index] == nil {
		return execTarget{}, nil, false
	}
	inputPortID := 0
	if index < len(n.NextInPort) {
		inputPortID = n.NextInPort[index]
	}
	return execTarget{node: n.Next[index], inputPortID: inputPortID}, nil, true
}

func (n *ExecNode) refreshInput(graph *Graph, ctx *ExecContext, inputPortID int) error {
	if inputPortID < 0 || inputPortID >= len(ctx.InputPorts) {
		return fmt.Errorf("node %s input port index %d not found", n.ID, inputPortID)
	}
	for _, binding := range n.InputBindings {
		if binding.InputPortID == inputPortID {
			return n.applyInputBinding(graph, ctx, binding, true)
		}
	}
	return n.setInPort(graph, ctx, inputPortID, ctx.InputPorts[inputPortID], true)
}

func (n *ExecNode) applyOutputArgs(ctx *ExecContext, outPortArgs ...any) error {
	start := n.Definition.OutPortParamStartIndex
	for index, arg := range outPortArgs {
		portIndex := index + start
		if portIndex < 0 || portIndex >= len(ctx.OutputPorts) {
			return fmt.Errorf("args %d not found in node %s", index, n.Definition.Name)
		}
		if err := ctx.OutputPorts[portIndex].setAnyValue(arg); err != nil {
			return fmt.Errorf("args %d set value error: %w", index, err)
		}
	}
	return nil
}

func (n *ExecNode) setInPort(graph *Graph, ctx *ExecContext, index int, inPort IPort, recompute bool) error {
	pre := n.PreInPort[index]
	if pre == nil {
		if index < len(n.DefaultInputSet) && n.DefaultInputSet[index] {
			return assignPortValue(inPort, n.DefaultInputs[index])
		}
		if value, ok := n.DefaultIn[index]; ok {
			return inPort.setAnyValue(value)
		}
		return nil
	}

	// 纯数据节点可能依赖循环输出，因此每次读取都重新计算。
	// 已执行的流程节点会保留当前上下文供下游数据读取。
	if recompute && !pre.Node.BeConnect && !pre.Node.IsEntrance {
		if err := graph.evaluateDataNode(pre.Node); err != nil {
			return err
		}
	}

	preCtx, ok := graph.getContext(pre.Node)
	if !ok {
		return fmt.Errorf("pre node %s not exec", pre.Node.ID)
	}
	if pre.OutPortID < 0 || pre.OutPortID >= len(preCtx.OutputPorts) {
		return fmt.Errorf("pre node %s out port index %d not found", pre.Node.ID, pre.OutPortID)
	}
	if err := assignPortValue(inPort, preCtx.OutputPorts[pre.OutPortID]); err != nil {
		return fmt.Errorf("node %s input port %d: %w", n.ID, index, err)
	}
	_ = ctx
	return nil
}

func (n *ExecNode) applyInputBinding(graph *Graph, ctx *ExecContext, binding InputBinding, recompute bool) error {
	if binding.InputPortID < 0 || binding.InputPortID >= len(ctx.InputPorts) {
		return fmt.Errorf("node %s input port index %d not found", n.ID, binding.InputPortID)
	}
	inPort := ctx.InputPorts[binding.InputPortID]
	if inPort == nil {
		return fmt.Errorf("node %s input port index %d is nil", n.ID, binding.InputPortID)
	}

	switch binding.Kind {
	case InputBindingDefault:
		if binding.DefaultPort != nil {
			return assignPortValue(inPort, binding.DefaultPort)
		}
		return nil
	case InputBindingProducer:
		producer := binding.Producer
		if producer == nil {
			return fmt.Errorf("node %s input port %d producer is nil", n.ID, binding.InputPortID)
		}
		if recompute && binding.RecomputeProducer {
			if err := graph.evaluateDataNode(producer); err != nil {
				return err
			}
		}
		preCtx, ok := graph.getContext(producer)
		if !ok {
			return fmt.Errorf("pre node %s not exec", producer.ID)
		}
		if binding.ProducerOutPortID < 0 || binding.ProducerOutPortID >= len(preCtx.OutputPorts) {
			return fmt.Errorf("pre node %s out port index %d not found", producer.ID, binding.ProducerOutPortID)
		}
		if err := assignPortValue(inPort, preCtx.OutputPorts[binding.ProducerOutPortID]); err != nil {
			return fmt.Errorf("node %s input port %d: %w", n.ID, binding.InputPortID, err)
		}
		return nil
	default:
		return fmt.Errorf("node %s input port %d has unknown binding kind %d", n.ID, binding.InputPortID, binding.Kind)
	}
}

func (g *Graph) evaluateDataNode(target *ExecNode) error {
	if g == nil || target == nil {
		return fmt.Errorf("data producer is nil")
	}
	type dataFrame struct {
		node     *ExecNode
		ctx      *ExecContext
		bindings []InputBinding
		next     int
		waiting  *InputBinding
	}
	newFrame := func(node *ExecNode) (dataFrame, error) {
		if node == nil || node.Definition == nil {
			return dataFrame{}, fmt.Errorf("data producer is invalid")
		}
		if g.execution != nil {
			if err := g.execution.cancellationError(); err != nil {
				return dataFrame{}, err
			}
		}
		if err := g.executionBudget().consume(); err != nil {
			return dataFrame{}, fmt.Errorf("node %s: %w", node.ID, err)
		}
		ctx := g.acquireContext(node)
		if node.Index < 0 {
			g.setContext(node, ctx)
		}
		bindings := node.InputBindings
		if len(bindings) == 0 {
			bindings = make([]InputBinding, 0, len(node.Definition.DataInPortIndexes))
			for _, inputPortID := range node.Definition.DataInPortIndexes {
				if inputPortID >= 0 && inputPortID < len(node.PreInPort) && node.PreInPort[inputPortID] != nil {
					pre := node.PreInPort[inputPortID]
					bindings = append(bindings, InputBinding{Kind: InputBindingProducer, InputPortID: inputPortID, Producer: pre.Node, ProducerOutPortID: pre.OutPortID, RecomputeProducer: pre.Node != nil && !pre.Node.BeConnect && !pre.Node.IsEntrance})
					continue
				}
				if err := node.setInPort(g, ctx, inputPortID, ctx.InputPorts[inputPortID], false); err != nil {
					g.releaseContext(node, ctx)
					return dataFrame{}, err
				}
			}
		}
		return dataFrame{node: node, ctx: ctx, bindings: bindings}, nil
	}
	first, err := newFrame(target)
	if err != nil {
		return err
	}
	active := map[*ExecNode]bool{target: true}
	stack := []dataFrame{first}
	defer func() {
		for index := len(stack) - 1; index >= 0; index-- {
			g.releaseContext(stack[index].node, stack[index].ctx)
		}
	}()
	for len(stack) != 0 {
		frame := &stack[len(stack)-1]
		if frame.waiting != nil {
			if err := frame.node.applyInputBinding(g, frame.ctx, *frame.waiting, false); err != nil {
				return err
			}
			frame.waiting = nil
			frame.next++
			continue
		}
		if frame.next < len(frame.bindings) {
			binding := frame.bindings[frame.next]
			if binding.Kind == InputBindingProducer && binding.RecomputeProducer {
				producer := binding.Producer
				if producer == nil {
					return fmt.Errorf("node %s input port %d producer is nil", frame.node.ID, binding.InputPortID)
				}
				if active[producer] {
					return fmt.Errorf("data dependency cycle at node %s", producer.ID)
				}
				child, err := newFrame(producer)
				if err != nil {
					return err
				}
				frame.waiting = &frame.bindings[frame.next]
				active[producer] = true
				stack = append(stack, child)
				continue
			}
			if err := frame.node.applyInputBinding(g, frame.ctx, binding, false); err != nil {
				return err
			}
			frame.next++
			continue
		}

		exec := frame.node.Definition.New()
		if binder, ok := exec.(interface {
			bind(*Graph, *ExecNode, *ExecContext)
		}); ok {
			binder.bind(g, frame.node, frame.ctx)
		}
		nextIndex, err := exec.Exec()
		g.logLegacyNode(frame.node, frame.ctx, nextIndex, err)
		g.traceNode(frame.node, frame.ctx, nextIndex, err)
		if err != nil {
			return err
		}
		delete(active, frame.node)
		node := frame.node
		g.releaseContext(frame.node, frame.ctx)
		stack = stack[:len(stack)-1]
		if nextIndex != -1 {
			if err := node.doNext(g, nextIndex); err != nil {
				return err
			}
		}
	}
	return nil
}

// CompiledGraph 是编译后的蓝图图结构，可被多个实例共享。
type CompiledGraph struct {
	Entrances map[int64]*ExecNode
	Variables map[string]VariableConfig
	Functions map[string]*CompiledGraph
	NodeCount int
}

// Graph 是单次执行过程中的轻量运行对象。
//
// 它引用共享的 CompiledGraph，并挂接实例变量、timer 和函数调用上下文。
type Graph struct {
	compiled           *CompiledGraph
	contextMu          sync.Mutex
	context            []*ExecContext
	contextGeneration  uint64
	contextFallback    map[*ExecNode]*ExecContext
	name               string
	graphID            int64
	module             IBlueprintModule
	instance           *GraphInstance
	returns            PortArray
	returnPort         IPort
	functionResults    []any
	functionCompleted  atomic.Bool
	onFunctionComplete func([]any) error
	callDepth          int
	variables          map[string]IPort
	variableMu         *sync.RWMutex
	logger             IBlueprintLogger
	trace              *blueprintTraceRuntime
	execution          *Execution
	functionFrame      *functionFrame
	suspendedMu        sync.Mutex
	suspended          *Continuation
	budget             *executionBudget
	stepLimit          uint64
}

// NewGraph 创建一次执行用的运行对象。
//
// 传入的 CompiledGraph 不会被修改。
func NewGraph(compiled *CompiledGraph) *Graph {
	return &Graph{compiled: compiled}
}

// Do 同步执行指定入口，并把蓝图返回值转换为 PortArray。
//
// 遇到异步挂起时返回 ErrExecutionSuspended；需要等待异步结果的调用方应使用 Blueprint.Start 或 DoContext。
func (g *Graph) Do(entranceID int64, args ...any) (PortArray, error) {
	if g != nil && g.execution == nil {
		limit := g.stepLimit
		if limit == 0 {
			limit = defaultExecutionStepLimit
		}
		g.budget = newExecutionBudget(limit)
	}
	returns, err := g.runEntrance(entranceID, args...)
	if errors.Is(err, ErrExecutionSuspended) {
		return nil, ErrExecutionSuspended
	}
	g.releaseContextReferences()
	if errors.Is(err, ErrFunctionReturned) {
		return returns, nil
	}
	return returns, err
}

func (g *Graph) executionBudget() *executionBudget {
	if g == nil {
		return nil
	}
	if g.execution != nil {
		scope := g.execution.ensureScope()
		if scope != nil {
			return scope.budget
		}
	}
	return g.budget
}

func (g *Graph) enterStep() error {
	return g.executionBudget().enter()
}

func (g *Graph) leaveStep() {
	g.executionBudget().leave()
}

// runEntrance 执行入口节点并收集返回值。
func (g *Graph) runEntrance(entranceID int64, args ...any) (PortArray, error) {
	if g == nil || g.compiled == nil {
		return nil, nil
	}
	entrance := g.compiled.Entrances[entranceID]
	if entrance == nil {
		return nil, nil
	}

	g.resetContext()
	clear(g.returns)
	g.returns = g.returns[:0]
	g.returnPort = nil
	clear(g.functionResults)
	g.functionResults = g.functionResults[:0]
	g.functionCompleted.Store(false)
	if g.variableMu == nil {
		g.variableMu = &sync.RWMutex{}
	}
	if g.variables == nil {
		g.variables = g.initialVariables()
	}
	if err := entrance.Do(g, args...); err != nil {
		if errors.Is(err, ErrExecutionSuspended) {
			return nil, err
		}
		return append(PortArray(nil), g.returns...), err
	}
	if len(g.returns) == 0 && g.returnPort != nil {
		if returns, ok := g.returnPort.GetArray(); ok {
			return append(PortArray(nil), returns...), nil
		}
	}
	return append(PortArray(nil), g.returns...), nil
}

func (g *Graph) resultSnapshot() PortArray {
	if g == nil {
		return nil
	}
	if len(g.returns) != 0 {
		return append(PortArray(nil), g.returns...)
	}
	if g.returnPort != nil {
		if returns, ok := g.returnPort.GetArray(); ok {
			return append(PortArray(nil), returns...)
		}
	}
	return nil
}

func (g *Graph) initialVariables() map[string]IPort {
	if g == nil {
		return map[string]IPort{}
	}
	return initialVariables(g.compiled)
}

func initialVariables(compiled *CompiledGraph) map[string]IPort {
	variables := map[string]IPort{}
	if compiled == nil {
		return variables
	}
	for name, config := range compiled.Variables {
		port, err := newPortFromDataType(config.Type)
		if err != nil {
			continue
		}
		if config.Value != nil {
			_ = port.setAnyValue(config.Value)
		}
		variables[name] = port
	}
	return variables
}

func compiledNodeCount(compiled *CompiledGraph) int {
	if compiled == nil || compiled.NodeCount < 0 {
		return 0
	}
	return compiled.NodeCount
}

func (g *Graph) resetContext() {
	g.clearSuspendedContinuation()
	g.contextMu.Lock()
	defer g.contextMu.Unlock()
	g.contextGeneration = (g.contextGeneration + 1) & execContextGenerationMask
	if g.contextGeneration == 0 {
		g.contextGeneration = 1
		clear(g.context)
	}
	nodeCount := compiledNodeCount(g.compiled)
	if cap(g.context) >= nodeCount {
		g.context = g.context[:nodeCount]
	} else {
		previous := g.context
		g.context = make([]*ExecContext, nodeCount)
		copy(g.context, previous)
	}
	g.contextFallback = nil
}

func (g *Graph) acquireContext(node *ExecNode) *ExecContext {
	if node == nil || node.Definition == nil {
		return nil
	}
	if g == nil || node.Index < 0 {
		return node.Definition.cloneContext()
	}
	g.contextMu.Lock()
	defer g.contextMu.Unlock()
	if node.Index >= len(g.context) {
		next := make([]*ExecContext, node.Index+1)
		copy(next, g.context)
		g.context = next
	}
	ctx := g.context[node.Index]
	if ctx == nil || ctx.state&execContextActiveBit != 0 {
		ctx = node.Definition.cloneContext()
	}
	ctx.reset(node.Definition)
	ctx.state = g.contextGeneration | execContextActiveBit
	g.context[node.Index] = ctx
	return ctx
}

func (g *Graph) releaseContext(node *ExecNode, ctx *ExecContext) {
	if g == nil || node == nil || ctx == nil || node.Index < 0 {
		return
	}
	g.contextMu.Lock()
	defer g.contextMu.Unlock()
	if node.Index >= len(g.context) {
		return
	}
	ctx.state &= execContextGenerationMask
}

func (g *Graph) setSuspendedContinuation(continuation *Continuation) {
	if g == nil {
		return
	}
	g.suspendedMu.Lock()
	g.suspended = continuation
	g.suspendedMu.Unlock()
}

func (g *Graph) clearSuspendedContinuation() {
	if g == nil {
		return
	}
	g.suspendedMu.Lock()
	g.suspended = nil
	g.suspendedMu.Unlock()
}

func (g *Graph) currentSuspendedContinuation() *Continuation {
	if g == nil {
		return nil
	}
	g.suspendedMu.Lock()
	defer g.suspendedMu.Unlock()
	return g.suspended
}

func (g *Graph) releaseContextReferences() {
	if g == nil {
		return
	}
	g.contextMu.Lock()
	defer g.contextMu.Unlock()
	for _, ctx := range g.context {
		releaseExecContextReferences(ctx)
	}
	for _, ctx := range g.contextFallback {
		releaseExecContextReferences(ctx)
	}
	clear(g.returns)
	g.returns = g.returns[:0]
	g.returnPort = nil
	clear(g.functionResults)
	g.functionResults = g.functionResults[:0]
}

func releaseExecContextReferences(ctx *ExecContext) {
	if ctx == nil {
		return
	}
	releasePortReferences(ctx.InputPorts)
	releasePortReferences(ctx.OutputPorts)
}

func releasePortReferences(ports []IPort) {
	for index, port := range ports {
		builtin, ok := port.(*Port)
		if !ok {
			ports[index] = nil
			continue
		}
		if builtin == nil {
			continue
		}
		builtin.strv = ""
		builtin.arrv = nil
		builtin.anyv = nil
	}
}

func (g *Graph) setContext(node *ExecNode, ctx *ExecContext) {
	if g == nil || node == nil {
		return
	}
	g.contextMu.Lock()
	defer g.contextMu.Unlock()
	if node.Index >= 0 {
		if node.Index >= len(g.context) {
			next := make([]*ExecContext, node.Index+1)
			copy(next, g.context)
			g.context = next
		}
		ctx.state = (ctx.state & execContextActiveBit) | g.contextGeneration
		g.context[node.Index] = ctx
		return
	}
	if g.contextFallback == nil {
		g.contextFallback = map[*ExecNode]*ExecContext{}
	}
	g.contextFallback[node] = ctx
}

func (g *Graph) getContext(node *ExecNode) (*ExecContext, bool) {
	if g == nil || node == nil {
		return nil, false
	}
	g.contextMu.Lock()
	defer g.contextMu.Unlock()
	if node.Index >= 0 {
		if node.Index >= len(g.context) || g.context[node.Index] == nil || g.context[node.Index].state&execContextGenerationMask != g.contextGeneration {
			return nil, false
		}
		return g.context[node.Index], true
	}
	ctx := g.contextFallback[node]
	return ctx, ctx != nil
}

func clonePorts(source []IPort) []IPort {
	if source == nil {
		return nil
	}
	ports := make([]IPort, len(source))
	var concretePorts []Port
	for index, port := range source {
		if port != nil {
			if concrete, ok := port.(*Port); ok {
				if concretePorts == nil {
					concretePorts = make([]Port, len(source))
				}
				clone := clonePortValue(*concrete)
				concretePorts[index] = clone
				ports[index] = &concretePorts[index]
				continue
			}
			ports[index] = port.Clone()
		}
	}
	return ports
}

func (g *Graph) appendReturn(value ArrayData) {
	g.returns = append(g.returns, value)
}

func (g *Graph) getAndCreateReturnPort() IPort {
	if g == nil {
		return nil
	}
	if g.returnPort != nil {
		return g.returnPort
	}
	g.returnPort = NewPortArray()
	return g.returnPort
}

func (g *Graph) completeFunction(values []any) error {
	g.functionResults = append(g.functionResults[:0], values...)
	g.functionCompleted.Store(true)
	if g.onFunctionComplete != nil {
		return g.onFunctionComplete(values)
	}
	return nil
}

func firstDataOutPort(ports []IPort) int {
	for index, port := range ports {
		if port != nil && !port.IsPortExec() {
			return index
		}
	}
	return len(ports)
}

func dataInPortIndexes(ports []IPort) []int {
	indexes := make([]int, 0, len(ports))
	for index, port := range ports {
		if port != nil && !port.IsPortExec() {
			indexes = append(indexes, index)
		}
	}
	return indexes
}
