package blueprint

import "fmt"

type vmFlowKind uint8

const (
	vmFlowTarget vmFlowKind = iota
	vmFlowLoopContinue
)

type vmFlowFrame struct {
	kind   vmFlowKind
	target VMTarget
	loopID uint64
}

type vmLoopKind uint8

const (
	vmLoopRange vmLoopKind = iota
	vmLoopIntArray
	vmLoopAnyArray
	vmLoopWhile
	vmLoopBreakable
)

type vmLoopFrame struct {
	id         uint64
	pc         PC
	node       *ExecNode
	ctx        *ExecContext
	kind       vmLoopKind
	index      int64
	end        int64
	array      PortArray
	iterations uint64
	flowBase   int
	err        error
}

const vmMaximumLoopIterations uint64 = 100_000

func (m *vmMachine) runSequence(nodeIndex int) error {
	plan, ctx, err := m.prepareControlNode(nodeIndex)
	if err != nil {
		return err
	}
	m.graph.releaseContext(plan.Node, ctx)
	targets := make([]VMTarget, 0, len(plan.Successors))
	for portIndex, port := range plan.Node.Definition.OutPorts {
		if port == nil || !port.IsPortExec() {
			break
		}
		if portIndex < len(plan.Successors) {
			targets = append(targets, plan.Successors[portIndex]...)
		}
	}
	m.scheduleTargets(targets)
	return nil
}

func (m *vmMachine) runLoop(op OpCode, nodeIndex int) error {
	plan, err := m.nodePlan(nodeIndex)
	if err != nil {
		return err
	}
	if op == OpBreakableLoop && m.inputPortID == 3 {
		return m.breakLoop(m.pc)
	}
	plan, ctx, err := m.prepareControlNode(nodeIndex)
	if err != nil {
		return err
	}
	m.loopSeq++
	frame := vmLoopFrame{id: m.loopSeq, pc: m.pc, node: plan.Node, ctx: ctx}
	switch op {
	case OpRangeLoop:
		frame.kind = vmLoopRange
		frame.index, _ = portIntAt(ctx.InputPorts, 1)
		frame.end, _ = portIntAt(ctx.InputPorts, 2)
	case OpArrayLoop:
		array, _ := portArrayAt(ctx.InputPorts, 1)
		frame.array = append(PortArray(nil), array...)
		if plan.Node.Definition.Name == "ForeachArray" {
			frame.kind = vmLoopAnyArray
		} else {
			frame.kind = vmLoopIntArray
		}
	case OpWhileLoop:
		frame.kind = vmLoopWhile
	case OpBreakableLoop:
		frame.kind = vmLoopBreakable
		frame.index, _ = portIntAt(ctx.InputPorts, 1)
		frame.end, _ = portIntAt(ctx.InputPorts, 2)
	default:
		m.graph.releaseContext(plan.Node, ctx)
		return fmt.Errorf("blueprint VM invalid loop opcode %d at pc %d", op, m.pc)
	}
	m.loopStack = append(m.loopStack, frame)
	return m.continueLoop(frame.id)
}

func (m *vmMachine) prepareControlNode(nodeIndex int) (*NodePlan, *ExecContext, error) {
	plan, err := m.nodePlan(nodeIndex)
	if err != nil {
		return nil, nil, err
	}
	node := plan.Node
	if err := m.graph.enterStep(); err != nil {
		return nil, nil, fmt.Errorf("node %s: %w", node.ID, err)
	}
	defer m.graph.leaveStep()
	ctx := m.graph.acquireContext(node)
	ctx.ExecInputPortID = m.inputPortID
	if err := node.applyOutputArgs(ctx, m.outputArgs...); err != nil {
		m.graph.releaseContext(node, ctx)
		return nil, nil, err
	}
	for _, binding := range node.InputBindings {
		if err := node.applyInputBinding(m.graph, ctx, binding, true); err != nil {
			m.graph.releaseContext(node, ctx)
			return nil, nil, err
		}
	}
	return plan, ctx, nil
}

func (m *vmMachine) continueLoop(loopID uint64) error {
	loopIndex := m.loopIndex(loopID)
	if loopIndex < 0 {
		return fmt.Errorf("blueprint VM loop frame %d not found", loopID)
	}
	frame := &m.loopStack[loopIndex]
	if frame.err != nil {
		return frame.err
	}
	if frame.iterations >= vmMaximumLoopIterations {
		return fmt.Errorf("node %s exceeded max iterations", frame.node.ID)
	}

	var hasNext bool
	switch frame.kind {
	case vmLoopRange, vmLoopBreakable:
		hasNext = frame.index < frame.end
		if hasNext {
			outputIndex := 2
			if frame.kind == vmLoopBreakable {
				outputIndex = 1
			}
			if !setPortIntAt(frame.ctx.OutputPorts, outputIndex, frame.index) {
				return fmt.Errorf("node %s loop index output not found", frame.node.ID)
			}
			frame.index++
		}
	case vmLoopIntArray, vmLoopAnyArray:
		hasNext = frame.index < int64(len(frame.array))
		if hasNext {
			item := frame.array[frame.index]
			if frame.kind == vmLoopIntArray {
				if !setPortIntAt(frame.ctx.OutputPorts, 2, frame.index) || !setPortIntAt(frame.ctx.OutputPorts, 3, item.IntVal) {
					return fmt.Errorf("node %s array loop outputs not found", frame.node.ID)
				}
			} else {
				if len(frame.ctx.OutputPorts) <= 3 || frame.ctx.OutputPorts[2] == nil || frame.ctx.OutputPorts[2].setAnyValue(item) != nil || !setPortIntAt(frame.ctx.OutputPorts, 3, frame.index) {
					return fmt.Errorf("node %s array loop outputs not found", frame.node.ID)
				}
			}
			frame.index++
		}
	case vmLoopWhile:
		if frame.iterations != 0 {
			if err := frame.node.refreshInput(m.graph, frame.ctx, 1); err != nil {
				return fmt.Errorf("node %s refresh condition: %w", frame.node.ID, err)
			}
		}
		condition, _ := portBoolAt(frame.ctx.InputPorts, 1)
		hasNext = condition
	default:
		return fmt.Errorf("node %s has invalid loop kind %d", frame.node.ID, frame.kind)
	}

	if !hasNext {
		return m.completeLoop(frame.id)
	}
	frame.iterations++
	frame.flowBase = len(m.flowStack)
	m.flowStack = append(m.flowStack, vmFlowFrame{kind: vmFlowLoopContinue, loopID: frame.id})
	plan := &m.program.Nodes[frame.pc]
	if len(plan.Successors) == 0 || len(plan.Successors[0]) == 0 {
		m.resumeFlow()
		return frame.err
	}
	m.scheduleTargets(plan.Successors[0])
	return nil
}

func (m *vmMachine) completeLoop(loopID uint64) error {
	loopIndex := m.loopIndex(loopID)
	if loopIndex < 0 {
		return fmt.Errorf("blueprint VM loop frame %d not found", loopID)
	}
	frame := m.loopStack[loopIndex]
	m.loopStack = append(m.loopStack[:loopIndex], m.loopStack[loopIndex+1:]...)
	m.graph.releaseContext(frame.node, frame.ctx)
	completedPort := 1
	if frame.kind == vmLoopBreakable {
		completedPort = 2
	}
	m.pc = frame.pc
	m.advanceFromPort(completedPort)
	return nil
}

func (m *vmMachine) breakLoop(pc PC) error {
	loopIndex := -1
	for index := len(m.loopStack) - 1; index >= 0; index-- {
		if m.loopStack[index].pc == pc && m.loopStack[index].kind == vmLoopBreakable {
			loopIndex = index
			break
		}
	}
	if loopIndex < 0 {
		return fmt.Errorf("break reached loop pc %d without active loop", pc)
	}
	frame := m.loopStack[loopIndex]
	loopID := frame.id
	if frame.flowBase < len(m.flowStack) {
		m.flowStack = m.flowStack[:frame.flowBase]
	}
	for index := len(m.loopStack) - 1; index > loopIndex; index-- {
		nested := m.loopStack[index]
		m.graph.releaseContext(nested.node, nested.ctx)
	}
	m.loopStack = m.loopStack[:loopIndex+1]
	return m.completeLoop(loopID)
}

func (m *vmMachine) loopIndex(loopID uint64) int {
	for index := len(m.loopStack) - 1; index >= 0; index-- {
		if m.loopStack[index].id == loopID {
			return index
		}
	}
	return -1
}

func portIntAt(ports []IPort, index int) (PortInt, bool) {
	if index < 0 || index >= len(ports) || ports[index] == nil {
		return 0, false
	}
	return ports[index].GetInt()
}

func portBoolAt(ports []IPort, index int) (PortBool, bool) {
	if index < 0 || index >= len(ports) || ports[index] == nil {
		return false, false
	}
	return ports[index].GetBool()
}

func portArrayAt(ports []IPort, index int) (PortArray, bool) {
	if index < 0 || index >= len(ports) || ports[index] == nil {
		return nil, false
	}
	return ports[index].GetArray()
}

func setPortIntAt(ports []IPort, index int, value PortInt) bool {
	return index >= 0 && index < len(ports) && ports[index] != nil && ports[index].SetInt(value)
}
