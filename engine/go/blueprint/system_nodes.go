package blueprint

import (
	"fmt"
	"math"
	"math/rand"
)

const (
	// EntranceIDIntParam 是整数参数入口的固定 ID。
	EntranceIDIntParam int64 = 1
	// EntranceIDArrayParam 是数组参数入口的固定 ID。
	EntranceIDArrayParam int64 = 2
	returnVariable             = "g_Return"
)

// BuiltinExecNodeFactories 返回内置系统节点工厂。
func BuiltinExecNodeFactories() []func() IExecNode {
	return []func() IExecNode{
		NewExecNodeFactory[EntranceIntParam, *EntranceIntParam](), NewExecNodeFactory[EntranceArrayParam, *EntranceArrayParam](),
		NewExecNodeFactory[DebugOutput, *DebugOutput](), NewExecNodeFactory[Sequence, *Sequence](), NewExecNodeFactory[Foreach, *Foreach](), NewExecNodeFactory[ForeachIntArray, *ForeachIntArray](),
		NewExecNodeFactory[BoolIf, *BoolIf](), NewExecNodeFactory[GreaterThanInteger, *GreaterThanInteger](), NewExecNodeFactory[LessThanInteger, *LessThanInteger](), NewExecNodeFactory[EqualInteger, *EqualInteger](),
		NewExecNodeFactory[RangeCompare, *RangeCompare](), NewExecNodeFactory[EqualSwitch, *EqualSwitch](), NewExecNodeFactory[Probability, *Probability](),
		NewExecNodeFactory[AddInt, *AddInt](), NewExecNodeFactory[SubInt, *SubInt](), NewExecNodeFactory[MulInt, *MulInt](), NewExecNodeFactory[DivInt, *DivInt](),
		NewExecNodeFactory[ModInt, *ModInt](), NewExecNodeFactory[RandNumber, *RandNumber](),
		NewExecNodeFactory[GetArrayInt, *GetArrayInt](), NewExecNodeFactory[GetArrayString, *GetArrayString](), NewExecNodeFactory[GetArrayLen, *GetArrayLen](),
		NewExecNodeFactory[CreateIntArray, *CreateIntArray](), NewExecNodeFactory[CreateStringArray, *CreateStringArray](), NewExecNodeFactory[AppendIntegerToArray, *AppendIntegerToArray](), NewExecNodeFactory[AppendStringToArray, *AppendStringToArray](),
		NewExecNodeFactory[IntInArray, *IntInArray](),
		NewExecNodeFactory[AppendIntReturn, *AppendIntReturn](), NewExecNodeFactory[AppendStringReturn, *AppendStringReturn](),
		NewExecNodeFactory[SleepNode, *SleepNode](),
		NewExecNodeFactory[SetTimerByFunctionNode, *SetTimerByFunctionNode](), NewExecNodeFactory[ClearTimerNode, *ClearTimerNode](), NewExecNodeFactory[PauseTimerNode, *PauseTimerNode](), NewExecNodeFactory[UnpauseTimerNode, *UnpauseTimerNode](),
		NewExecNodeFactory[IsTimerActiveNode, *IsTimerActiveNode](), NewExecNodeFactory[IsTimerPausedNode, *IsTimerPausedNode](), NewExecNodeFactory[IsTimerValidNode, *IsTimerValidNode](),
		NewExecNodeFactory[GetTimerRemainingNode, *GetTimerRemainingNode](), NewExecNodeFactory[GetTimerElapsedNode, *GetTimerElapsedNode](),
		NewExecNodeFactory[LiteralString, *LiteralString](), NewExecNodeFactory[CastIntegerString, *CastIntegerString](), NewExecNodeFactory[CastFloatString, *CastFloatString](), NewExecNodeFactory[CastAnyString, *CastAnyString](),
		NewExecNodeFactory[AddFloat, *AddFloat](), NewExecNodeFactory[SubFloat, *SubFloat](), NewExecNodeFactory[MulFloat, *MulFloat](), NewExecNodeFactory[DivFloat, *DivFloat](), NewExecNodeFactory[CompareGreaterInteger, *CompareGreaterInteger](),
		NewExecNodeFactory[StringSplit, *StringSplit](), NewExecNodeFactory[GetArrayAny, *GetArrayAny](),
		NewExecNodeFactory[WhileNode, *WhileNode](), NewExecNodeFactory[ForLoopBreak, *ForLoopBreak](), NewExecNodeFactory[ForeachArray, *ForeachArray](),
	}
}

type execNodePtr[T any] interface {
	*T
	IExecNode
}

// NewExecNodeFactory 将具体节点类型包装为统一工厂函数。
func NewExecNodeFactory[T any, P execNodePtr[T]]() func() IExecNode {
	return func() IExecNode {
		var node T
		return P(&node)
	}
}

// EntranceIntParam 是带整数参数的蓝图入口节点。
type EntranceIntParam struct{ BaseExecNode }

// EntranceArrayParam 是带数组参数的蓝图入口节点。
type EntranceArrayParam struct{ BaseExecNode }

func (n *EntranceIntParam) GetName() string   { return "Entrance_IntParam" }
func (n *EntranceArrayParam) GetName() string { return "Entrance_ArrayParam" }
func (n *EntranceIntParam) Exec() (int, error) {
	return 0, nil
}
func (n *EntranceArrayParam) Exec() (int, error) {
	return 0, nil
}

// DebugOutput 是调试输出节点。
type DebugOutput struct{ BaseExecNode }

func (n *DebugOutput) GetName() string { return "DebugOutput" }
func (n *DebugOutput) Exec() (int, error) {
	return 0, nil
}

// Sequence 是顺序执行多个输出分支的节点。
type Sequence struct{ BaseExecNode }

func (n *Sequence) GetName() string { return "Sequence" }
func (n *Sequence) Exec() (int, error) {
	for index, port := range n.ctx.OutputPorts {
		if port == nil || !port.IsPortExec() {
			break
		}
		if err := n.DoNext(index); err != nil {
			return -1, err
		}
	}
	return -1, nil
}

type Foreach struct{ BaseExecNode }

func (n *Foreach) GetName() string { return "Foreach" }
func (n *Foreach) Exec() (int, error) {
	start, ok := n.GetInPortInt(1)
	if !ok {
		return -1, fmt.Errorf("Foreach start input not found")
	}
	end, ok := n.GetInPortInt(2)
	if !ok {
		return -1, fmt.Errorf("Foreach end input not found")
	}
	for index := start; index < end; index++ {
		n.SetOutPortInt(2, index)
		if err := n.DoNext(0); err != nil {
			return -1, err
		}
	}
	if err := n.DoNext(1); err != nil {
		return -1, err
	}
	return -1, nil
}

type ForeachIntArray struct{ BaseExecNode }

func (n *ForeachIntArray) GetName() string { return "ForeachIntArray" }
func (n *ForeachIntArray) Exec() (int, error) {
	array, ok := n.GetInPortArray(1)
	if !ok {
		return -1, fmt.Errorf("ForeachIntArray array input not found")
	}
	for index, item := range array {
		n.SetOutPortInt(2, PortInt(index))
		n.SetOutPortInt(3, item.IntVal)
		if err := n.DoNext(0); err != nil {
			return -1, err
		}
	}
	if err := n.DoNext(1); err != nil {
		return -1, err
	}
	return -1, nil
}

type BoolIf struct{ BaseExecNode }

func (n *BoolIf) GetName() string { return "BoolIf" }
func (n *BoolIf) Exec() (int, error) {
	value, ok := n.GetInPortBool(1)
	if !ok {
		return -1, fmt.Errorf("BoolIf input not found")
	}
	if value {
		return 1, nil
	}
	return 0, nil
}

type GreaterThanInteger struct{ BaseExecNode }
type LessThanInteger struct{ BaseExecNode }
type EqualInteger struct{ BaseExecNode }

func (n *GreaterThanInteger) GetName() string { return "GreaterThanInteger" }
func (n *LessThanInteger) GetName() string    { return "LessThanInteger" }
func (n *EqualInteger) GetName() string       { return "EqualInteger" }

func (n *GreaterThanInteger) Exec() (int, error) {
	includeEqual, ok := n.GetInPortBool(1)
	if !ok {
		return -1, fmt.Errorf("GreaterThanInteger equal input not found")
	}
	a, aok := n.GetInPortInt(2)
	b, bok := n.GetInPortInt(3)
	if !aok || !bok {
		return -1, fmt.Errorf("GreaterThanInteger inputs not found")
	}
	if (includeEqual && a >= b) || (!includeEqual && a > b) {
		return 1, nil
	}
	return 0, nil
}

func (n *LessThanInteger) Exec() (int, error) {
	includeEqual, ok := n.GetInPortBool(1)
	if !ok {
		return -1, fmt.Errorf("LessThanInteger equal input not found")
	}
	a, aok := n.GetInPortInt(2)
	b, bok := n.GetInPortInt(3)
	if !aok || !bok {
		return -1, fmt.Errorf("LessThanInteger inputs not found")
	}
	if (includeEqual && a <= b) || (!includeEqual && a < b) {
		return 1, nil
	}
	return 0, nil
}

func (n *EqualInteger) Exec() (int, error) {
	a, aok := n.GetInPortInt(1)
	b, bok := n.GetInPortInt(2)
	if !aok || !bok {
		return -1, fmt.Errorf("EqualInteger inputs not found")
	}
	if a == b {
		return 1, nil
	}
	return 0, nil
}

type RangeCompare struct{ BaseExecNode }
type EqualSwitch struct{ BaseExecNode }

func (n *RangeCompare) GetName() string { return "RangeCompare" }
func (n *EqualSwitch) GetName() string  { return "EqualSwitch" }

func (n *RangeCompare) Exec() (int, error) {
	value, ok := n.GetInPortInt(1)
	if !ok {
		return -1, fmt.Errorf("RangeCompare value input not found")
	}
	array, _ := n.GetInPortArray(2)
	for index, item := range array {
		if index >= n.GetOutPortCount()-2 {
			break
		}
		if value <= item.IntVal {
			return index + 2, nil
		}
	}
	return 0, nil
}

func (n *EqualSwitch) Exec() (int, error) {
	value, ok := n.GetInPortInt(1)
	if !ok {
		return -1, fmt.Errorf("EqualSwitch value input not found")
	}
	array, _ := n.GetInPortArray(2)
	for index, item := range array {
		if index >= n.GetOutPortCount()-2 {
			break
		}
		if value == item.IntVal {
			return index + 2, nil
		}
	}
	return 0, nil
}

type Probability struct{ BaseExecNode }

func (n *Probability) GetName() string { return "Probability" }
func (n *Probability) Exec() (int, error) {
	value, ok := n.GetInPortInt(1)
	if !ok {
		return -1, fmt.Errorf("Probability input not found")
	}
	if value > PortInt(rand.Int63n(10000)) {
		return 1, nil
	}
	return 0, nil
}

type AddInt struct{ BaseExecNode }
type SubInt struct{ BaseExecNode }
type MulInt struct{ BaseExecNode }
type DivInt struct{ BaseExecNode }
type ModInt struct{ BaseExecNode }
type RandNumber struct{ BaseExecNode }

func (n *AddInt) GetName() string     { return "AddInt" }
func (n *SubInt) GetName() string     { return "SubInt" }
func (n *MulInt) GetName() string     { return "MulInt" }
func (n *DivInt) GetName() string     { return "DivInt" }
func (n *ModInt) GetName() string     { return "ModInt" }
func (n *RandNumber) GetName() string { return "RandNumber" }

func (n *AddInt) Exec() (int, error) {
	a, aok := n.GetInPortInt(0)
	b, bok := n.GetInPortInt(1)
	if !aok || !bok {
		return -1, fmt.Errorf("AddInt inputs not found")
	}
	n.SetOutPortInt(0, a+b)
	return -1, nil
}

func (n *SubInt) Exec() (int, error) {
	a, aok := n.GetInPortInt(0)
	b, bok := n.GetInPortInt(1)
	abs, _ := n.GetInPortBool(2)
	if !aok || !bok {
		return -1, fmt.Errorf("SubInt inputs not found")
	}
	value := a - b
	if abs && value < 0 {
		value = -value
	}
	n.SetOutPortInt(0, value)
	return -1, nil
}

func (n *MulInt) Exec() (int, error) {
	a, aok := n.GetInPortInt(0)
	b, bok := n.GetInPortInt(1)
	if !aok || !bok {
		return -1, fmt.Errorf("MulInt inputs not found")
	}
	n.SetOutPortInt(0, a*b)
	return -1, nil
}

func (n *DivInt) Exec() (int, error) {
	a, aok := n.GetInPortInt(0)
	b, bok := n.GetInPortInt(1)
	round, _ := n.GetInPortBool(2)
	if !aok || !bok {
		return -1, fmt.Errorf("DivInt inputs not found")
	}
	if b == 0 {
		return -1, fmt.Errorf("div zero error")
	}
	if round {
		n.SetOutPortInt(0, PortInt(math.Round(float64(a)/float64(b))))
	} else {
		n.SetOutPortInt(0, a/b)
	}
	return -1, nil
}

func (n *ModInt) Exec() (int, error) {
	a, aok := n.GetInPortInt(0)
	b, bok := n.GetInPortInt(1)
	if !aok || !bok {
		return -1, fmt.Errorf("ModInt inputs not found")
	}
	if b == 0 {
		return -1, fmt.Errorf("mod zero error")
	}
	n.SetOutPortInt(0, a%b)
	return -1, nil
}

func (n *RandNumber) Exec() (int, error) {
	seed, _ := n.GetInPortInt(0)
	minv, minOK := n.GetInPortInt(1)
	maxv, maxOK := n.GetInPortInt(2)
	if !minOK || !maxOK {
		return -1, fmt.Errorf("RandNumber inputs not found")
	}
	if maxv < minv {
		return -1, fmt.Errorf("RandNumber invalid range")
	}
	var value PortInt
	if seed > 0 {
		r := rand.New(rand.NewSource(int64(seed)))
		value = PortInt(r.Int63n(int64(maxv-minv+1))) + minv
	} else {
		value = PortInt(rand.Int63n(int64(maxv-minv+1))) + minv
	}
	n.SetOutPortInt(0, value)
	return -1, nil
}

type GetArrayInt struct{ BaseExecNode }
type GetArrayString struct{ BaseExecNode }
type GetArrayLen struct{ BaseExecNode }
type CreateIntArray struct{ BaseExecNode }
type CreateStringArray struct{ BaseExecNode }
type AppendIntegerToArray struct{ BaseExecNode }
type AppendStringToArray struct{ BaseExecNode }

func (n *GetArrayInt) GetName() string          { return "GetArrayInt" }
func (n *GetArrayString) GetName() string       { return "GetArrayString" }
func (n *GetArrayLen) GetName() string          { return "GetArrayLen" }
func (n *CreateIntArray) GetName() string       { return "CreateIntArray" }
func (n *CreateStringArray) GetName() string    { return "CreateStringArray" }
func (n *AppendIntegerToArray) GetName() string { return "AppendIntegerToArray" }
func (n *AppendStringToArray) GetName() string  { return "AppendStringToArray" }

func (n *GetArrayInt) Exec() (int, error) {
	arrayPort := n.GetInPort(0)
	index, ok := n.GetInPortInt(1)
	if arrayPort == nil || !ok {
		return -1, fmt.Errorf("GetArrayInt inputs not found")
	}
	value, ok := arrayPort.GetArrayValInt(int(index))
	if !ok {
		return -1, fmt.Errorf("GetArrayInt index out of range")
	}
	n.SetOutPortInt(0, value)
	return -1, nil
}

func (n *GetArrayString) Exec() (int, error) {
	arrayPort := n.GetInPort(0)
	index, ok := n.GetInPortInt(1)
	if arrayPort == nil || !ok {
		return -1, fmt.Errorf("GetArrayString inputs not found")
	}
	value, ok := arrayPort.GetArrayValStr(int(index))
	if !ok {
		return -1, fmt.Errorf("GetArrayString index out of range")
	}
	n.SetOutPortStr(0, value)
	return -1, nil
}

func (n *GetArrayLen) Exec() (int, error) {
	port := n.GetInPort(0)
	if port == nil {
		return -1, fmt.Errorf("GetArrayLen input not found")
	}
	n.SetOutPortInt(0, port.GetArrayLen())
	return -1, nil
}

func (n *CreateIntArray) Exec() (int, error) {
	array, ok := n.GetInPortArray(0)
	if !ok {
		array = nil
	}
	out := n.GetOutPort(0)
	if out == nil {
		return -1, fmt.Errorf("CreateIntArray output not found")
	}
	for _, item := range array {
		out.AppendArrayValInt(item.IntVal)
	}
	return -1, nil
}

func (n *CreateStringArray) Exec() (int, error) {
	array, ok := n.GetInPortArray(0)
	if !ok {
		array = nil
	}
	out := n.GetOutPort(0)
	if out == nil {
		return -1, fmt.Errorf("CreateStringArray output not found")
	}
	for _, item := range array {
		out.AppendArrayValStr(item.StrVal)
	}
	return -1, nil
}

func (n *AppendIntegerToArray) Exec() (int, error) {
	array, ok := n.GetInPortArray(0)
	value, vok := n.GetInPortInt(1)
	if !ok || !vok {
		return -1, fmt.Errorf("AppendIntegerToArray inputs not found")
	}
	out := n.GetOutPort(0)
	if out == nil {
		return -1, fmt.Errorf("AppendIntegerToArray output not found")
	}
	for _, item := range array {
		out.AppendArrayValInt(item.IntVal)
	}
	out.AppendArrayValInt(value)
	return -1, nil
}

func (n *AppendStringToArray) Exec() (int, error) {
	array, ok := n.GetInPortArray(0)
	value, vok := n.GetInPortStr(1)
	if !ok || !vok {
		return -1, fmt.Errorf("AppendStringToArray inputs not found")
	}
	out := n.GetOutPort(0)
	if out == nil {
		return -1, fmt.Errorf("AppendStringToArray output not found")
	}
	for _, item := range array {
		out.AppendArrayValStr(item.StrVal)
	}
	out.AppendArrayValStr(value)
	return -1, nil
}

type AppendIntReturn struct{ BaseExecNode }
type AppendStringReturn struct{ BaseExecNode }
type IntInArray struct{ BaseExecNode }

func (n *AppendIntReturn) GetName() string    { return "AppendIntReturn" }
func (n *AppendStringReturn) GetName() string { return "AppendStringReturn" }
func (n *IntInArray) GetName() string         { return "IntInArray" }
func (n *AppendIntReturn) Exec() (int, error) {
	value, ok := n.GetInPortInt(1)
	if !ok {
		return -1, fmt.Errorf("AppendIntReturn input not found")
	}
	n.graph.appendReturn(ArrayData{IntVal: value})
	return 0, nil
}
func (n *AppendStringReturn) Exec() (int, error) {
	value, ok := n.GetInPortStr(1)
	if !ok {
		return -1, fmt.Errorf("AppendStringReturn input not found")
	}
	n.graph.appendReturn(ArrayData{StrVal: value})
	return 0, nil
}
func (n *IntInArray) Exec() (int, error) {
	value, ok := n.GetInPortInt(1)
	if !ok {
		return -1, fmt.Errorf("IntInArray inParam 1 not found")
	}
	array, ok := n.GetInPortArray(2)
	if !ok {
		return -1, fmt.Errorf("IntInArray inParam 2 not found")
	}
	for _, item := range array {
		if item.IntVal == value {
			n.SetOutPortBool(1, true)
			return 0, nil
		}
	}
	n.SetOutPortBool(1, false)
	return 0, nil
}
