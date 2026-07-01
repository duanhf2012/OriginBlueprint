package golang

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
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

type FilePath struct{ BaseExecNode }
type SaveFilePath struct{ BaseExecNode }
type ReadText struct{ BaseExecNode }
type SaveText struct{ BaseExecNode }

func (n *FilePath) GetName() string     { return "FilePath" }
func (n *SaveFilePath) GetName() string { return "SaveFilePath" }
func (n *ReadText) GetName() string     { return "ReadText" }
func (n *SaveText) GetName() string     { return "SaveText" }
func (n *FilePath) Exec() (int, error) {
	value, _ := n.GetInPortStr(0)
	n.GetOutPort(0).setAnyValue(string(value))
	return -1, nil
}
func (n *SaveFilePath) Exec() (int, error) { return (&FilePath{BaseExecNode: n.BaseExecNode}).Exec() }
func (n *ReadText) Exec() (int, error) {
	path := fmt.Sprint(portAnyValue(n.GetInPort(1)))
	data, err := os.ReadFile(path)
	if err != nil {
		return 2, nil
	}
	n.SetOutPortStr(1, PortString(data))
	return 0, nil
}
func (n *SaveText) Exec() (int, error) {
	path := fmt.Sprint(portAnyValue(n.GetInPort(1)))
	text, _ := n.GetInPortStr(2)
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		return -1, err
	}
	return 0, nil
}

type StringSplit struct{ BaseExecNode }
type GetArrayAny struct{ BaseExecNode }
type WhileNode struct{ BaseExecNode }
type ForLoopBreak struct{ BaseExecNode }
type ForeachArray struct{ BaseExecNode }
type ForeachTableRow struct{ BaseExecNode }

func (n *StringSplit) GetName() string  { return "StringSplit" }
func (n *GetArrayAny) GetName() string  { return "GetArrayAny" }
func (n *WhileNode) GetName() string    { return "WhileNode" }
func (n *ForLoopBreak) GetName() string { return "ForLoopBreak" }
func (n *ForeachArray) GetName() string { return "ForeachArray" }
func (n *ForeachTableRow) GetName() string {
	return "ForeachTableRow"
}
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
	for guard := 0; guard < 100000; guard++ {
		condition, _ := n.GetInPortBool(1)
		if !condition {
			return 1, nil
		}
		if err := n.DoNext(0); err != nil {
			return -1, err
		}
	}
	return -1, fmt.Errorf("WhileNode exceeded max iterations")
}
func (n *ForLoopBreak) Exec() (int, error) {
	start, _ := n.GetInPortInt(1)
	end, _ := n.GetInPortInt(2)
	for index := start; index < end; index++ {
		n.SetOutPortInt(1, index)
		if err := n.DoNext(0); err != nil {
			return -1, err
		}
	}
	return 2, nil
}
func (n *ForeachArray) Exec() (int, error) {
	array, _ := n.GetInPortArray(1)
	for index, item := range array {
		n.GetOutPort(2).setAnyValue(item)
		n.SetOutPortInt(3, PortInt(index))
		if err := n.DoNext(0); err != nil {
			return -1, err
		}
	}
	return 1, nil
}
func (n *ForeachTableRow) Exec() (int, error) {
	table := asTable(portAnyValue(n.GetInPort(1)))
	for index, row := range table.Rows {
		n.GetOutPort(2).setAnyValue(row)
		n.SetOutPortInt(3, PortInt(index))
		if err := n.DoNext(0); err != nil {
			return -1, err
		}
	}
	return 1, nil
}

type DictionarySet struct{ BaseExecNode }
type DictionarySize struct{ BaseExecNode }
type DictionaryKeys struct{ BaseExecNode }

func (n *DictionarySet) GetName() string  { return "DictionarySet" }
func (n *DictionarySize) GetName() string { return "DictionarySize" }
func (n *DictionaryKeys) GetName() string { return "DictionaryKeys" }
func (n *DictionarySet) Exec() (int, error) {
	dict := asDictionary(portAnyValue(n.GetInPort(1)))
	key, _ := n.GetInPortStr(2)
	dict[string(key)] = portAnyValue(n.GetInPort(3))
	n.GetOutPort(1).setAnyValue(dict)
	return 0, nil
}
func (n *DictionarySize) Exec() (int, error) {
	n.SetOutPortInt(0, PortInt(len(asDictionary(portAnyValue(n.GetInPort(0))))))
	return -1, nil
}
func (n *DictionaryKeys) Exec() (int, error) {
	dict := asDictionary(portAnyValue(n.GetInPort(0)))
	keys := make([]string, 0, len(dict))
	for key := range dict {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	array := make(PortArray, 0, len(keys))
	for _, key := range keys {
		array = append(array, ArrayData{StrVal: PortString(key)})
	}
	n.GetOutPort(0).setAnyValue(array)
	return -1, nil
}

func asDictionary(value any) map[string]any {
	if dict, ok := value.(map[string]any); ok {
		clone := make(map[string]any, len(dict))
		for key, item := range dict {
			clone[key] = item
		}
		return clone
	}
	return map[string]any{}
}

type ReadCSV struct{ BaseExecNode }
type SaveCSV struct{ BaseExecNode }
type TableRowCount struct{ BaseExecNode }
type TableHeaders struct{ BaseExecNode }
type TableMerge struct{ BaseExecNode }
type TableSelectColumns struct{ BaseExecNode }
type TablePrint struct{ BaseExecNode }
type TableSort struct{ BaseExecNode }
type TableFilterEqual struct{ BaseExecNode }
type TableRenameColumn struct{ BaseExecNode }
type TableDropColumns struct{ BaseExecNode }
type TableFillEmpty struct{ BaseExecNode }
type TableGetColumn struct{ BaseExecNode }
type TablePreview struct{ BaseExecNode }

func (n *ReadCSV) GetName() string            { return "ReadCSV" }
func (n *SaveCSV) GetName() string            { return "SaveCSV" }
func (n *TableRowCount) GetName() string      { return "TableRowCount" }
func (n *TableHeaders) GetName() string       { return "TableHeaders" }
func (n *TableMerge) GetName() string         { return "TableMerge" }
func (n *TableSelectColumns) GetName() string { return "TableSelectColumns" }
func (n *TablePrint) GetName() string         { return "TablePrint" }
func (n *TableSort) GetName() string          { return "TableSort" }
func (n *TableFilterEqual) GetName() string   { return "TableFilterEqual" }
func (n *TableRenameColumn) GetName() string  { return "TableRenameColumn" }
func (n *TableDropColumns) GetName() string   { return "TableDropColumns" }
func (n *TableFillEmpty) GetName() string     { return "TableFillEmpty" }
func (n *TableGetColumn) GetName() string     { return "TableGetColumn" }
func (n *TablePreview) GetName() string       { return "TablePreview" }

func (n *ReadCSV) Exec() (int, error) {
	path := fmt.Sprint(portAnyValue(n.GetInPort(1)))
	delimiter, _ := n.GetInPortStr(2)
	hasHeader, _ := n.GetInPortBool(3)
	file, err := os.Open(path)
	if err != nil {
		return 2, nil
	}
	defer file.Close()
	reader := csv.NewReader(file)
	if delimiter != "" {
		reader.Comma = []rune(string(delimiter))[0]
	}
	records, err := reader.ReadAll()
	if err != nil {
		return 2, nil
	}
	table := recordsToTable(records, bool(hasHeader))
	n.GetOutPort(1).setAnyValue(table)
	return 0, nil
}

func (n *SaveCSV) Exec() (int, error) {
	path := fmt.Sprint(portAnyValue(n.GetInPort(2)))
	table := asTable(portAnyValue(n.GetInPort(1)))
	file, err := os.Create(path)
	if err != nil {
		return -1, err
	}
	defer file.Close()
	writer := csv.NewWriter(file)
	defer writer.Flush()
	_ = writer.Write(table.Headers)
	for _, row := range table.Rows {
		record := make([]string, 0, len(table.Headers))
		for _, header := range table.Headers {
			record = append(record, fmt.Sprint(row[header]))
		}
		_ = writer.Write(record)
	}
	n.GetOutPort(1).setAnyValue(table)
	return 0, nil
}

func (n *TableRowCount) Exec() (int, error) {
	table := asTable(portAnyValue(n.GetInPort(1)))
	n.SetOutPortInt(1, PortInt(len(table.Rows)))
	return 0, nil
}
func (n *TableHeaders) Exec() (int, error) {
	table := asTable(portAnyValue(n.GetInPort(1)))
	n.GetOutPort(1).setAnyValue(stringsToArray(table.Headers))
	return 0, nil
}
func (n *TableMerge) Exec() (int, error) {
	left := asTable(portAnyValue(n.GetInPort(1)))
	right := asTable(portAnyValue(n.GetInPort(2)))
	left.Rows = append(left.Rows, right.Rows...)
	left.Headers = mergeHeaders(left.Headers, right.Headers)
	n.GetOutPort(1).setAnyValue(left)
	return 0, nil
}
func (n *TableSelectColumns) Exec() (int, error) {
	table := asTable(portAnyValue(n.GetInPort(1)))
	columns := arrayToStrings(portAnyValue(n.GetInPort(2)))
	n.GetOutPort(1).setAnyValue(selectColumns(table, columns))
	return 0, nil
}
func (n *TablePrint) Exec() (int, error) {
	n.GetOutPort(1).setAnyValue(asTable(portAnyValue(n.GetInPort(1))))
	return 0, nil
}
func (n *TableSort) Exec() (int, error) {
	table := asTable(portAnyValue(n.GetInPort(1)))
	column, _ := n.GetInPortStr(2)
	ascending, _ := n.GetInPortBool(3)
	sort.SliceStable(table.Rows, func(i, j int) bool {
		less := fmt.Sprint(table.Rows[i][string(column)]) < fmt.Sprint(table.Rows[j][string(column)])
		return (bool(ascending) && less) || (!bool(ascending) && !less)
	})
	n.GetOutPort(1).setAnyValue(table)
	return 0, nil
}
func (n *TableFilterEqual) Exec() (int, error) {
	table := asTable(portAnyValue(n.GetInPort(1)))
	column, _ := n.GetInPortStr(2)
	value := fmt.Sprint(portAnyValue(n.GetInPort(3)))
	filtered := TableData{Headers: table.Headers}
	for _, row := range table.Rows {
		if fmt.Sprint(row[string(column)]) == value {
			filtered.Rows = append(filtered.Rows, row)
		}
	}
	n.GetOutPort(1).setAnyValue(filtered)
	return 0, nil
}
func (n *TableRenameColumn) Exec() (int, error) {
	table := asTable(portAnyValue(n.GetInPort(1)))
	from, _ := n.GetInPortStr(2)
	to, _ := n.GetInPortStr(3)
	for index, header := range table.Headers {
		if header == string(from) {
			table.Headers[index] = string(to)
		}
	}
	for _, row := range table.Rows {
		row[string(to)] = row[string(from)]
		delete(row, string(from))
	}
	n.GetOutPort(1).setAnyValue(table)
	return 0, nil
}
func (n *TableDropColumns) Exec() (int, error) {
	table := asTable(portAnyValue(n.GetInPort(1)))
	drop := map[string]bool{}
	for _, column := range arrayToStrings(portAnyValue(n.GetInPort(2))) {
		drop[column] = true
	}
	kept := make([]string, 0, len(table.Headers))
	for _, header := range table.Headers {
		if !drop[header] {
			kept = append(kept, header)
		}
	}
	n.GetOutPort(1).setAnyValue(selectColumns(table, kept))
	return 0, nil
}
func (n *TableFillEmpty) Exec() (int, error) {
	table := asTable(portAnyValue(n.GetInPort(1)))
	value := portAnyValue(n.GetInPort(2))
	for _, row := range table.Rows {
		for _, header := range table.Headers {
			if row[header] == nil || row[header] == "" {
				row[header] = value
			}
		}
	}
	n.GetOutPort(1).setAnyValue(table)
	return 0, nil
}
func (n *TableGetColumn) Exec() (int, error) {
	table := asTable(portAnyValue(n.GetInPort(0)))
	column, _ := n.GetInPortStr(1)
	array := make(PortArray, 0, len(table.Rows))
	for _, row := range table.Rows {
		array = append(array, arrayDataFromAny(row[string(column)]))
	}
	n.GetOutPort(0).setAnyValue(array)
	return -1, nil
}
func (n *TablePreview) Exec() (int, error) { return -1, nil }

func recordsToTable(records [][]string, hasHeader bool) TableData {
	if len(records) == 0 {
		return TableData{}
	}
	headers := append([]string(nil), records[0]...)
	start := 1
	if !hasHeader {
		headers = make([]string, len(records[0]))
		for index := range headers {
			headers[index] = fmt.Sprintf("column%d", index+1)
		}
		start = 0
	}
	table := TableData{Headers: headers}
	for _, record := range records[start:] {
		row := map[string]any{}
		for index, header := range headers {
			if index < len(record) {
				row[header] = record[index]
			}
		}
		table.Rows = append(table.Rows, row)
	}
	return table
}

func asTable(value any) TableData {
	switch v := value.(type) {
	case TableData:
		return cloneTableData(v)
	case *TableData:
		if v == nil {
			return TableData{}
		}
		return cloneTableData(*v)
	default:
		return TableData{}
	}
}

func stringsToArray(values []string) PortArray {
	array := make(PortArray, 0, len(values))
	for _, value := range values {
		array = append(array, ArrayData{StrVal: PortString(value)})
	}
	return array
}

func arrayToStrings(value any) []string {
	array, ok := asPortArray(value)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(array))
	for _, item := range array {
		out = append(out, string(item.StrVal))
	}
	return out
}

func mergeHeaders(left []string, right []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(left)+len(right))
	for _, header := range append(left, right...) {
		if !seen[header] {
			seen[header] = true
			out = append(out, header)
		}
	}
	return out
}

func selectColumns(table TableData, columns []string) TableData {
	out := TableData{Headers: append([]string(nil), columns...)}
	for _, row := range table.Rows {
		selected := map[string]any{}
		for _, column := range columns {
			selected[column] = row[column]
		}
		out.Rows = append(out.Rows, selected)
	}
	return out
}
