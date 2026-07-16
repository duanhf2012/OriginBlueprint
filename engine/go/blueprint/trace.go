package blueprint

import (
	"errors"
	"sync/atomic"
)

// BlueprintTraceLogger 在每个节点执行后接收一条结构化 trace 事件。
//
// trace 默认关闭；只有开启 trace 且配置 logger 后，才会复制端口值。
type BlueprintTraceLogger interface {
	TraceBlueprintNode(BlueprintTraceEvent)
}

type legacyBlueprintLogger interface {
	LogNodeExec(nodeName string, nodeID string, inPorts []IPort, outPorts []IPort, execResult int, err error)
}

// BlueprintTraceEvent 是单个蓝图节点的执行流程日志。
type BlueprintTraceEvent struct {
	Step            uint64
	GraphName       string
	GraphID         int64
	ExecutionID     uint64
	EntranceID      int64
	PC              PC
	Stage           string
	NodeID          string
	NodeName        string
	ExecInputPortID int
	NextIndex       int
	Inputs          []BlueprintTracePortValue
	Outputs         []BlueprintTracePortValue
	Error           string
}

// BlueprintTracePortValue 是 trace 捕获的一个输入或输出端口值。
type BlueprintTracePortValue struct {
	Index  int
	Type   string
	Value  any
	IsExec bool
}

type blueprintTraceState struct {
	nextStep uint64
}

type blueprintTraceRuntime struct {
	logger BlueprintTraceLogger
	state  *blueprintTraceState
}

// SetTraceEnabled 开关蓝图执行流程 trace。
//
// 关闭时执行路径只做轻量 nil 判断，不构造端口日志值。
func (b *Blueprint) SetTraceEnabled(enabled bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.traceEnabled = enabled
}

// SetTraceLogger 设置 trace 开启时使用的 logger。
func (b *Blueprint) SetTraceLogger(logger BlueprintTraceLogger) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.traceLogger = logger
}

func (g *Graph) traceNode(node *ExecNode, ctx *ExecContext, nextIndex int, execErr error) {
	g.traceNodeStage(node, ctx, nextIndex, execErr, "native")
}

func (g *Graph) traceControlNode(node *ExecNode, ctx *ExecContext) {
	g.traceNodeStage(node, ctx, -1, nil, "control")
}

func (g *Graph) traceNodeStage(node *ExecNode, ctx *ExecContext, nextIndex int, execErr error, stage string) {
	if g == nil || g.trace == nil || g.trace.logger == nil || node == nil || ctx == nil || node.Definition == nil {
		return
	}
	state := g.trace.state
	if state == nil {
		state = &blueprintTraceState{}
		g.trace.state = state
	}
	event := BlueprintTraceEvent{
		Step:            atomic.AddUint64(&state.nextStep, 1),
		GraphName:       g.name,
		GraphID:         g.graphID,
		PC:              InvalidPC,
		Stage:           stage,
		NodeID:          node.ID,
		NodeName:        node.Definition.Name,
		ExecInputPortID: ctx.ExecInputPortID,
		NextIndex:       nextIndex,
		Inputs:          tracePortValues(ctx.InputPorts),
		Outputs:         tracePortValues(ctx.OutputPorts),
	}
	if g.execution != nil {
		event.ExecutionID = g.execution.id
		event.EntranceID = g.execution.entranceID
	}
	if g.vm != nil {
		event.PC = g.vm.pc
	}
	if execErr != nil && !isTraceControlError(execErr) {
		event.Error = execErr.Error()
	}
	g.trace.logger.TraceBlueprintNode(event)
}

func (g *Graph) logLegacyNode(node *ExecNode, ctx *ExecContext, nextIndex int, execErr error) {
	if g == nil || g.logger == nil || node == nil || ctx == nil || node.Definition == nil {
		return
	}
	logger, ok := g.logger.(legacyBlueprintLogger)
	if !ok {
		return
	}
	logger.LogNodeExec(node.Definition.Name, node.ID, ctx.InputPorts, ctx.OutputPorts, nextIndex, execErr)
}

func isTraceControlError(err error) bool {
	return errors.Is(err, ErrExecutionSuspended)
}

func tracePortValues(ports []IPort) []BlueprintTracePortValue {
	if len(ports) == 0 {
		return nil
	}
	values := make([]BlueprintTracePortValue, 0, len(ports))
	for index, port := range ports {
		if port == nil {
			continue
		}
		values = append(values, tracePortValue(index, port))
	}
	return values
}

func tracePortValue(index int, port IPort) BlueprintTracePortValue {
	if concrete, ok := port.(*Port); ok && concrete != nil {
		return traceConcretePortValue(index, concrete)
	}
	if port.IsPortExec() {
		return BlueprintTracePortValue{Index: index, Type: "执行", IsExec: true}
	}
	return BlueprintTracePortValue{Index: index, Type: "任意", Value: port.GetAny()}
}

func traceConcretePortValue(index int, port *Port) BlueprintTracePortValue {
	switch port.kind {
	case portKindExec:
		return BlueprintTracePortValue{Index: index, Type: "执行", IsExec: true}
	case portKindInt:
		return BlueprintTracePortValue{Index: index, Type: "整数", Value: port.intv}
	case portKindFloat:
		return BlueprintTracePortValue{Index: index, Type: "浮点", Value: port.floatv}
	case portKindString:
		return BlueprintTracePortValue{Index: index, Type: "字符串", Value: port.strv}
	case portKindBool:
		return BlueprintTracePortValue{Index: index, Type: "布尔", Value: port.boolv}
	case portKindArray:
		return BlueprintTracePortValue{Index: index, Type: "数组", Value: append(PortArray(nil), port.arrv...)}
	case portKindAny:
		return BlueprintTracePortValue{Index: index, Type: "任意", Value: cloneAnyValue(port.anyv)}
	default:
		return BlueprintTracePortValue{Index: index, Type: "未知"}
	}
}
