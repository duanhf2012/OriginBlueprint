package golang

import "fmt"

type PortInt int64
type PortArray []any

type portKind uint8

const (
	portKindExec portKind = iota
	portKindInt
	portKindArray
)

type IPort interface {
	Clone() IPort
	IsPortExec() bool
	SetValue(IPort)
	GetInt() (PortInt, bool)
	setAnyValue(any) error
}

type Port struct {
	kind portKind
	intv PortInt
	arrv PortArray
}

func NewPortExec() IPort {
	return &Port{kind: portKindExec}
}

func NewPortInt() IPort {
	return &Port{kind: portKindInt}
}

func NewPortArray() IPort {
	return &Port{kind: portKindArray}
}

func (p *Port) Clone() IPort {
	if p == nil {
		return nil
	}
	clone := *p
	if p.arrv != nil {
		clone.arrv = append(PortArray(nil), p.arrv...)
	}
	return &clone
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
	p.arrv = append(p.arrv[:0], sourcePort.arrv...)
}

func (p *Port) GetInt() (PortInt, bool) {
	if p == nil || p.kind != portKindInt {
		return 0, false
	}
	return p.intv, true
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
	case portKindArray:
		arrayv, ok := value.(PortArray)
		if !ok {
			return fmt.Errorf("port expects array, got %T", value)
		}
		p.arrv = append(p.arrv[:0], arrayv...)
		return nil
	case portKindExec:
		return fmt.Errorf("can not assign data to exec port")
	default:
		return fmt.Errorf("unknown port kind %d", p.kind)
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
