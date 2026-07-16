package blueprint

import (
	"fmt"
	"strconv"
	"strings"
)

const maxFunctionFlowStates = 100_000

type functionFlowStep struct {
	pc        PC
	inputPort int
}

type functionFlowState struct {
	steps []functionFlowStep
}

// validateFunctionReturnPaths 对函数的可终止执行路径做保守检查。
// 分析不确定或状态数量超限时放行，运行时无返回保护仍是最后防线。
func validateFunctionReturnPaths(compiled *CompiledGraph) error {
	if compiled == nil || !compiled.hasFunctionEntry || compiled.Program == nil {
		return nil
	}
	entryPC, ok := compiled.Program.Entrances[FunctionEntranceID]
	if !ok {
		return nil
	}
	program := compiled.Program
	work := []functionFlowState{{steps: []functionFlowStep{{pc: entryPC}}}}
	visited := make(map[string]struct{})
	returnReachable := false
	uncertain := false
	var firstFallthrough *ExecNode

	push := func(steps []functionFlowStep, terminal *ExecNode) {
		if len(steps) == 0 {
			if firstFallthrough == nil {
				firstFallthrough = terminal
			}
			return
		}
		if len(steps) > len(program.Nodes)*2+8 {
			uncertain = true
			return
		}
		work = append(work, functionFlowState{steps: steps})
	}

	for len(work) != 0 {
		last := len(work) - 1
		state := work[last]
		work = work[:last]
		key := functionFlowStateKey(state.steps)
		if _, exists := visited[key]; exists {
			continue
		}
		visited[key] = struct{}{}
		if len(visited) > maxFunctionFlowStates {
			uncertain = true
			break
		}

		current := state.steps[0]
		rest := state.steps[1:]
		if current.pc < 0 || int(current.pc) >= len(program.Nodes) {
			uncertain = true
			continue
		}
		plan := &program.Nodes[current.pc]
		node := plan.Node
		if node == nil || node.Definition == nil {
			uncertain = true
			continue
		}
		if plan.Control == ControlFunctionReturn {
			returnReachable = true
			continue
		}

		switch plan.Control {
		case ControlSequence:
			push(functionFlowSchedule(plan.SequenceTargets, rest), node)
		case ControlRangeLoop, ControlArrayLoop, ControlWhileLoop, ControlBreakableLoop:
			if analyzeFunctionLoop(plan, current, rest, push) {
				uncertain = true
			}
		case ControlFunctionCall:
			if node.FunctionGraph == nil {
				uncertain = true
				continue
			}
			push(functionFlowSchedule(functionFlowSuccessors(plan, 0), rest), node)
		default:
			outputs, certain := functionFlowOutputs(plan)
			if !certain {
				uncertain = true
				continue
			}
			for _, output := range outputs {
				push(functionFlowSchedule(functionFlowSuccessors(plan, output), rest), node)
			}
		}
	}

	if uncertain {
		return nil
	}
	entryNode := program.Nodes[entryPC].Node
	if !returnReachable {
		return newBlueprintNodeError(BlueprintStageCompile, nil, entryNode, InvalidPC, fmt.Errorf("unreachable FunctionReturn from FunctionEntry"))
	}
	if firstFallthrough != nil {
		return newBlueprintNodeError(BlueprintStageCompile, nil, firstFallthrough, InvalidPC, fmt.Errorf("function execution can fallthrough without FunctionReturn"))
	}
	return nil
}

func analyzeFunctionLoop(plan *NodePlan, current functionFlowStep, rest []functionFlowStep, push func([]functionFlowStep, *ExecNode)) bool {
	if plan == nil || plan.Node == nil {
		return true
	}
	completedOutput := 1
	if plan.Control == ControlBreakableLoop {
		completedOutput = 2
	}
	breakOnly := plan.Control == ControlBreakableLoop && current.inputPort == 3
	if !breakOnly && plan.Control != ControlWhileLoop {
		return true
	}
	bodyPossible := !breakOnly
	completedPossible := true
	if plan.Control == ControlWhileLoop {
		value, known := staticBoolInput(plan.Node, 1)
		if !known {
			return true
		}
		bodyPossible = value
		completedPossible = !value
	}
	if completedPossible {
		push(functionFlowSchedule(functionFlowSuccessors(plan, completedOutput), rest), plan.Node)
	}
	if bodyPossible {
		loopAgain := functionFlowStep{pc: current.pc, inputPort: current.inputPort}
		next := append([]functionFlowStep{loopAgain}, rest...)
		push(functionFlowSchedule(functionFlowSuccessors(plan, 0), next), plan.Node)
	}
	return false
}

func functionFlowOutputs(plan *NodePlan) ([]int, bool) {
	if plan == nil || plan.Node == nil {
		return []int{0}, false
	}
	if plan.Node.Definition.Name == "BoolIf" {
		if value, known := staticBoolInput(plan.Node, 1); known {
			if value {
				return []int{1}, true
			}
			return []int{0}, true
		}
		return []int{0, 1}, true
	}
	outputs := make([]int, 0, len(plan.ExecOutputs))
	for index, isExec := range plan.ExecOutputs {
		if isExec {
			outputs = append(outputs, index)
		}
	}
	if len(outputs) == 0 {
		return []int{0}, true
	}
	if len(outputs) > 1 {
		return nil, false
	}
	return outputs, true
}

func staticBoolInput(node *ExecNode, index int) (PortBool, bool) {
	if node == nil || node.Definition == nil || index < 0 || index >= len(node.Definition.InPorts) {
		return false, false
	}
	if index < len(node.PreInPort) && node.PreInPort[index] != nil {
		return false, false
	}
	if index < len(node.DefaultInputSet) && node.DefaultInputSet[index] && index < len(node.DefaultInputs) && node.DefaultInputs[index] != nil {
		return node.DefaultInputs[index].GetBool()
	}
	if node.Definition.InPorts[index] == nil {
		return false, false
	}
	return node.Definition.InPorts[index].GetBool()
}

func functionFlowSuccessors(plan *NodePlan, output int) []VMTarget {
	if plan == nil || output < 0 || output >= len(plan.Successors) {
		return nil
	}
	return plan.Successors[output]
}

func functionFlowSchedule(targets []VMTarget, rest []functionFlowStep) []functionFlowStep {
	steps := make([]functionFlowStep, 0, len(targets)+len(rest))
	for _, target := range targets {
		steps = append(steps, functionFlowStep{pc: target.PC, inputPort: target.InputPortID})
	}
	steps = append(steps, rest...)
	return steps
}

func functionFlowStateKey(steps []functionFlowStep) string {
	var builder strings.Builder
	for _, step := range steps {
		builder.WriteString(strconv.FormatInt(int64(step.pc), 10))
		builder.WriteByte(':')
		builder.WriteString(strconv.Itoa(step.inputPort))
		builder.WriteByte(';')
	}
	return builder.String()
}
