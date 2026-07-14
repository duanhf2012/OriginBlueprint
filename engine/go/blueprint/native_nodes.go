package blueprint

import (
	"fmt"
	"strconv"
	"strings"
)

type LiteralString struct{ BaseExecNode }
type CastIntegerString struct{ BaseExecNode }
type CastFloatString struct{ BaseExecNode }
type CastAnyString struct{ BaseExecNode }

func (n *LiteralString) GetName() string     { return "LiteralString" }
func (n *CastIntegerString) GetName() string { return "CastIntegerString" }
func (n *CastFloatString) GetName() string   { return "CastFloatString" }
func (n *CastAnyString) GetName() string     { return "CastAnyString" }
func (n *LiteralString) Exec() (int, error) {
	value, _ := n.GetInPortStr(0)
	n.SetOutPortStr(0, value)
	return -1, nil
}
func (n *CastIntegerString) Exec() (int, error) {
	value, _ := n.GetInPortInt(0)
	n.SetOutPortStr(0, PortString(strconv.FormatInt(int64(value), 10)))
	return -1, nil
}
func (n *CastFloatString) Exec() (int, error) {
	value, _ := n.GetInPortFloat(0)
	n.SetOutPortStr(0, PortString(strconv.FormatFloat(float64(value), 'f', -1, 64)))
	return -1, nil
}
func (n *CastAnyString) Exec() (int, error) {
	value := portAnyValue(n.GetInPort(1))
	n.SetOutPortBool(1, value != nil)
	n.SetOutPortStr(2, PortString(fmt.Sprint(value)))
	return 0, nil
}

type AddFloat struct{ BaseExecNode }
type SubFloat struct{ BaseExecNode }
type MulFloat struct{ BaseExecNode }
type DivFloat struct{ BaseExecNode }
type CompareGreaterInteger struct{ BaseExecNode }

func (n *AddFloat) GetName() string              { return "AddFloat" }
func (n *SubFloat) GetName() string              { return "SubFloat" }
func (n *MulFloat) GetName() string              { return "MulFloat" }
func (n *DivFloat) GetName() string              { return "DivFloat" }
func (n *CompareGreaterInteger) GetName() string { return "CompareGreaterInteger" }
func (n *AddFloat) Exec() (int, error) {
	return execFloatBinary(&n.BaseExecNode, func(a, b PortFloat) PortFloat { return a + b })
}
func (n *SubFloat) Exec() (int, error) {
	return execFloatBinary(&n.BaseExecNode, func(a, b PortFloat) PortFloat { return a - b })
}
func (n *MulFloat) Exec() (int, error) {
	return execFloatBinary(&n.BaseExecNode, func(a, b PortFloat) PortFloat { return a * b })
}
func (n *DivFloat) Exec() (int, error) {
	b, ok := n.GetInPortFloat(1)
	if !ok || b == 0 {
		return -1, fmt.Errorf("DivFloat invalid divisor")
	}
	a, _ := n.GetInPortFloat(0)
	n.SetOutPortFloat(0, a/b)
	return -1, nil
}
func (n *CompareGreaterInteger) Exec() (int, error) {
	a, aok := n.GetInPortInt(0)
	b, bok := n.GetInPortInt(1)
	if !aok || !bok {
		return -1, fmt.Errorf("CompareGreaterInteger inputs not found")
	}
	n.SetOutPortBool(0, a > b)
	n.SetOutPortInt(1, a)
	n.SetOutPortInt(2, b)
	return -1, nil
}

func execFloatBinary(n *BaseExecNode, op func(PortFloat, PortFloat) PortFloat) (int, error) {
	a, aok := n.GetInPortFloat(0)
	b, bok := n.GetInPortFloat(1)
	if !aok || !bok {
		return -1, fmt.Errorf("float inputs not found")
	}
	n.SetOutPortFloat(0, op(a, b))
	return -1, nil
}

type StringSplit struct{ BaseExecNode }
type GetArrayAny struct{ BaseExecNode }
type WhileNode struct{ BaseExecNode }
type ForLoopBreak struct{ BaseExecNode }
type ForeachArray struct{ BaseExecNode }

func (n *StringSplit) GetName() string  { return "StringSplit" }
func (n *GetArrayAny) GetName() string  { return "GetArrayAny" }
func (n *WhileNode) GetName() string    { return "WhileNode" }
func (n *ForLoopBreak) GetName() string { return "ForLoopBreak" }
func (n *ForeachArray) GetName() string { return "ForeachArray" }
func (n *StringSplit) Exec() (int, error) {
	text, _ := n.GetInPortStr(1)
	delimiter, _ := n.GetInPortStr(2)
	parts := strings.Split(string(text), string(delimiter))
	array := make(PortArray, 0, len(parts))
	for _, part := range parts {
		array = append(array, ArrayData{StrVal: PortString(part)})
	}
	n.GetOutPort(1).setAnyValue(array)
	return 0, nil
}
func (n *GetArrayAny) Exec() (int, error) {
	array, ok := n.GetInPortArray(0)
	index, iok := n.GetInPortInt(1)
	if !ok || !iok || int(index) < 0 || int(index) >= len(array) {
		return -1, fmt.Errorf("GetArrayAny inputs invalid")
	}
	n.GetOutPort(0).setAnyValue(array[int(index)])
	return -1, nil
}
func (n *WhileNode) Exec() (int, error) {
	return -1, ErrControlNodeRequiresVM
}
func (n *ForLoopBreak) Exec() (int, error) {
	return -1, ErrControlNodeRequiresVM
}
func (n *ForeachArray) Exec() (int, error) {
	return -1, ErrControlNodeRequiresVM
}
