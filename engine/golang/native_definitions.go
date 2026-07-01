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
	case "FilePath":
		return NewNodeDefinition(className, func() IExecNode { return &FilePath{} }, portList("string"), portList("file"))
	case "SaveFilePath":
		return NewNodeDefinition(className, func() IExecNode { return &SaveFilePath{} }, portList("string"), portList("file"))
	case "ReadText":
		return NewNodeDefinition(className, func() IExecNode { return &ReadText{} }, portList("exec", "file"), portList("exec", "string", "exec"))
	case "SaveText":
		return NewNodeDefinition(className, func() IExecNode { return &SaveText{} }, portList("exec", "file", "string"), portList("exec"))
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
	case "ForeachTableRow":
		return NewNodeDefinition(className, func() IExecNode { return &ForeachTableRow{} }, portList("exec", "table"), portList("exec", "exec", "dictionary", "integer"))
	case "DictionarySet":
		return NewNodeDefinition(className, func() IExecNode { return &DictionarySet{} }, portList("exec", "dictionary", "string", "any"), portList("exec", "dictionary"))
	case "DictionarySize":
		return NewNodeDefinition(className, func() IExecNode { return &DictionarySize{} }, portList("dictionary"), portList("integer"))
	case "DictionaryKeys":
		return NewNodeDefinition(className, func() IExecNode { return &DictionaryKeys{} }, portList("dictionary"), portList("array"))
	case "ReadCSV":
		return NewNodeDefinition(className, func() IExecNode { return &ReadCSV{} }, portList("exec", "file", "string", "bool"), portList("exec", "table", "exec"))
	case "SaveCSV":
		return NewNodeDefinition(className, func() IExecNode { return &SaveCSV{} }, portList("exec", "table", "file"), portList("exec", "table"))
	case "TableRowCount":
		return NewNodeDefinition(className, func() IExecNode { return &TableRowCount{} }, portList("exec", "table"), portList("exec", "integer"))
	case "TableHeaders":
		return NewNodeDefinition(className, func() IExecNode { return &TableHeaders{} }, portList("exec", "table"), portList("exec", "array"))
	case "TableMerge":
		return NewNodeDefinition(className, func() IExecNode { return &TableMerge{} }, portList("exec", "table", "table", "string"), portList("exec", "table"))
	case "TableSelectColumns":
		return NewNodeDefinition(className, func() IExecNode { return &TableSelectColumns{} }, portList("exec", "table", "array"), portList("exec", "table"))
	case "TablePrint":
		return NewNodeDefinition(className, func() IExecNode { return &TablePrint{} }, portList("exec", "table"), portList("exec", "table"))
	case "TableSort":
		return NewNodeDefinition(className, func() IExecNode { return &TableSort{} }, portList("exec", "table", "string", "bool"), portList("exec", "table"))
	case "TableFilterEqual":
		return NewNodeDefinition(className, func() IExecNode { return &TableFilterEqual{} }, portList("exec", "table", "string", "any"), portList("exec", "table"))
	case "TableRenameColumn":
		return NewNodeDefinition(className, func() IExecNode { return &TableRenameColumn{} }, portList("exec", "table", "string", "string"), portList("exec", "table"))
	case "TableDropColumns":
		return NewNodeDefinition(className, func() IExecNode { return &TableDropColumns{} }, portList("exec", "table", "array"), portList("exec", "table"))
	case "TableFillEmpty":
		return NewNodeDefinition(className, func() IExecNode { return &TableFillEmpty{} }, portList("exec", "table", "any"), portList("exec", "table"))
	case "TableGetColumn":
		return NewNodeDefinition(className, func() IExecNode { return &TableGetColumn{} }, portList("table", "string"), portList("array"))
	case "TablePreview":
		return NewNodeDefinition(className, func() IExecNode { return &TablePreview{} }, portList("table"), nil)
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
