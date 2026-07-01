package golang

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ErrExecutionSuspended is returned by async nodes after they capture a continuation.
var ErrExecutionSuspended = errors.New("golang blueprint execution suspended")

// ErrFunctionReturned stops a child function graph after FunctionReturn resumes
// the caller continuation.
var ErrFunctionReturned = errors.New("golang blueprint function returned")

// ErrLoopBreak is raised by a loop break exec input and consumed by the owning loop.
var ErrLoopBreak = errors.New("golang blueprint loop break")

// IExecNode is the executable contract implemented by all runtime nodes.
type IExecNode interface {
	GetName() string
	Exec() (int, error)
}

// ExecContext holds cloned ports for a single node execution.
type ExecContext struct {
	InputPorts      []IPort
	OutputPorts     []IPort
	ExecInputPortID int
}

// BaseExecNode gives concrete nodes typed port helpers and continuation access.
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

// NodeDefinition describes a reusable node class.
//
// Definitions are shared by compiled graphs; ports are cloned into ExecContext
// before each node execution.
type NodeDefinition struct {
	Name                   string
	New                    func() IExecNode
	InPorts                []IPort
	OutPorts               []IPort
	OutPortParamStartIndex int
	DataInPortIndexes      []int
}

// NewNodeDefinition builds a node definition and records where output arguments start.
func NewNodeDefinition(name string, newExec func() IExecNode, inPorts []IPort, outPorts []IPort) *NodeDefinition {
	return &NodeDefinition{
		Name:                   name,
		New:                    newExec,
		InPorts:                clonePorts(inPorts),
		OutPorts:               clonePorts(outPorts),
		OutPortParamStartIndex: firstDataOutPort(outPorts),
		DataInPortIndexes:      dataInPortIndexes(inPorts),
	}
}

func (d *NodeDefinition) cloneContext() *ExecContext {
	return &ExecContext{
		InputPorts:  clonePorts(d.InPorts),
		OutputPorts: clonePorts(d.OutPorts),
	}
}

// PrePortNode records the producer node for a data input.
type PrePortNode struct {
	Node      *ExecNode
	OutPortID int
}

// ExecNode is a compiled node in the shared execution tree.
//
// It does not store per-run port values; those live in Graph.context.
type ExecNode struct {
	ID              string
	Index           int
	Definition      *NodeDefinition
	Next            []*ExecNode
	NextInPort      []int
	PreInPort       []*PrePortNode
	DefaultIn       map[int]any
	DefaultInputs   []IPort
	DefaultInputSet []bool
	VariableName    string
	FunctionID      string
	FunctionName    string
	FunctionGraph   *CompiledGraph
	BeConnect       bool
	IsEntrance      bool
}

// NewExecNode creates a compiled node instance from a definition.
func NewExecNode(id string, definition *NodeDefinition) *ExecNode {
	return &ExecNode{
		ID:         id,
		Index:      -1,
		Definition: definition,
		PreInPort:  make([]*PrePortNode, len(definition.InPorts)),
		DefaultIn:  map[int]any{},
	}
}

// Do executes this node in one Graph session.
//
// Data dependencies are evaluated lazily: pure producer nodes run only when an
// executed node asks for their output.
func (n *ExecNode) Do(graph *Graph, outPortArgs ...any) error {
	return n.doWithInput(graph, 0, outPortArgs...)
}

func (n *ExecNode) doWithInput(graph *Graph, execInputPortID int, outPortArgs ...any) error {
	if n == nil || n.Definition == nil {
		return fmt.Errorf("exec node is invalid")
	}
	ctx := n.Definition.cloneContext()
	ctx.ExecInputPortID = execInputPortID
	graph.setContext(n, ctx)

	if err := n.applyOutputArgs(ctx, outPortArgs...); err != nil {
		return err
	}
	for _, index := range n.Definition.DataInPortIndexes {
		inPort := ctx.InputPorts[index]
		if err := n.setInPort(graph, ctx, index, inPort); err != nil {
			return err
		}
	}

	exec := n.Definition.New()
	if binder, ok := exec.(interface {
		bind(*Graph, *ExecNode, *ExecContext)
	}); ok {
		binder.bind(graph, n, ctx)
	}

	nextIndex, err := exec.Exec()
	if err != nil {
		return err
	}
	return n.doNext(graph, nextIndex)
}

func (n *ExecNode) doNext(graph *Graph, index int) error {
	if index == -1 {
		return nil
	}
	if index < 0 {
		return fmt.Errorf("next index %d not found", index)
	}
	if index >= len(n.Next) {
		return nil
	}
	if n.Next[index] == nil {
		return nil
	}
	nextInPort := 0
	if index < len(n.NextInPort) {
		nextInPort = n.NextInPort[index]
	}
	return n.Next[index].doWithInput(graph, nextInPort)
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

func (n *ExecNode) setInPort(graph *Graph, ctx *ExecContext, index int, inPort IPort) error {
	pre := n.PreInPort[index]
	if pre == nil {
		if index < len(n.DefaultInputSet) && n.DefaultInputSet[index] {
			inPort.SetValue(n.DefaultInputs[index])
			return nil
		}
		if value, ok := n.DefaultIn[index]; ok {
			return inPort.setAnyValue(value)
		}
		return nil
	}

	// Pure data nodes may depend on loop outputs, so compute them on every
	// demand. Executed flow nodes keep their current context for downstream data.
	if !pre.Node.BeConnect && !pre.Node.IsEntrance {
		if err := pre.Node.Do(graph); err != nil {
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
	inPort.SetValue(preCtx.OutputPorts[pre.OutPortID])
	_ = ctx
	return nil
}

// CompiledGraph is the immutable runtime form shared by many graph instances.
type CompiledGraph struct {
	Entrances map[int64]*ExecNode
	Variables map[string]VariableConfig
	Functions map[string]*CompiledGraph
	NodeCount int
}

// Graph is one execution session.
//
// A Graph is intentionally short-lived for Blueprint.Do calls. Continuations
// keep it alive only when async nodes suspend.
type Graph struct {
	compiled           *CompiledGraph
	context            []*ExecContext
	contextFallback    map[*ExecNode]*ExecContext
	name               string
	graphID            int64
	module             IBlueprintModule
	instance           *GraphInstance
	returns            PortArray
	functionResults    []any
	onFunctionComplete func([]any) error
	callDepth          int
	variables          map[string]IPort
	variableMu         *sync.RWMutex
	timers             map[uint64]*time.Timer
	timerSeq           uint64
}

// NewGraph creates a standalone execution session for a compiled graph.
//
// Blueprint.Do normally creates sessions for you; tests and embedded callers can
// use NewGraph directly.
func NewGraph(compiled *CompiledGraph) *Graph {
	return &Graph{compiled: compiled, context: make([]*ExecContext, compiledNodeCount(compiled)), timers: map[uint64]*time.Timer{}}
}

// Do executes one entrance and hides internal suspension/function-return sentinels.
func (g *Graph) Do(entranceID int64, args ...any) (PortArray, error) {
	returns, err := g.runEntrance(entranceID, args...)
	if errors.Is(err, ErrExecutionSuspended) {
		return nil, nil
	}
	if errors.Is(err, ErrFunctionReturned) {
		return returns, nil
	}
	return returns, err
}

// runEntrance executes one entrance and returns sentinel errors to internal callers.
func (g *Graph) runEntrance(entranceID int64, args ...any) (PortArray, error) {
	if g == nil || g.compiled == nil {
		return nil, nil
	}
	entrance := g.compiled.Entrances[entranceID]
	if entrance == nil {
		return nil, nil
	}

	g.resetContext()
	g.returns = nil
	g.functionResults = nil
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
	return append(PortArray(nil), g.returns...), nil
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
	g.context = make([]*ExecContext, compiledNodeCount(g.compiled))
	g.contextFallback = nil
}

func (g *Graph) setContext(node *ExecNode, ctx *ExecContext) {
	if g == nil || node == nil {
		return
	}
	if node.Index >= 0 {
		if node.Index >= len(g.context) {
			next := make([]*ExecContext, node.Index+1)
			copy(next, g.context)
			g.context = next
		}
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
	if node.Index >= 0 {
		if node.Index >= len(g.context) || g.context[node.Index] == nil {
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
	for index, port := range source {
		if port != nil {
			ports[index] = port.Clone()
		}
	}
	return ports
}

func (g *Graph) appendReturn(value ArrayData) {
	g.returns = append(g.returns, value)
}

func (g *Graph) completeFunction(values []any) error {
	g.functionResults = append(g.functionResults[:0], values...)
	if g.onFunctionComplete != nil {
		return g.onFunctionComplete(values)
	}
	return nil
}

func (g *Graph) addTimer(timer *time.Timer) uint64 {
	if g.timers == nil {
		g.timers = map[uint64]*time.Timer{}
	}
	id := atomic.AddUint64(&g.timerSeq, 1)
	g.timers[id] = timer
	return id
}

func (g *Graph) cancelTimer(id uint64) bool {
	timer := g.timers[id]
	if timer == nil {
		return false
	}
	delete(g.timers, id)
	return timer.Stop()
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
