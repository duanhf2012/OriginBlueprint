package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type RuntimeTable struct {
	Columns []string        `json:"columns"`
	Rows    [][]interface{} `json:"rows"`
}

type ExecutionNodeState struct {
	NodeID string `json:"nodeId"`
	State  string `json:"state"`
}

type ExecutionLog struct {
	Level   string `json:"level"`
	Message string `json:"message"`
	NodeID  string `json:"nodeId,omitempty"`
}

type ExecutionEvent struct {
	SessionID string                 `json:"sessionId"`
	Type      string                 `json:"type"`
	Message   string                 `json:"message,omitempty"`
	States    []ExecutionNodeState   `json:"states,omitempty"`
	Logs      []ExecutionLog         `json:"logs,omitempty"`
	Results   []interface{}          `json:"results,omitempty"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

type graphExecutor struct {
	ctx       context.Context
	document  GraphDocument
	nodes     map[string]GraphNode
	incoming  map[string]map[string]GraphConnection
	outgoing  map[string]map[string][]GraphConnection
	variables map[string]interface{}
	transient map[string]map[string]interface{}
	results   []interface{}
	logs      []ExecutionLog
	allLogs   []ExecutionLog
	states    []ExecutionNodeState
	steps     int
	maxSteps  int
	onBatch   func(ExecutionEvent)
	sessionID string
}

func executeGraph(ctx context.Context, sessionID string, document GraphDocument, onBatch func(ExecutionEvent)) (ExecutionEvent, error) {
	executor := newGraphExecutor(ctx, sessionID, document, onBatch)
	entries := make([]GraphNode, 0)
	for _, node := range document.Nodes {
		if node.TypeID == "origin.event.begin" || node.TypeID == "origin.event.timer" || strings.HasPrefix(node.TypeID, "origin.event.entry-") {
			entries = append(entries, node)
		}
	}
	if len(entries) == 0 {
		return executor.result(), errors.New("graph has no execution entry node")
	}
	for _, entry := range entries {
		if err := executor.runNode(entry.ID); err != nil {
			executor.flush()
			return executor.result(), err
		}
	}
	for _, node := range document.Nodes {
		if node.TypeID != "origin.table.preview" {
			continue
		}
		value, err := executor.input(node, "table", map[string]bool{})
		if err != nil {
			return executor.result(), executor.fail(node, err)
		}
		table, err := asRuntimeTable(value)
		if err != nil {
			return executor.result(), executor.fail(node, err)
		}
		executor.results = append(executor.results, map[string]interface{}{"kind": "table", "nodeId": node.ID, "table": table})
	}
	executor.flush()
	return executor.result(), nil
}

func newGraphExecutor(ctx context.Context, sessionID string, document GraphDocument, onBatch func(ExecutionEvent)) *graphExecutor {
	e := &graphExecutor{ctx: ctx, sessionID: sessionID, document: document, nodes: map[string]GraphNode{}, incoming: map[string]map[string]GraphConnection{}, outgoing: map[string]map[string][]GraphConnection{}, variables: map[string]interface{}{}, transient: map[string]map[string]interface{}{}, maxSteps: 10000, onBatch: onBatch}
	for _, variable := range document.Variables {
		e.variables[variable.ID] = cloneValue(variable.DefaultValue)
	}
	for _, node := range document.Nodes {
		e.nodes[node.ID] = node
	}
	for _, connection := range document.Connections {
		if e.incoming[connection.Target] == nil {
			e.incoming[connection.Target] = map[string]GraphConnection{}
		}
		if e.outgoing[connection.Source] == nil {
			e.outgoing[connection.Source] = map[string][]GraphConnection{}
		}
		e.incoming[connection.Target][connection.TargetInput] = connection
		e.outgoing[connection.Source][connection.SourceOutput] = append(e.outgoing[connection.Source][connection.SourceOutput], connection)
	}
	return e
}

func (e *graphExecutor) result() ExecutionEvent {
	variables := make(map[string]interface{}, len(e.variables))
	for _, variable := range e.document.Variables {
		variables[variable.Name] = cloneValue(e.variables[variable.ID])
	}
	return ExecutionEvent{SessionID: e.sessionID, Logs: append([]ExecutionLog(nil), e.allLogs...), Results: append([]interface{}(nil), e.results...), Variables: variables}
}

func (e *graphExecutor) flush() {
	if e.onBatch == nil || (len(e.states) == 0 && len(e.logs) == 0) {
		return
	}
	e.onBatch(ExecutionEvent{SessionID: e.sessionID, Type: "progress", States: append([]ExecutionNodeState(nil), e.states...), Logs: append([]ExecutionLog(nil), e.logs...)})
	e.states = nil
	e.logs = nil
}

func (e *graphExecutor) check(nodeID string) error {
	select {
	case <-e.ctx.Done():
		return e.ctx.Err()
	default:
	}
	e.steps++
	if e.steps > e.maxSteps {
		return fmt.Errorf("execution stopped after %d steps near node %s", e.maxSteps, nodeID)
	}
	return nil
}

func (e *graphExecutor) runNode(nodeID string) error {
	if err := e.check(nodeID); err != nil {
		return err
	}
	node, ok := e.nodes[nodeID]
	if !ok {
		return fmt.Errorf("execution references missing node %s", nodeID)
	}
	e.states = append(e.states, ExecutionNodeState{NodeID: nodeID, State: "running"})
	if len(e.states) >= 16 {
		e.flush()
	}
	next := []string{"exec"}
	switch node.TypeID {
	case "origin.event.begin", "origin.event.entry-array", "origin.event.entry-two-integers", "origin.event.timer":
		next = []string{"exec"}
	case "origin.flow.branch":
		condition, err := e.input(node, "condition", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		if asBool(condition) {
			next = []string{"true"}
		} else {
			next = []string{"false"}
		}
	case "origin.flow.for-loop":
		startValue, err := e.input(node, "start", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		endValue, err := e.input(node, "end", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		for index := asInt(startValue); index < asInt(endValue); index++ {
			if err := e.check(node.ID); err != nil {
				return e.fail(node, err)
			}
			e.transient[node.ID] = map[string]interface{}{"index": index}
			if err := e.follow(node.ID, "body"); err != nil {
				return err
			}
		}
		next = []string{"completed"}
	case "origin.flow.for-loop-break":
		startValue, err := e.input(node, "start", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		endValue, err := e.input(node, "end", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		for index := asInt(startValue); index < asInt(endValue); index++ {
			if err := e.check(node.ID); err != nil {
				return e.fail(node, err)
			}
			e.transient[node.ID] = map[string]interface{}{"index": index, "break": false}
			if err := e.follow(node.ID, "body"); err != nil {
				return err
			}
			if asBool(e.transient[node.ID]["break"]) {
				break
			}
		}
		next = []string{"completed"}
	case "origin.flow.while":
		for {
			condition, err := e.input(node, "condition", map[string]bool{})
			if err != nil {
				return e.fail(node, err)
			}
			if !asBool(condition) {
				break
			}
			if err := e.check(node.ID); err != nil {
				return e.fail(node, err)
			}
			if err := e.follow(node.ID, "body"); err != nil {
				return err
			}
		}
		next = []string{"completed"}
	case "origin.flow.foreach-integer-array":
		value, err := e.input(node, "array", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		for index, item := range asSlice(value) {
			if err := e.check(node.ID); err != nil {
				return e.fail(node, err)
			}
			e.transient[node.ID] = map[string]interface{}{"index": index, "value": asInt(item)}
			if err := e.follow(node.ID, "body"); err != nil {
				return err
			}
		}
		next = []string{"completed"}
	case "origin.flow.foreach-array":
		value, err := e.input(node, "array", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		for index, item := range asSlice(value) {
			if err := e.check(node.ID); err != nil {
				return e.fail(node, err)
			}
			e.transient[node.ID] = map[string]interface{}{"index": index, "value": cloneValue(item)}
			if err := e.follow(node.ID, "body"); err != nil {
				return err
			}
		}
		next = []string{"completed"}
	case "origin.flow.foreach-table-row":
		value, err := e.input(node, "table", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		table, err := asRuntimeTable(value)
		if err != nil {
			return e.fail(node, err)
		}
		for index, row := range table.Rows {
			if err := e.check(node.ID); err != nil {
				return e.fail(node, err)
			}
			e.transient[node.ID] = map[string]interface{}{"index": index, "row": tableRowDictionary(table.Columns, row)}
			if err := e.follow(node.ID, "body"); err != nil {
				return err
			}
		}
		next = []string{"completed"}
	case "origin.flow.sequence":
		keys := make([]string, 0)
		for key := range e.outgoing[node.ID] {
			if strings.HasPrefix(key, "then") {
				keys = append(keys, key)
			}
		}
		sort.Slice(keys, func(i, j int) bool { return suffixNumber(keys[i]) < suffixNumber(keys[j]) })
		for _, key := range keys {
			if err := e.follow(node.ID, key); err != nil {
				return err
			}
		}
		next = nil
	case "origin.flow.greater-integer", "origin.flow.less-integer", "origin.flow.equal-integer":
		a, err := e.input(node, "a", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		b, err := e.input(node, "b", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		orEqual, _ := e.input(node, "orEqual", map[string]bool{})
		match := asInt(a) == asInt(b)
		if node.TypeID == "origin.flow.greater-integer" {
			match = asInt(a) > asInt(b) || (asBool(orEqual) && asInt(a) == asInt(b))
		}
		if node.TypeID == "origin.flow.less-integer" {
			match = asInt(a) < asInt(b) || (asBool(orEqual) && asInt(a) == asInt(b))
		}
		if match {
			next = []string{"true"}
		} else {
			next = []string{"false"}
		}
	case "origin.flow.probability":
		probability, err := e.input(node, "probability", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		if rand.Intn(10000) < asInt(probability) {
			next = []string{"hit"}
		} else {
			next = []string{"miss"}
		}
	case "origin.flow.range-compare", "origin.flow.equal-switch":
		value, err := e.input(node, "value", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		listKey := "ranges"
		if node.TypeID == "origin.flow.equal-switch" {
			listKey = "cases"
		}
		items, err := e.input(node, listKey, map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		selected := "otherwise"
		for index, item := range asSlice(items) {
			match := asInt(value) <= asInt(item)
			if node.TypeID == "origin.flow.equal-switch" {
				match = asInt(value) == asInt(item)
			}
			if match {
				if index < 5 {
					selected = fmt.Sprintf("case%d", index)
				}
				break
			}
		}
		next = []string{selected}
	case "origin.variable.set":
		value, err := e.input(node, "value", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		e.variables[node.Properties.VariableID] = cloneValue(value)
	case "origin.io.read-text":
		fileValue, err := e.input(node, "file", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		data, err := os.ReadFile(fmt.Sprint(fileValue))
		if err != nil {
			e.log("error", fmt.Sprintf("read file: %v", err), node.ID)
			next = []string{"error"}
			break
		}
		text := string(data)
		e.transient[node.ID] = map[string]interface{}{"text": text}
	case "origin.io.save-text":
		fileValue, err := e.input(node, "file", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		textValue, err := e.input(node, "text", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		if err := os.WriteFile(fmt.Sprint(fileValue), []byte(fmt.Sprint(textValue)), 0644); err != nil {
			return e.fail(node, fmt.Errorf("save file: %w", err))
		}
	case "origin.table.read-csv":
		fileValue, err := e.input(node, "file", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		delimiterValue, _ := e.input(node, "delimiter", map[string]bool{})
		headerValue, _ := e.input(node, "header", map[string]bool{})
		table, err := readCSVTable(fmt.Sprint(fileValue), fmt.Sprint(delimiterValue), asBool(headerValue))
		if err != nil {
			e.log("error", err.Error(), node.ID)
			next = []string{"error"}
		} else {
			e.transient[node.ID] = map[string]interface{}{"table": table}
			next = []string{"exec"}
		}
	case "origin.table.save-csv":
		tableValue, err := e.input(node, "table", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		table, err := asRuntimeTable(tableValue)
		if err != nil {
			return e.fail(node, err)
		}
		fileValue, err := e.input(node, "file", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		if err := writeCSVTable(fmt.Sprint(fileValue), table); err != nil {
			return e.fail(node, err)
		}
		e.transient[node.ID] = map[string]interface{}{"table": table}
	case "origin.table.row-count":
		value, err := e.input(node, "table", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		table, err := asRuntimeTable(value)
		if err != nil {
			return e.fail(node, err)
		}
		e.transient[node.ID] = map[string]interface{}{"count": len(table.Rows)}
	case "origin.table.headers":
		value, err := e.input(node, "table", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		table, err := asRuntimeTable(value)
		if err != nil {
			return e.fail(node, err)
		}
		e.transient[node.ID] = map[string]interface{}{"headers": stringsToInterfaces(table.Columns)}
	case "origin.table.merge":
		leftValue, err := e.input(node, "left", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		rightValue, err := e.input(node, "right", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		keyValue, _ := e.input(node, "key", map[string]bool{})
		merged, err := mergeTables(leftValue, rightValue, fmt.Sprint(keyValue))
		if err != nil {
			return e.fail(node, err)
		}
		e.transient[node.ID] = map[string]interface{}{"table": merged}
	case "origin.table.select-columns":
		tableValue, err := e.input(node, "table", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		columnsValue, err := e.input(node, "columns", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		table, err := selectTableColumns(tableValue, interfaceStrings(asSlice(columnsValue)))
		if err != nil {
			return e.fail(node, err)
		}
		e.transient[node.ID] = map[string]interface{}{"table": table}
	case "origin.table.print":
		tableValue, err := e.input(node, "table", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		table, err := asRuntimeTable(tableValue)
		if err != nil {
			return e.fail(node, err)
		}
		e.transient[node.ID] = map[string]interface{}{"table": table}
		e.results = append(e.results, map[string]interface{}{"kind": "table", "nodeId": node.ID, "table": table})
		e.log("info", fmt.Sprintf("Table: %d rows x %d columns", len(table.Rows), len(table.Columns)), node.ID)
	case "origin.table.sort":
		tableValue, err := e.input(node, "table", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		columnValue, _ := e.input(node, "column", map[string]bool{})
		ascendingValue, _ := e.input(node, "ascending", map[string]bool{})
		table, err := sortTable(tableValue, fmt.Sprint(columnValue), asBool(ascendingValue))
		if err != nil {
			return e.fail(node, err)
		}
		e.transient[node.ID] = map[string]interface{}{"table": table}
	case "origin.table.filter-equal":
		tableValue, err := e.input(node, "table", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		columnValue, _ := e.input(node, "column", map[string]bool{})
		matchValue, err := e.input(node, "value", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		table, err := filterTableEqual(tableValue, fmt.Sprint(columnValue), matchValue)
		if err != nil {
			return e.fail(node, err)
		}
		e.transient[node.ID] = map[string]interface{}{"table": table}
	case "origin.table.rename-column":
		tableValue, err := e.input(node, "table", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		fromValue, _ := e.input(node, "from", map[string]bool{})
		toValue, _ := e.input(node, "to", map[string]bool{})
		table, err := renameTableColumn(tableValue, fmt.Sprint(fromValue), fmt.Sprint(toValue))
		if err != nil {
			return e.fail(node, err)
		}
		e.transient[node.ID] = map[string]interface{}{"table": table}
	case "origin.table.drop-columns":
		tableValue, err := e.input(node, "table", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		columnsValue, err := e.input(node, "columns", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		table, err := dropTableColumns(tableValue, interfaceStrings(asSlice(columnsValue)))
		if err != nil {
			return e.fail(node, err)
		}
		e.transient[node.ID] = map[string]interface{}{"table": table}
	case "origin.table.fill-empty":
		tableValue, err := e.input(node, "table", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		replacement, err := e.input(node, "value", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		table, err := fillEmptyTableCells(tableValue, replacement)
		if err != nil {
			return e.fail(node, err)
		}
		e.transient[node.ID] = map[string]interface{}{"table": table}
	case "origin.string.split":
		textValue, err := e.input(node, "text", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		delimiterValue, _ := e.input(node, "delimiter", map[string]bool{})
		delimiter := fmt.Sprint(delimiterValue)
		parts := []string{fmt.Sprint(textValue)}
		if delimiter != "" {
			parts = strings.Split(fmt.Sprint(textValue), delimiter)
		}
		e.transient[node.ID] = map[string]interface{}{"array": stringsToInterfaces(parts)}
	case "origin.cast.any-string":
		value, err := e.input(node, "value", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		e.transient[node.ID] = map[string]interface{}{"valid": true, "result": fmt.Sprint(value)}
	case "origin.dictionary.set":
		dictionaryValue, err := e.input(node, "dictionary", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		keyValue, _ := e.input(node, "key", map[string]bool{})
		value, err := e.input(node, "value", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		dictionary := asDictionary(dictionaryValue)
		dictionary[fmt.Sprint(keyValue)] = cloneValue(value)
		e.transient[node.ID] = map[string]interface{}{"dictionary": dictionary}
	case "origin.action.print":
		value, err := e.input(node, "value", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		e.log("info", fmt.Sprint(value), node.ID)
	case "origin.debug.output":
		parts := make([]string, 0, 3)
		for _, key := range []string{"integer", "string", "array"} {
			value, err := e.input(node, key, map[string]bool{})
			if err == nil {
				parts = append(parts, fmt.Sprintf("%s=%v", key, value))
			}
		}
		e.log("debug", strings.Join(parts, "  "), node.ID)
	case "origin.result.append-integer", "origin.result.append-string":
		value, err := e.input(node, "value", map[string]bool{})
		if err != nil {
			return e.fail(node, err)
		}
		e.results = append(e.results, cloneValue(value))
	case "origin.timer.create":
		e.transient[node.ID] = map[string]interface{}{"timerId": time.Now().UnixNano()}
		e.log("warning", "Timer node executed in preview mode; no persistent timer was scheduled", node.ID)
	case "origin.timer.close":
		e.log("warning", "Timer close executed in preview mode", node.ID)
	default:
		return e.fail(node, fmt.Errorf("node type %s is not executable as a flow node", node.TypeID))
	}
	e.states = append(e.states, ExecutionNodeState{NodeID: nodeID, State: "completed"})
	for _, output := range next {
		if err := e.follow(node.ID, output); err != nil {
			return err
		}
	}
	return nil
}

func (e *graphExecutor) fail(node GraphNode, err error) error {
	e.states = append(e.states, ExecutionNodeState{NodeID: node.ID, State: "error"})
	e.log("error", err.Error(), node.ID)
	return fmt.Errorf("node %s: %w", node.ID, err)
}

func (e *graphExecutor) log(level, message, nodeID string) {
	entry := ExecutionLog{Level: level, Message: message, NodeID: nodeID}
	e.logs = append(e.logs, entry)
	e.allLogs = append(e.allLogs, entry)
}

func (e *graphExecutor) follow(nodeID, output string) error {
	for _, connection := range e.outgoing[nodeID][output] {
		if target := e.nodes[connection.Target]; target.TypeID == "origin.flow.for-loop-break" && connection.TargetInput == "break" {
			if e.transient[target.ID] == nil {
				e.transient[target.ID] = map[string]interface{}{}
			}
			e.transient[target.ID]["break"] = true
			continue
		}
		if err := e.runNode(connection.Target); err != nil {
			return err
		}
	}
	return nil
}

func (e *graphExecutor) input(node GraphNode, key string, visiting map[string]bool) (interface{}, error) {
	if connection, ok := e.incoming[node.ID][key]; ok {
		return e.output(connection.Source, connection.SourceOutput, visiting)
	}
	return cloneValue(node.Values[key]), nil
}

func (e *graphExecutor) output(nodeID, key string, visiting map[string]bool) (interface{}, error) {
	visitKey := nodeID + ":" + key
	if visiting[visitKey] {
		return nil, fmt.Errorf("data dependency cycle at %s", visitKey)
	}
	visiting[visitKey] = true
	defer delete(visiting, visitKey)
	if values := e.transient[nodeID]; values != nil {
		if value, ok := values[key]; ok {
			return cloneValue(value), nil
		}
	}
	node := e.nodes[nodeID]
	input := func(key string) (interface{}, error) { return e.input(node, key, visiting) }
	switch node.TypeID {
	case "origin.event.entry-array":
		if key == "objectId" {
			return 0, nil
		}
		if key == "params" {
			return []interface{}{}, nil
		}
	case "origin.event.entry-two-integers":
		return 0, nil
	case "origin.event.timer":
		if key == "timerId" {
			return 0, nil
		}
		if key == "params" {
			return []interface{}{}, nil
		}
	case "origin.variable.get":
		return cloneValue(e.variables[node.Properties.VariableID]), nil
	case "origin.variable.set":
		return input("value")
	case "origin.io.file-path", "origin.io.save-file-path":
		return input("path")
	case "origin.literal.string":
		return input("value")
	case "origin.compare.greater-integer":
		if key == "result" {
			a, err := input("a")
			if err != nil {
				return nil, err
			}
			b, err := input("b")
			return asInt(a) > asInt(b), err
		}
		return input(key)
	case "origin.cast.integer-string":
		value, err := input("value")
		return strconv.Itoa(asInt(value)), err
	case "origin.cast.float-string":
		value, err := input("value")
		return fmt.Sprint(value), err
	case "origin.math.add-integer", "origin.math.subtract-integer", "origin.math.multiply-integer", "origin.math.divide-integer", "origin.math.modulo-integer":
		a, err := input("a")
		if err != nil {
			return nil, err
		}
		b, err := input("b")
		if err != nil {
			return nil, err
		}
		av, bv := asInt(a), asInt(b)
		switch node.TypeID {
		case "origin.math.add-integer":
			return av + bv, nil
		case "origin.math.subtract-integer":
			absolute, _ := input("absolute")
			value := av - bv
			if asBool(absolute) && value < 0 {
				value = -value
			}
			return value, nil
		case "origin.math.multiply-integer":
			return av * bv, nil
		case "origin.math.divide-integer":
			if bv == 0 {
				return nil, errors.New("division by zero")
			}
			roundValue, _ := input("round")
			if asBool(roundValue) {
				return int(math.Round(float64(av) / float64(bv))), nil
			}
			return av / bv, nil
		default:
			if bv == 0 {
				return nil, errors.New("modulo by zero")
			}
			return av % bv, nil
		}
	case "origin.math.random-integer":
		seed, _ := input("seed")
		minValue, _ := input("min")
		maxValue, _ := input("max")
		min, max := asInt(minValue), asInt(maxValue)
		if max < min {
			return nil, errors.New("random maximum is less than minimum")
		}
		source := rand.New(rand.NewSource(int64(asInt(seed))))
		return min + source.Intn(max-min+1), nil
	case "origin.math.add-float", "origin.math.subtract-float", "origin.math.multiply-float", "origin.math.divide-float":
		a, err := input("a")
		if err != nil {
			return nil, err
		}
		b, err := input("b")
		if err != nil {
			return nil, err
		}
		av, bv := asFloat(a), asFloat(b)
		switch node.TypeID {
		case "origin.math.add-float":
			return av + bv, nil
		case "origin.math.subtract-float":
			return av - bv, nil
		case "origin.math.multiply-float":
			return av * bv, nil
		default:
			if bv == 0 {
				return nil, errors.New("division by zero")
			}
			return av / bv, nil
		}
	case "origin.array.length":
		value, err := input("array")
		return len(asSlice(value)), err
	case "origin.array.get-integer", "origin.array.get-string", "origin.array.get-any":
		value, err := input("array")
		if err != nil {
			return nil, err
		}
		indexValue, _ := input("index")
		index := asInt(indexValue)
		items := asSlice(value)
		if index < 0 || index >= len(items) {
			return nil, fmt.Errorf("array index %d out of range", index)
		}
		if node.TypeID == "origin.array.get-integer" {
			return asInt(items[index]), nil
		}
		if node.TypeID == "origin.array.get-any" {
			return cloneValue(items[index]), nil
		}
		return fmt.Sprint(items[index]), nil
	case "origin.array.create-integer", "origin.array.create-integer-new", "origin.array.create-string", "origin.array.create-string-new":
		return input("items")
	case "origin.array.append-integer", "origin.array.append-string":
		items, err := input("array")
		if err != nil {
			return nil, err
		}
		value, err := input("value")
		if err != nil {
			return nil, err
		}
		return append(asSlice(items), cloneValue(value)), nil
	case "origin.dictionary.size":
		value, err := input("dictionary")
		return len(asDictionary(value)), err
	case "origin.dictionary.keys":
		value, err := input("dictionary")
		if err != nil {
			return nil, err
		}
		dictionary := asDictionary(value)
		keys := make([]string, 0, len(dictionary))
		for key := range dictionary {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		return stringsToInterfaces(keys), nil
	case "origin.table.get-column":
		tableValue, err := input("table")
		if err != nil {
			return nil, err
		}
		columnValue, err := input("column")
		if err != nil {
			return nil, err
		}
		table, err := asRuntimeTable(tableValue)
		if err != nil {
			return nil, err
		}
		column := indexOf(table.Columns, fmt.Sprint(columnValue))
		if column < 0 {
			return nil, fmt.Errorf("table column %q does not exist", columnValue)
		}
		values := make([]interface{}, 0, len(table.Rows))
		for _, row := range table.Rows {
			if column < len(row) {
				values = append(values, cloneValue(row[column]))
			} else {
				values = append(values, nil)
			}
		}
		return values, nil
	}
	return nil, fmt.Errorf("node %s has no runtime output %s", node.TypeID, key)
}

func cloneValue(value interface{}) interface{} {
	data, _ := json.Marshal(value)
	var result interface{}
	_ = json.Unmarshal(data, &result)
	return result
}
func asInt(value interface{}) int {
	switch item := value.(type) {
	case int:
		return item
	case int64:
		return int(item)
	case float64:
		return int(item)
	case json.Number:
		value, _ := item.Int64()
		return int(value)
	case string:
		value, _ := strconv.Atoi(item)
		return value
	case bool:
		if item {
			return 1
		}
	}
	return 0
}
func asBool(value interface{}) bool {
	switch item := value.(type) {
	case bool:
		return item
	case float64:
		return item != 0
	case int:
		return item != 0
	case string:
		value, _ := strconv.ParseBool(item)
		return value
	}
	return false
}
func asFloat(value interface{}) float64 {
	switch item := value.(type) {
	case float64:
		return item
	case float32:
		return float64(item)
	case int:
		return float64(item)
	case int64:
		return float64(item)
	case json.Number:
		value, _ := item.Float64()
		return value
	case string:
		value, _ := strconv.ParseFloat(strings.TrimSpace(item), 64)
		return value
	case bool:
		if item {
			return 1
		}
	}
	return 0
}
func asSlice(value interface{}) []interface{} {
	switch items := value.(type) {
	case []interface{}:
		return append([]interface{}(nil), items...)
	case []int:
		result := make([]interface{}, len(items))
		for i, item := range items {
			result[i] = item
		}
		return result
	case []string:
		result := make([]interface{}, len(items))
		for i, item := range items {
			result[i] = item
		}
		return result
	}
	return []interface{}{}
}

func asDictionary(value interface{}) map[string]interface{} {
	if value == nil {
		return map[string]interface{}{}
	}
	if dictionary, ok := cloneValue(value).(map[string]interface{}); ok {
		return dictionary
	}
	return map[string]interface{}{}
}

func asRuntimeTable(value interface{}) (RuntimeTable, error) {
	if table, ok := value.(RuntimeTable); ok {
		return table, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return RuntimeTable{}, err
	}
	var table RuntimeTable
	if err := json.Unmarshal(data, &table); err != nil {
		return RuntimeTable{}, errors.New("value is not a table")
	}
	if table.Columns == nil || table.Rows == nil {
		return RuntimeTable{}, errors.New("value is not a table")
	}
	return table, nil
}

func readCSVTable(path, delimiter string, header bool) (RuntimeTable, error) {
	file, err := os.Open(path)
	if err != nil {
		return RuntimeTable{}, fmt.Errorf("open CSV file: %w", err)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	if delimiter != "" {
		reader.Comma = []rune(delimiter)[0]
	}
	records, err := reader.ReadAll()
	if err != nil {
		return RuntimeTable{}, fmt.Errorf("read CSV file: %w", err)
	}
	table := RuntimeTable{Columns: []string{}, Rows: [][]interface{}{}}
	if len(records) == 0 {
		return table, nil
	}
	start := 0
	if header {
		table.Columns = append(table.Columns, records[0]...)
		start = 1
	} else {
		for index := range records[0] {
			table.Columns = append(table.Columns, fmt.Sprintf("Column %d", index+1))
		}
	}
	for _, record := range records[start:] {
		row := make([]interface{}, len(record))
		for index, item := range record {
			row[index] = item
		}
		table.Rows = append(table.Rows, row)
	}
	return table, nil
}

func writeCSVTable(path string, table RuntimeTable) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create CSV file: %w", err)
	}
	defer file.Close()
	writer := csv.NewWriter(file)
	if len(table.Columns) > 0 {
		if err := writer.Write(table.Columns); err != nil {
			return fmt.Errorf("write CSV header: %w", err)
		}
	}
	for _, row := range table.Rows {
		record := make([]string, len(row))
		for index, item := range row {
			record[index] = fmt.Sprint(item)
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("write CSV row: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("write CSV file: %w", err)
	}
	return nil
}

func mergeTables(leftValue, rightValue interface{}, key string) (RuntimeTable, error) {
	left, err := asRuntimeTable(leftValue)
	if err != nil {
		return RuntimeTable{}, fmt.Errorf("left table: %w", err)
	}
	right, err := asRuntimeTable(rightValue)
	if err != nil {
		return RuntimeTable{}, fmt.Errorf("right table: %w", err)
	}
	leftKey, rightKey := indexOf(left.Columns, key), indexOf(right.Columns, key)
	if leftKey < 0 || rightKey < 0 {
		return RuntimeTable{}, fmt.Errorf("merge key %q is missing from one of the tables", key)
	}
	columns := append([]string(nil), left.Columns...)
	rightColumns := make([]int, 0, len(right.Columns)-1)
	for index, column := range right.Columns {
		if index == rightKey {
			continue
		}
		name := column
		if indexOf(columns, name) >= 0 {
			name += "_right"
		}
		columns = append(columns, name)
		rightColumns = append(rightColumns, index)
	}
	rightRows := map[string][][]interface{}{}
	for _, row := range right.Rows {
		if rightKey < len(row) {
			rightRows[fmt.Sprint(row[rightKey])] = append(rightRows[fmt.Sprint(row[rightKey])], row)
		}
	}
	result := RuntimeTable{Columns: columns, Rows: [][]interface{}{}}
	for _, leftRow := range left.Rows {
		if leftKey >= len(leftRow) {
			continue
		}
		for _, rightRow := range rightRows[fmt.Sprint(leftRow[leftKey])] {
			row := append([]interface{}{}, leftRow...)
			for _, index := range rightColumns {
				if index < len(rightRow) {
					row = append(row, rightRow[index])
				} else {
					row = append(row, nil)
				}
			}
			result.Rows = append(result.Rows, row)
		}
	}
	return result, nil
}

func indexOf(items []string, value string) int {
	for index, item := range items {
		if item == value {
			return index
		}
	}
	return -1
}

func stringsToInterfaces(items []string) []interface{} {
	result := make([]interface{}, len(items))
	for index, item := range items {
		result[index] = item
	}
	return result
}

func interfaceStrings(items []interface{}) []string {
	result := make([]string, len(items))
	for index, item := range items {
		result[index] = fmt.Sprint(item)
	}
	return result
}

func tableRowDictionary(columns []string, row []interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(columns))
	for index, column := range columns {
		if index < len(row) {
			result[column] = cloneValue(row[index])
		} else {
			result[column] = nil
		}
	}
	return result
}

func selectTableColumns(value interface{}, columns []string) (RuntimeTable, error) {
	table, err := asRuntimeTable(value)
	if err != nil {
		return RuntimeTable{}, err
	}
	indexes := make([]int, len(columns))
	for index, column := range columns {
		indexes[index] = indexOf(table.Columns, column)
		if indexes[index] < 0 {
			return RuntimeTable{}, fmt.Errorf("table column %q does not exist", column)
		}
	}
	result := RuntimeTable{Columns: append([]string(nil), columns...), Rows: make([][]interface{}, 0, len(table.Rows))}
	for _, source := range table.Rows {
		row := make([]interface{}, len(indexes))
		for index, sourceIndex := range indexes {
			if sourceIndex < len(source) {
				row[index] = cloneValue(source[sourceIndex])
			}
		}
		result.Rows = append(result.Rows, row)
	}
	return result, nil
}

func dropTableColumns(value interface{}, columns []string) (RuntimeTable, error) {
	table, err := asRuntimeTable(value)
	if err != nil {
		return RuntimeTable{}, err
	}
	dropped := make(map[string]bool, len(columns))
	for _, column := range columns {
		if indexOf(table.Columns, column) < 0 {
			return RuntimeTable{}, fmt.Errorf("table column %q does not exist", column)
		}
		dropped[column] = true
	}
	kept := make([]string, 0, len(table.Columns)-len(columns))
	for _, column := range table.Columns {
		if !dropped[column] {
			kept = append(kept, column)
		}
	}
	return selectTableColumns(table, kept)
}

func renameTableColumn(value interface{}, from, to string) (RuntimeTable, error) {
	table, err := asRuntimeTable(value)
	if err != nil {
		return RuntimeTable{}, err
	}
	index := indexOf(table.Columns, from)
	if index < 0 {
		return RuntimeTable{}, fmt.Errorf("table column %q does not exist", from)
	}
	if strings.TrimSpace(to) == "" {
		return RuntimeTable{}, errors.New("new table column name is empty")
	}
	if existing := indexOf(table.Columns, to); existing >= 0 && existing != index {
		return RuntimeTable{}, fmt.Errorf("table column %q already exists", to)
	}
	table.Columns[index] = to
	return table, nil
}

func filterTableEqual(value interface{}, column string, match interface{}) (RuntimeTable, error) {
	table, err := asRuntimeTable(value)
	if err != nil {
		return RuntimeTable{}, err
	}
	index := indexOf(table.Columns, column)
	if index < 0 {
		return RuntimeTable{}, fmt.Errorf("table column %q does not exist", column)
	}
	result := RuntimeTable{Columns: append([]string(nil), table.Columns...), Rows: [][]interface{}{}}
	for _, row := range table.Rows {
		if index < len(row) && tableValuesEqual(row[index], match) {
			result.Rows = append(result.Rows, append([]interface{}(nil), row...))
		}
	}
	return result, nil
}

func sortTable(value interface{}, column string, ascending bool) (RuntimeTable, error) {
	table, err := asRuntimeTable(value)
	if err != nil {
		return RuntimeTable{}, err
	}
	index := indexOf(table.Columns, column)
	if index < 0 {
		return RuntimeTable{}, fmt.Errorf("table column %q does not exist", column)
	}
	sort.SliceStable(table.Rows, func(left, right int) bool {
		comparison := compareTableValues(tableCell(table.Rows[left], index), tableCell(table.Rows[right], index))
		if ascending {
			return comparison < 0
		}
		return comparison > 0
	})
	return table, nil
}

func fillEmptyTableCells(value, replacement interface{}) (RuntimeTable, error) {
	table, err := asRuntimeTable(value)
	if err != nil {
		return RuntimeTable{}, err
	}
	for rowIndex, row := range table.Rows {
		if len(row) < len(table.Columns) {
			row = append(row, make([]interface{}, len(table.Columns)-len(row))...)
			table.Rows[rowIndex] = row
		}
		for columnIndex, cell := range row {
			if cell == nil || strings.TrimSpace(fmt.Sprint(cell)) == "" {
				row[columnIndex] = cloneValue(replacement)
			}
		}
	}
	return table, nil
}

func tableCell(row []interface{}, index int) interface{} {
	if index >= 0 && index < len(row) {
		return row[index]
	}
	return nil
}

func tableValuesEqual(left, right interface{}) bool {
	leftNumber, leftOK := tableNumber(left)
	rightNumber, rightOK := tableNumber(right)
	if leftOK && rightOK {
		return leftNumber == rightNumber
	}
	return fmt.Sprint(left) == fmt.Sprint(right)
}

func compareTableValues(left, right interface{}) int {
	leftNumber, leftOK := tableNumber(left)
	rightNumber, rightOK := tableNumber(right)
	if leftOK && rightOK {
		if leftNumber < rightNumber {
			return -1
		}
		if leftNumber > rightNumber {
			return 1
		}
		return 0
	}
	return strings.Compare(strings.ToLower(fmt.Sprint(left)), strings.ToLower(fmt.Sprint(right)))
}

func tableNumber(value interface{}) (float64, bool) {
	switch item := value.(type) {
	case int:
		return float64(item), true
	case int64:
		return float64(item), true
	case float64:
		return item, true
	case json.Number:
		number, err := item.Float64()
		return number, err == nil
	case string:
		number, err := strconv.ParseFloat(strings.TrimSpace(item), 64)
		return number, err == nil
	}
	return 0, false
}

func suffixNumber(value string) int {
	number, _ := strconv.Atoi(strings.TrimPrefix(value, "then"))
	return number
}
