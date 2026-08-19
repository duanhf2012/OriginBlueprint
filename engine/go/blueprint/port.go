package blueprint

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// PortInt 是蓝图整数端口值。
type PortInt = int64

// PortFloat 是蓝图浮点端口值。
type PortFloat = float64

// PortString 是蓝图字符串端口值。
type PortString = string

// PortBool 是蓝图布尔端口值。
type PortBool = bool

// PortArray 是蓝图数组端口值。
type PortArray []ArrayData

// PortAny 是蓝图任意类型端口值。
type PortAny = any

// ArrayData 是数组端口中的单个元素。运行时在 Port 内部维护元素类型，
// 不改变这个公开结构；宿主直接构造的旧 PortArray 继续使用字段兼容语义。
type ArrayData struct {
	IntVal   PortInt
	FloatVal PortFloat
	StrVal   PortString
	BoolVal  PortBool
}

// arrayElementKinds is runtime-only metadata. It deliberately stays outside
// ArrayData so the public PortArray shape and persisted graph format remain
// compatible with existing callers and legacy files.
type arrayElementKinds uint8

const (
	arrayElementInteger arrayElementKinds = 1 << iota
	arrayElementFloat
	arrayElementString
	arrayElementBoolean
)

// typedPortArray carries element kinds across internal Any/function bindings.
// Public GetAny calls unwrap it back to PortArray.
type typedPortArray struct {
	values PortArray
	kinds  []arrayElementKinds
}

type Port_Int = PortInt
type Port_Float = PortFloat
type Port_Str = PortString
type Port_Bool = PortBool
type Port_Array = PortArray

type portKind uint8

const (
	portKindExec portKind = iota
	portKindInt
	portKindFloat
	portKindString
	portKindBool
	portKindArray
	portKindAny
	portKindTimerHandle
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
	GetTimerHandle() (TimerHandle, bool)
	GetArrayLen() PortInt
	GetArrayValInt(int) (PortInt, bool)
	GetArrayValStr(int) (PortString, bool)
	SetInt(PortInt) bool
	SetFloat(PortFloat) bool
	SetStr(PortString) bool
	SetBool(PortBool) bool
	SetTimerHandle(TimerHandle) bool
	AppendArrayValInt(PortInt) bool
	AppendArrayValStr(PortString) bool
	GetAny() any
	SetAny(any) bool
	setAnyValue(any) error
}

// Port 是 IPort 的默认实现。
type Port struct {
	kind     portKind
	intv     PortInt
	floatv   PortFloat
	strv     PortString
	boolv    PortBool
	arrv     PortArray
	arrKinds []arrayElementKinds
	anyv     any
	timerv   TimerHandle
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

func NewPortTimerHandle() IPort {
	return &Port{kind: portKindTimerHandle}
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
	if source.arrKinds != nil {
		clone.arrKinds = append([]arrayElementKinds(nil), source.arrKinds...)
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
	p.arrKinds = append(p.arrKinds[:0], sourcePort.arrKinds...)
	p.anyv = cloneAnyValue(sourcePort.anyv)
	p.timerv = sourcePort.timerv
}

func assignPortValue(target, source IPort) error {
	targetPort, targetBuiltin := target.(*Port)
	sourcePort, sourceBuiltin := source.(*Port)
	if !targetBuiltin || !sourceBuiltin {
		if target == nil || source == nil {
			return fmt.Errorf("port assignment uses nil port")
		}
		target.SetValue(source)
		return nil
	}
	if targetPort == nil || sourcePort == nil {
		return fmt.Errorf("port assignment uses nil port")
	}
	if targetPort.kind == portKindExec || sourcePort.kind == portKindExec {
		return fmt.Errorf("can not assign exec port")
	}
	if targetPort.kind == portKindAny {
		if sourcePort.kind == portKindAny {
			targetPort.anyv = cloneAnyValue(sourcePort.anyv)
		} else {
			targetPort.anyv = cloneAnyValue(portAnyValue(sourcePort))
		}
		return nil
	}
	if sourcePort.kind == portKindAny {
		return targetPort.setAnyValue(cloneAnyValue(sourcePort.anyv))
	}
	if targetPort.kind != sourcePort.kind {
		return fmt.Errorf("can not assign port kind %d to %d", sourcePort.kind, targetPort.kind)
	}
	switch targetPort.kind {
	case portKindInt:
		targetPort.intv = sourcePort.intv
	case portKindFloat:
		targetPort.floatv = sourcePort.floatv
	case portKindString:
		targetPort.strv = sourcePort.strv
	case portKindBool:
		targetPort.boolv = sourcePort.boolv
	case portKindArray:
		targetPort.arrv = append(targetPort.arrv[:0], sourcePort.arrv...)
		targetPort.arrKinds = append(targetPort.arrKinds[:0], sourcePort.arrKinds...)
	case portKindTimerHandle:
		targetPort.timerv = sourcePort.timerv
	default:
		return fmt.Errorf("unknown port kind %d", targetPort.kind)
	}
	return nil
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

func (p *Port) GetTimerHandle() (TimerHandle, bool) {
	if p == nil || p.kind != portKindTimerHandle {
		return TimerHandle{}, false
	}
	return p.timerv, true
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
	if !p.arrayElementAllows(index, arrayElementInteger) {
		return 0, false
	}
	return p.arrv[index].IntVal, true
}

func (p *Port) GetArrayValStr(index int) (PortString, bool) {
	if p == nil || p.kind != portKindArray || index < 0 || index >= len(p.arrv) {
		return "", false
	}
	if !p.arrayElementAllows(index, arrayElementString) {
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

func (p *Port) SetTimerHandle(value TimerHandle) bool {
	if p == nil || p.kind != portKindTimerHandle {
		return false
	}
	p.timerv = value
	return true
}

func (p *Port) AppendArrayValInt(value PortInt) bool {
	if p == nil || p.kind != portKindArray {
		return false
	}
	p.appendArrayElement(ArrayData{IntVal: value}, arrayElementInteger)
	return true
}

func (p *Port) AppendArrayValStr(value PortString) bool {
	if p == nil || p.kind != portKindArray {
		return false
	}
	p.appendArrayElement(ArrayData{StrVal: value}, arrayElementString)
	return true
}

func (p *Port) GetAny() any {
	if p == nil {
		return nil
	}
	if p.kind == portKindAny {
		value := cloneAnyValue(p.anyv)
		if array, ok := value.(typedPortArray); ok {
			return append(PortArray(nil), array.values...)
		}
		return value
	}
	if p.kind == portKindArray {
		return append(PortArray(nil), p.arrv...)
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
		arrayv, ok := asTypedPortArray(value)
		if !ok {
			return fmt.Errorf("port expects array, got %T", value)
		}
		p.setArrayValue(arrayv)
		return nil
	case portKindAny:
		p.anyv = cloneAnyValue(value)
		return nil
	case portKindTimerHandle:
		handle, ok := value.(TimerHandle)
		if !ok {
			return fmt.Errorf("port expects TimerHandle, got %T", value)
		}
		p.timerv = handle
		return nil
	case portKindExec:
		return fmt.Errorf("can not assign data to exec port")
	default:
		return fmt.Errorf("unknown port kind %d", p.kind)
	}
}

func cloneAnyValue(value any) any {
	switch v := value.(type) {
	case typedPortArray:
		return cloneTypedPortArray(v)
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

func cloneTypedPortArray(source typedPortArray) typedPortArray {
	return typedPortArray{
		values: append(PortArray(nil), source.values...),
		kinds:  append([]arrayElementKinds(nil), source.kinds...),
	}
}

func (p *Port) setArrayValue(value typedPortArray) {
	p.arrv = append(p.arrv[:0], value.values...)
	p.arrKinds = append(p.arrKinds[:0], value.kinds...)
	for len(p.arrKinds) < len(p.arrv) {
		p.arrKinds = append(p.arrKinds, 0)
	}
	if len(p.arrKinds) > len(p.arrv) {
		p.arrKinds = p.arrKinds[:len(p.arrv)]
	}
}

func (p *Port) appendArrayElement(value ArrayData, kinds arrayElementKinds) {
	for len(p.arrKinds) < len(p.arrv) {
		p.arrKinds = append(p.arrKinds, 0)
	}
	p.arrv = append(p.arrv, value)
	p.arrKinds = append(p.arrKinds, kinds)
}

func (p *Port) arrayElementAllows(index int, expected arrayElementKinds) bool {
	if index < 0 || index >= len(p.arrv) {
		return false
	}
	// Missing metadata means a caller supplied a legacy PortArray. Preserve its
	// historical field-based behavior because zero values cannot be inferred.
	if index >= len(p.arrKinds) || p.arrKinds[index] == 0 {
		return true
	}
	return p.arrKinds[index]&expected != 0
}

func (p *Port) arrayElementTypeLabel(index int) string {
	if p == nil || index < 0 || index >= len(p.arrv) || index >= len(p.arrKinds) || p.arrKinds[index] == 0 {
		return "Legacy/Unknown"
	}
	kinds := p.arrKinds[index]
	labels := make([]string, 0, 2)
	if kinds&arrayElementInteger != 0 {
		labels = append(labels, "Integer")
	}
	if kinds&arrayElementFloat != 0 {
		labels = append(labels, "Float")
	}
	if kinds&arrayElementString != 0 {
		labels = append(labels, "String")
	}
	if kinds&arrayElementBoolean != 0 {
		labels = append(labels, "Boolean")
	}
	return strings.Join(labels, "/")
}

func describeArrayElementType(port IPort, index int) string {
	if concrete, ok := port.(*Port); ok && concrete != nil {
		return concrete.arrayElementTypeLabel(index)
	}
	return "Unknown"
}

func asPortInt(value any) (PortInt, bool) {
	switch v := value.(type) {
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
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Trunc(v) != v || v < -9223372036854775808.0 || v >= 9223372036854775808.0 {
			return 0, false
		}
		return PortInt(v), true
	case float32:
		converted := float64(v)
		if math.IsNaN(converted) || math.IsInf(converted, 0) || math.Trunc(converted) != converted || converted < -9223372036854775808.0 || converted >= 9223372036854775808.0 {
			return 0, false
		}
		return PortInt(v), true
	case uint:
		if uint64(v) > uint64(math.MaxInt64) {
			return 0, false
		}
		return PortInt(v), true
	case uint8:
		return PortInt(v), true
	case uint16:
		return PortInt(v), true
	case uint32:
		return PortInt(v), true
	case uint64:
		if v > uint64(math.MaxInt64) {
			return 0, false
		}
		return PortInt(v), true
	default:
		return 0, false
	}
}

func asPortFloat(value any) (PortFloat, bool) {
	switch v := value.(type) {
	case float64:
		return PortFloat(v), true
	case float32:
		return PortFloat(v), true
	case int:
		return PortFloat(v), true
	case int64:
		return PortFloat(v), true
	default:
		return 0, false
	}
}

func asPortString(value any) (PortString, bool) {
	switch v := value.(type) {
	case string:
		return PortString(v), true
	default:
		return "", false
	}
}

func asPortBool(value any) (PortBool, bool) {
	switch v := value.(type) {
	case bool:
		return PortBool(v), true
	case int:
		return PortBool(v != 0), true
	case int64:
		return PortBool(v != 0), true
	default:
		return false, false
	}
}

// arrayDataFromString 将字符串转换为数组元素。
// 若字符串可解析为整数，同时填充 IntVal 与 StrVal（兼容数字与字符串两种读取方式）；
// 否则仅填充 StrVal，IntVal 保持默认 0。
func arrayDataFromString(s string) ArrayData {
	if intv, err := strconv.ParseInt(s, 10, 64); err == nil {
		return ArrayData{IntVal: PortInt(intv), StrVal: PortString(s)}
	}
	return ArrayData{StrVal: PortString(s)}
}

func asPortArray(value any) (PortArray, bool) {
	array, ok := asTypedPortArray(value)
	if !ok {
		return nil, false
	}
	return array.values, true
}

func asTypedPortArray(value any) (typedPortArray, bool) {
	switch v := value.(type) {
	case typedPortArray:
		return cloneTypedPortArray(v), true
	case PortArray:
		return typedPortArray{values: append(PortArray(nil), v...), kinds: inferArrayElementKinds(v)}, true
	case []ArrayData:
		values := append(PortArray(nil), v...)
		return typedPortArray{values: values, kinds: inferArrayElementKinds(values)}, true
	case []int:
		array := make(PortArray, 0, len(v))
		kinds := make([]arrayElementKinds, 0, len(v))
		for _, item := range v {
			array = append(array, ArrayData{IntVal: PortInt(item)})
			kinds = append(kinds, arrayElementInteger)
		}
		return typedPortArray{values: array, kinds: kinds}, true
	case []int64:
		array := make(PortArray, 0, len(v))
		kinds := make([]arrayElementKinds, 0, len(v))
		for _, item := range v {
			array = append(array, ArrayData{IntVal: PortInt(item)})
			kinds = append(kinds, arrayElementInteger)
		}
		return typedPortArray{values: array, kinds: kinds}, true
	case []string:
		array := make(PortArray, 0, len(v))
		kinds := make([]arrayElementKinds, 0, len(v))
		for _, item := range v {
			array = append(array, arrayDataFromString(item))
			kinds = append(kinds, stringArrayElementKinds(item))
		}
		return typedPortArray{values: array, kinds: kinds}, true
	case []any:
		array := make(PortArray, 0, len(v))
		kinds := make([]arrayElementKinds, 0, len(v))
		for _, item := range v {
			if intv, ok := asPortInt(item); ok {
				array = append(array, ArrayData{IntVal: intv})
				kinds = append(kinds, arrayElementInteger)
				continue
			}
			if strv, ok := asPortString(item); ok {
				array = append(array, arrayDataFromString(strv))
				kinds = append(kinds, stringArrayElementKinds(strv))
				continue
			}
			if boolv, ok := asPortBool(item); ok {
				array = append(array, ArrayData{BoolVal: boolv})
				kinds = append(kinds, arrayElementBoolean)
				continue
			}
			if floatv, ok := asPortFloat(item); ok {
				array = append(array, ArrayData{FloatVal: floatv})
				kinds = append(kinds, arrayElementFloat)
				continue
			}
		}
		return typedPortArray{values: array, kinds: kinds}, true
	default:
		return typedPortArray{}, false
	}
}

func stringArrayElementKinds(value string) arrayElementKinds {
	if _, err := strconv.ParseInt(value, 10, 64); err == nil {
		return arrayElementInteger | arrayElementString
	}
	return arrayElementString
}

func inferArrayElementKinds(values PortArray) []arrayElementKinds {
	kinds := make([]arrayElementKinds, len(values))
	for index, value := range values {
		if value.IntVal != 0 {
			kinds[index] |= arrayElementInteger
		}
		if value.FloatVal != 0 {
			kinds[index] |= arrayElementFloat
		}
		if value.StrVal != "" {
			kinds[index] |= arrayElementString
		}
		if value.BoolVal {
			kinds[index] |= arrayElementBoolean
		}
	}
	return kinds
}
