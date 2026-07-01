package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"
)

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

func stringsToInterfaces(items []string) []interface{} {
	result := make([]interface{}, len(items))
	for index, item := range items {
		result[index] = item
	}
	return result
}

func suffixNumber(value string) int {
	number, _ := strconv.Atoi(strings.TrimPrefix(value, "then"))
	return number
}
