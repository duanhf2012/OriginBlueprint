package golang

import (
	"errors"
	"sync/atomic"
)

// BlueprintTraceLogger receives one event after each node execution.
//
// Trace is opt-in. The engine does not format or copy port values unless trace
// is enabled on Blueprint and a logger is configured.
type BlueprintTraceLogger interface {
	TraceBlueprintNode(BlueprintTraceEvent)
}

// BlueprintTraceEvent is a structured execution log for one blueprint node.
type BlueprintTraceEvent struct {
	Step            uint64
	GraphName       string
	GraphID         int64
	NodeID          string
	NodeName        string
	ExecInputPortID int
	NextIndex       int
	Inputs          []BlueprintTracePortValue
	Outputs         []BlueprintTracePortValue
	Error           string
}

// BlueprintTracePortValue is one input or output value captured for trace logs.
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

// SetTraceEnabled toggles blueprint execution tracing.
//
// Tracing is disabled by default. When disabled, the execution path only checks
// a boolean and does not build trace port values.
func (b *Blueprint) SetTraceEnabled(enabled bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.traceEnabled = enabled
}

// SetTraceLogger sets the logger used when trace is enabled.
func (b *Blueprint) SetTraceLogger(logger BlueprintTraceLogger) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.traceLogger = logger
}

func (g *Graph) traceNode(node *ExecNode, ctx *ExecContext, nextIndex int, execErr error) {
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
		NodeID:          node.ID,
		NodeName:        node.Definition.Name,
		ExecInputPortID: ctx.ExecInputPortID,
		NextIndex:       nextIndex,
		Inputs:          tracePortValues(ctx.InputPorts),
		Outputs:         tracePortValues(ctx.OutputPorts),
	}
	if execErr != nil && !isTraceControlError(execErr) {
		event.Error = execErr.Error()
	}
	g.trace.logger.TraceBlueprintNode(event)
}

func isTraceControlError(err error) bool {
	return errors.Is(err, ErrExecutionSuspended) || errors.Is(err, ErrFunctionReturned) || errors.Is(err, ErrLoopBreak)
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
		return BlueprintTracePortValue{Index: index, Type: "exec", IsExec: true}
	}
	return BlueprintTracePortValue{Index: index, Type: "any", Value: port.GetAny()}
}

func traceConcretePortValue(index int, port *Port) BlueprintTracePortValue {
	switch port.kind {
	case portKindExec:
		return BlueprintTracePortValue{Index: index, Type: "exec", IsExec: true}
	case portKindInt:
		return BlueprintTracePortValue{Index: index, Type: "integer", Value: port.intv}
	case portKindFloat:
		return BlueprintTracePortValue{Index: index, Type: "float", Value: port.floatv}
	case portKindString:
		return BlueprintTracePortValue{Index: index, Type: "string", Value: port.strv}
	case portKindBool:
		return BlueprintTracePortValue{Index: index, Type: "boolean", Value: port.boolv}
	case portKindArray:
		return BlueprintTracePortValue{Index: index, Type: "array", Value: append(PortArray(nil), port.arrv...)}
	case portKindAny:
		return BlueprintTracePortValue{Index: index, Type: "any", Value: cloneAnyValue(port.anyv)}
	default:
		return BlueprintTracePortValue{Index: index, Type: "unknown"}
	}
}
