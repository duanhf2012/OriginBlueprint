package blueprint

import "fmt"

// PortInt 是蓝图整数端口值。
type PortInt int64

// PortFloat 是蓝图浮点端口值。
type PortFloat float64

// PortString 是蓝图字符串端口值。
type PortString string

// PortBool 是蓝图布尔端口值。
type PortBool bool

// PortArray 是蓝图数组端口值。
type PortArray []ArrayData

// PortAny 是蓝图任意类型端口值。
type PortAny = any

// ArrayData 是数组端口中的单个元素。
//
// DataType 指示当前元素应该读取哪个具体值字段。
type ArrayData struct {
	IntVal   PortInt
	FloatVal PortFloat
	StrVal   PortString
	BoolVal  PortBool
}

type portKind uint8

const (
	portKindExec portKind = iota
	portKindInt
	portKindFloat
	portKindString
	portKindBool
	portKindArray
	portKindAny
)

// IPort 是蓝图端口的统一访问接口。
//
// 执行期通过该接口读写不同基础类型，避免频繁反射。
type IPort interface {
	Clone() IPort
	IsPortExec() bool
	SetValue(IPort)
	GetInt() (PortInt, bool)
	GetFloat() (PortFloat, bool)
	GetStr() (PortString, bool)
	GetBool() (PortBool, bool)
	GetArray() (PortArray, bool)
	GetArrayLen() PortInt
	GetArrayValInt(int) (PortInt, bool)
	GetArrayValStr(int) (PortString, bool)
	SetInt(PortInt) bool
	SetFloat(PortFloat) bool
	SetStr(PortString) bool
	SetBool(PortBool) bool
	AppendArrayValInt(PortInt) bool
	AppendArrayValStr(PortString) bool
	GetAny() any
	SetAny(any) bool
	setAnyValue(any) error
}

// Port 是 IPort 的默认实现。
type Port struct {
	kind   portKind
	intv   PortInt
	floatv PortFloat
	strv   PortString
	boolv  PortBool
	arrv   PortArray
	anyv   any
}

// NewPortExec 创建执行流端口。
func NewPortExec() IPort {
	return &Port{kind: portKindExec}
}

// NewPortInt 创建整数端口。
func NewPortInt() IPort {
	return &Port{kind: portKindInt}
}

// NewPortArray 创建数组端口。
func NewPortArray() IPort {
	return &Port{kind: portKindArray}
}

// NewPortFloat 创建浮点端口。
func NewPortFloat() IPort {
	return &Port{kind: portKindFloat}
}

// NewPortStr 创建字符串端口。
func NewPortStr() IPort {
	return &Port{kind: portKindString}
}

// NewPortBool 创建布尔端口。
func NewPortBool() IPort {
	return &Port{kind: portKindBool}
}

// NewPortAny 创建任意类型端口。
func NewPortAny() IPort {
	return &Port{kind: portKindAny}
}

func (p *Port) Clone() IPort {
	if p == nil {
		return nil
	}
	clone := clonePortValue(*p)
	return &clone
}

func clonePortValue(source Port) Port {
	clone := source
	if source.arrv != nil {
		clone.arrv = append(PortArray(nil), source.arrv...)
	}
	clone.anyv = cloneAnyValue(source.anyv)
	return clone
}

func (p *Port) IsPortExec() bool {
	return p != nil && p.kind == portKindExec
}

func (p *Port) SetValue(source IPort) {
	sourcePort, ok := source.(*Port)
	if !ok || p == nil || sourcePort == nil {
		return
	}
	p.kind = sourcePort.kind
	p.intv = sourcePort.intv
	p.floatv = sourcePort.floatv
	p.strv = sourcePort.strv
	p.boolv = sourcePort.boolv
	p.arrv = append(p.arrv[:0], sourcePort.arrv...)
	p.anyv = cloneAnyValue(sourcePort.anyv)
}

func (p *Port) GetInt() (PortInt, bool) {
	if p == nil || p.kind != portKindInt {
		return 0, false
	}
	return p.intv, true
}

func (p *Port) GetFloat() (PortFloat, bool) {
	if p == nil || p.kind != portKindFloat {
		return 0, false
	}
	return p.floatv, true
}

func (p *Port) GetStr() (PortString, bool) {
	if p == nil || p.kind != portKindString {
		return "", false
	}
	return p.strv, true
}

func (p *Port) GetBool() (PortBool, bool) {
	if p == nil || p.kind != portKindBool {
		return false, false
	}
	return p.boolv, true
}

func (p *Port) GetArray() (PortArray, bool) {
	if p == nil || p.kind != portKindArray {
		return nil, false
	}
	return p.arrv, true
}

func (p *Port) GetArrayLen() PortInt {
	if p == nil || p.kind != portKindArray {
		return 0
	}
	return PortInt(len(p.arrv))
}

func (p *Port) GetArrayValInt(index int) (PortInt, bool) {
	if p == nil || p.kind != portKindArray || index < 0 || index >= len(p.arrv) {
		return 0, false
	}
	return p.arrv[index].IntVal, true
}

func (p *Port) GetArrayValStr(index int) (PortString, bool) {
	if p == nil || p.kind != portKindArray || index < 0 || index >= len(p.arrv) {
		return "", false
	}
	return p.arrv[index].StrVal, true
}

func (p *Port) SetInt(value PortInt) bool {
	if p == nil || p.kind != portKindInt {
		return false
	}
	p.intv = value
	return true
}

func (p *Port) SetFloat(value PortFloat) bool {
	if p == nil || p.kind != portKindFloat {
		return false
	}
	p.floatv = value
	return true
}

func (p *Port) SetStr(value PortString) bool {
	if p == nil || p.kind != portKindString {
		return false
	}
	p.strv = value
	return true
}

func (p *Port) SetBool(value PortBool) bool {
	if p == nil || p.kind != portKindBool {
		return false
	}
	p.boolv = value
	return true
}

func (p *Port) AppendArrayValInt(value PortInt) bool {
	if p == nil || p.kind != portKindArray {
		return false
	}
	p.arrv = append(p.arrv, ArrayData{IntVal: value})
	return true
}

func (p *Port) AppendArrayValStr(value PortString) bool {
	if p == nil || p.kind != portKindArray {
		return false
	}
	p.arrv = append(p.arrv, ArrayData{StrVal: value})
	return true
}

func (p *Port) GetAny() any {
	if p == nil {
		return nil
	}
	if p.kind == portKindAny {
		return cloneAnyValue(p.anyv)
	}
	return portAnyValue(p)
}

func (p *Port) SetAny(value any) bool {
	if p == nil || p.kind != portKindAny {
		return false
	}
	p.anyv = cloneAnyValue(value)
	return true
}

func (p *Port) setAnyValue(value any) error {
	if p == nil {
		return fmt.Errorf("port is nil")
	}
	switch p.kind {
	case portKindInt:
		intv, ok := asPortInt(value)
		if !ok {
			return fmt.Errorf("port expects int, got %T", value)
		}
		p.intv = intv
		return nil
	case portKindFloat:
		floatv, ok := asPortFloat(value)
		if !ok {
			return fmt.Errorf("port expects float, got %T", value)
		}
		p.floatv = floatv
		return nil
	case portKindString:
		strv, ok := asPortString(value)
		if !ok {
			return fmt.Errorf("port expects string, got %T", value)
		}
		p.strv = strv
		return nil
	case portKindBool:
		boolv, ok := asPortBool(value)
		if !ok {
			return fmt.Errorf("port expects bool, got %T", value)
		}
		p.boolv = boolv
		return nil
	case portKindArray:
		arrayv, ok := asPortArray(value)
		if !ok {
			return fmt.Errorf("port expects array, got %T", value)
		}
		p.arrv = append(p.arrv[:0], arrayv...)
		return nil
	case portKindAny:
		p.anyv = cloneAnyValue(value)
		return nil
	case portKindExec:
		return fmt.Errorf("can not assign data to exec port")
	default:
		return fmt.Errorf("unknown port kind %d", p.kind)
	}
}

func cloneAnyValue(value any) any {
	switch v := value.(type) {
	case PortArray:
		return append(PortArray(nil), v...)
	case []ArrayData:
		return append(PortArray(nil), v...)
	case []string:
		return append([]string(nil), v...)
	case []any:
		return append([]any(nil), v...)
	case map[string]any:
		clone := make(map[string]any, len(v))
		for key, item := range v {
			clone[key] = cloneAnyValue(item)
		}
		return clone
	default:
		return value
	}
}

func asPortInt(value any) (PortInt, bool) {
	switch v := value.(type) {
	case PortInt:
		return v, true
	case int:
		return PortInt(v), true
	case int8:
		return PortInt(v), true
	case int16:
		return PortInt(v), true
	case int32:
		return PortInt(v), true
	case int64:
		return PortInt(v), true
	case float64:
		return PortInt(v), true
	case float32:
		return PortInt(v), true
	case uint:
		return PortInt(v), true
	case uint8:
		return PortInt(v), true
	case uint16:
		return PortInt(v), true
	case uint32:
		return PortInt(v), true
	case uint64:
		return PortInt(v), true
	default:
		return 0, false
	}
}

func asPortFloat(value any) (PortFloat, bool) {
	switch v := value.(type) {
	case PortFloat:
		return v, true
	case float64:
		return PortFloat(v), true
	case float32:
		return PortFloat(v), true
	case int:
		return PortFloat(v), true
	case int64:
		return PortFloat(v), true
	case PortInt:
		return PortFloat(v), true
	default:
		return 0, false
	}
}

func asPortString(value any) (PortString, bool) {
	switch v := value.(type) {
	case PortString:
		return v, true
	case string:
		return PortString(v), true
	default:
		return "", false
	}
}

func asPortBool(value any) (PortBool, bool) {
	switch v := value.(type) {
	case PortBool:
		return v, true
	case bool:
		return PortBool(v), true
	case int:
		return PortBool(v != 0), true
	case int64:
		return PortBool(v != 0), true
	case PortInt:
		return PortBool(v != 0), true
	default:
		return false, false
	}
}

func asPortArray(value any) (PortArray, bool) {
	switch v := value.(type) {
	case PortArray:
		return append(PortArray(nil), v...), true
	case []ArrayData:
		return append(PortArray(nil), v...), true
	case []int:
		array := make(PortArray, 0, len(v))
		for _, item := range v {
			array = append(array, ArrayData{IntVal: PortInt(item)})
		}
		return array, true
	case []int64:
		array := make(PortArray, 0, len(v))
		for _, item := range v {
			array = append(array, ArrayData{IntVal: PortInt(item)})
		}
		return array, true
	case []string:
		array := make(PortArray, 0, len(v))
		for _, item := range v {
			array = append(array, ArrayData{StrVal: PortString(item)})
		}
		return array, true
	case []any:
		array := make(PortArray, 0, len(v))
		for _, item := range v {
			if intv, ok := asPortInt(item); ok {
				array = append(array, ArrayData{IntVal: intv})
				continue
			}
			if strv, ok := asPortString(item); ok {
				array = append(array, ArrayData{StrVal: strv})
				continue
			}
			if boolv, ok := asPortBool(item); ok {
				array = append(array, ArrayData{BoolVal: boolv})
				continue
			}
			if floatv, ok := asPortFloat(item); ok {
				array = append(array, ArrayData{FloatVal: floatv})
				continue
			}
		}
		return array, true
	default:
		return nil, false
	}
}
