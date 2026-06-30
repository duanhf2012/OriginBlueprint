package golang

import (
	"errors"
	"fmt"
)

var ErrExecutionSuspended = errors.New("golang blueprint execution suspended")

type IExecNode interface {
	GetName() string
	Exec() (int, error)
}

type ExecContext struct {
	InputPorts  []IPort
	OutputPorts []IPort
}

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

func (n *BaseExecNode) DoNext(index int) error {
	if n == nil || n.node == nil || n.graph == nil {
		return fmt.Errorf("node is not executing")
	}
	return n.node.doNext(n.graph, index)
}

type NodeDefinition struct {
	Name                   string
	New                    func() IExecNode
	InPorts                []IPort
	OutPorts               []IPort
	OutPortParamStartIndex int
}

func NewNodeDefinition(name string, newExec func() IExecNode, inPorts []IPort, outPorts []IPort) *NodeDefinition {
	return &NodeDefinition{
		Name:                   name,
		New:                    newExec,
		InPorts:                clonePorts(inPorts),
		OutPorts:               clonePorts(outPorts),
		OutPortParamStartIndex: firstDataOutPort(outPorts),
	}
}

func (d *NodeDefinition) cloneContext() *ExecContext {
	return &ExecContext{
		InputPorts:  clonePorts(d.InPorts),
		OutputPorts: clonePorts(d.OutPorts),
	}
}

type PrePortNode struct {
	Node      *ExecNode
	OutPortID int
}

type ExecNode struct {
	ID         string
	Definition *NodeDefinition
	Next       []*ExecNode
	PreInPort  []*PrePortNode
	DefaultIn  map[int]any
	BeConnect  bool
	IsEntrance bool
}

func NewExecNode(id string, definition *NodeDefinition) *ExecNode {
	return &ExecNode{
		ID:         id,
		Definition: definition,
		PreInPort:  make([]*PrePortNode, len(definition.InPorts)),
		DefaultIn:  map[int]any{},
	}
}

func (n *ExecNode) Do(graph *Graph, outPortArgs ...any) error {
	if n == nil || n.Definition == nil {
		return fmt.Errorf("exec node is invalid")
	}
	ctx := n.Definition.cloneContext()
	graph.context[n.ID] = ctx

	if err := n.applyOutputArgs(ctx, outPortArgs...); err != nil {
		return err
	}
	for index, inPort := range ctx.InputPorts {
		if inPort == nil || inPort.IsPortExec() {
			continue
		}
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
	if index < 0 || index >= len(n.Next) {
		return fmt.Errorf("next index %d not found", index)
	}
	if n.Next[index] == nil {
		return nil
	}
	return n.Next[index].Do(graph)
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
		if value, ok := n.DefaultIn[index]; ok {
			return inPort.setAnyValue(value)
		}
		return nil
	}

	if _, ok := graph.context[pre.Node.ID]; !ok && !pre.Node.BeConnect && !pre.Node.IsEntrance {
		if err := pre.Node.Do(graph); err != nil {
			return err
		}
	}

	preCtx, ok := graph.context[pre.Node.ID]
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

type CompiledGraph struct {
	Entrances map[int64]*ExecNode
}

type Graph struct {
	compiled *CompiledGraph
	context  map[string]*ExecContext
	name     string
}

func NewGraph(compiled *CompiledGraph) *Graph {
	return &Graph{compiled: compiled, context: map[string]*ExecContext{}}
}

func (g *Graph) Do(entranceID int64, args ...any) (PortArray, error) {
	if g == nil || g.compiled == nil {
		return nil, nil
	}
	entrance := g.compiled.Entrances[entranceID]
	if entrance == nil {
		return nil, nil
	}

	g.context = map[string]*ExecContext{}
	if err := entrance.Do(g, args...); err != nil {
		if errors.Is(err, ErrExecutionSuspended) {
			return nil, nil
		}
		return nil, err
	}
	return nil, nil
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

func firstDataOutPort(ports []IPort) int {
	for index, port := range ports {
		if port != nil && !port.IsPortExec() {
			return index
		}
	}
	return len(ports)
}
