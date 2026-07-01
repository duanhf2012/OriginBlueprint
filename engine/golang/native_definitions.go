package golang

func builtinDynamicDefinition(className string) *NodeDefinition {
	switch className {
	case "LiteralString":
		return NewNodeDefinition(className, func() IExecNode { return &LiteralString{} }, portList("string"), portList("string"))
	case "CastIntegerString":
		return NewNodeDefinition(className, func() IExecNode { return &CastIntegerString{} }, portList("integer"), portList("string"))
	case "CastFloatString":
		return NewNodeDefinition(className, func() IExecNode { return &CastFloatString{} }, portList("float"), portList("string"))
	case "CastAnyString":
		return NewNodeDefinition(className, func() IExecNode { return &CastAnyString{} }, portList("exec", "any"), portList("exec", "bool", "string"))
	case "AddFloat":
		return NewNodeDefinition(className, func() IExecNode { return &AddFloat{} }, portList("float", "float"), portList("float"))
	case "SubFloat":
		return NewNodeDefinition(className, func() IExecNode { return &SubFloat{} }, portList("float", "float"), portList("float"))
	case "MulFloat":
		return NewNodeDefinition(className, func() IExecNode { return &MulFloat{} }, portList("float", "float"), portList("float"))
	case "DivFloat":
		return NewNodeDefinition(className, func() IExecNode { return &DivFloat{} }, portList("float", "float"), portList("float"))
	case "CompareGreaterInteger":
		return NewNodeDefinition(className, func() IExecNode { return &CompareGreaterInteger{} }, portList("integer", "integer"), portList("bool", "integer", "integer"))
	case "StringSplit":
		return NewNodeDefinition(className, func() IExecNode { return &StringSplit{} }, portList("exec", "string", "string"), portList("exec", "array"))
	case "GetArrayAny":
		return NewNodeDefinition(className, func() IExecNode { return &GetArrayAny{} }, portList("array", "integer"), portList("any"))
	case "WhileNode":
		return NewNodeDefinition(className, func() IExecNode { return &WhileNode{} }, portList("exec", "bool"), portList("exec", "exec"))
	case "ForLoopBreak":
		return NewNodeDefinition(className, func() IExecNode { return &ForLoopBreak{} }, portList("exec", "integer", "integer", "exec"), portList("exec", "integer", "exec"))
	case "ForeachArray":
		return NewNodeDefinition(className, func() IExecNode { return &ForeachArray{} }, portList("exec", "array"), portList("exec", "exec", "any", "integer"))
	default:
		return nil
	}
}

func portList(types ...string) []IPort {
	ports := make([]IPort, 0, len(types))
	for _, typ := range types {
		if typ == "exec" {
			ports = append(ports, NewPortExec())
			continue
		}
		port, err := newPortFromDataType(typ)
		if err != nil {
			return nil
		}
		ports = append(ports, port)
	}
	return ports
}
