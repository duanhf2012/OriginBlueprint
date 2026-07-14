package blueprint

// PC 是 VM Program 中的指令位置。
type PC int32

const InvalidPC PC = -1

// OpCode 描述 VM 的控制指令。
type OpCode uint8

const (
	OpInvalid OpCode = iota
	OpCallNative
	OpSequence
	OpRangeLoop
	OpArrayLoop
	OpWhileLoop
	OpBreakableLoop
	OpCallFunction
	OpReturnFunction
	OpYield
	OpHalt
)

// ControlKind 是节点定义在编译期携带的控制语义。
type ControlKind uint8

const (
	ControlNative ControlKind = iota
	ControlSequence
	ControlRangeLoop
	ControlArrayLoop
	ControlWhileLoop
	ControlBreakableLoop
	ControlFunctionCall
	ControlFunctionReturn
)

// Instruction 是紧凑的 VM 指令。复杂参数保存在 Program side table 中。
type Instruction struct {
	Op      OpCode
	A, B, C int32
}

func opcodeForControl(kind ControlKind) OpCode {
	switch kind {
	case ControlSequence:
		return OpSequence
	case ControlRangeLoop:
		return OpRangeLoop
	case ControlArrayLoop:
		return OpArrayLoop
	case ControlWhileLoop:
		return OpWhileLoop
	case ControlBreakableLoop:
		return OpBreakableLoop
	case ControlFunctionCall:
		return OpCallFunction
	case ControlFunctionReturn:
		return OpReturnFunction
	default:
		return OpCallNative
	}
}

func builtinControlKind(name string) ControlKind {
	switch name {
	case "Sequence":
		return ControlSequence
	case "Foreach":
		return ControlRangeLoop
	case "ForeachIntArray", "ForeachArray":
		return ControlArrayLoop
	case "WhileNode":
		return ControlWhileLoop
	case "ForLoopBreak":
		return ControlBreakableLoop
	case "FunctionCall":
		return ControlFunctionCall
	case "FunctionReturn":
		return ControlFunctionReturn
	default:
		return ControlNative
	}
}
