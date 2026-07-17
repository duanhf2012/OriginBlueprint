package main

import (
	"fmt"
	"sort"
	"strings"
)

type coreGraphEdges struct {
	execAdj     map[string][]string
	dataAdj     map[string][]string
	dataReverse map[string][]string
}

type coreExecConnection struct {
	GraphConnection
	breakCandidate bool
}

func analyzeCoreGraph(document GraphDocument, nodes map[string]GraphNode, ports map[string]portDefinition) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	executable := make(map[string]bool)
	entries := make(map[string]bool)
	for nodeID, definition := range ports {
		hasExecInput := hasPortType(definition.Inputs, "exec")
		hasExecOutput := hasPortType(definition.Outputs, "exec")
		if hasExecInput || hasExecOutput {
			executable[nodeID] = true
		}
		if !hasExecInput && hasExecOutput {
			entries[nodeID] = true
		}
	}

	edges := coreGraphEdges{execAdj: map[string][]string{}, dataAdj: map[string][]string{}, dataReverse: map[string][]string{}}
	execConnections := make([]coreExecConnection, 0, len(document.Connections))
	type endpointGroup struct {
		nodeID  string
		nodeIDs []string
	}
	dataProducers := map[string]*endpointGroup{}
	execTargets := map[string]*endpointGroup{}
	validConnections := make([]GraphConnection, 0, len(document.Connections))
	for _, connection := range document.Connections {
		sourceDefinition, sourceKnown := ports[connection.Source]
		targetDefinition, targetKnown := ports[connection.Target]
		if !sourceKnown || !targetKnown {
			continue
		}
		sourceType := sourceDefinition.Outputs[connection.SourceOutput]
		targetType := targetDefinition.Inputs[connection.TargetInput]
		if sourceType == "" || targetType == "" {
			continue
		}
		validConnections = append(validConnections, connection)
		if sourceType == "exec" && targetType == "exec" {
			execConnections = append(execConnections, coreExecConnection{
				GraphConnection: connection,
				breakCandidate:  nodes[connection.Target].TypeID == "origin.flow.for-loop-break" && connection.TargetInput == "break",
			})
			key := connection.Source + "\x00" + connection.SourceOutput
			group := execTargets[key]
			if group == nil {
				group = &endpointGroup{nodeID: connection.Source}
				execTargets[key] = group
			}
			group.nodeIDs = append(group.nodeIDs, connection.Target)
			continue
		}
		if sourceType != "exec" && targetType != "exec" {
			edges.dataAdj[connection.Source] = append(edges.dataAdj[connection.Source], connection.Target)
			edges.dataReverse[connection.Target] = append(edges.dataReverse[connection.Target], connection.Source)
			key := connection.Target + "\x00" + connection.TargetInput
			group := dataProducers[key]
			if group == nil {
				group = &endpointGroup{nodeID: connection.Target}
				dataProducers[key] = group
			}
			group.nodeIDs = append(group.nodeIDs, connection.Source)
		}
	}
	edges.execAdj = normalizedExecAdjacency(execConnections)
	issues = append(issues, coreCycleIssues("flow.exec-cycle", "执行流形成确定死循环", edges.execAdj, nodes)...)
	issues = append(issues, coreCycleIssues("flow.data-cycle", "数据依赖形成循环", edges.dataAdj, nodes)...)

	for _, group := range dataProducers {
		if len(group.nodeIDs) < 2 {
			continue
		}
		nodeIDs := stableUniqueNodeIDs(append(append([]string(nil), group.nodeIDs...), group.nodeID))
		issues = append(issues, ValidationIssue{Severity: "error", Code: "connection.multiple-producers", Message: "同一数据输入存在多个生产者", NodeID: group.nodeID, NodeIDs: nodeIDs})
	}
	for _, group := range execTargets {
		if len(group.nodeIDs) < 2 {
			continue
		}
		nodeIDs := stableUniqueNodeIDs(append(append([]string(nil), group.nodeIDs...), group.nodeID))
		issues = append(issues, ValidationIssue{Severity: "error", Code: "flow.exec-fanout", Message: "同一执行输出连接到多个目标，请使用 Sequence", NodeID: group.nodeID, NodeIDs: nodeIDs})
	}

	if len(executable) > 0 && len(entries) == 0 {
		issues = append(issues, ValidationIssue{Severity: "warning", Code: "flow.missing-entry", Message: "蓝图存在可执行结点，但没有入口结点", NodeIDs: sortedMapKeys(executable)})
	}

	reachable := make(map[string]bool)
	entryReachable := make(map[string]map[string]bool)
	liveData := make(map[string]bool)
	for entryID := range entries {
		execVisited := map[string]bool{}
		execStack := []string{entryID}
		for len(execStack) > 0 {
			nodeID := execStack[len(execStack)-1]
			execStack = execStack[:len(execStack)-1]
			if entryReachable[nodeID] == nil {
				entryReachable[nodeID] = map[string]bool{}
			}
			entryReachable[nodeID][entryID] = true
			reachable[nodeID] = true
			if execVisited[nodeID] {
				continue
			}
			execVisited[nodeID] = true
			dataVisited := map[string]bool{}
			dataStack := append([]string(nil), edges.dataReverse[nodeID]...)
			for len(dataStack) > 0 {
				dataNodeID := dataStack[len(dataStack)-1]
				dataStack = dataStack[:len(dataStack)-1]
				if entries[dataNodeID] && dataNodeID != entryID {
					continue
				}
				if dataVisited[dataNodeID] {
					continue
				}
				dataVisited[dataNodeID] = true
				liveData[dataNodeID] = true
				reachable[dataNodeID] = true
				if entryReachable[dataNodeID] == nil {
					entryReachable[dataNodeID] = map[string]bool{}
				}
				entryReachable[dataNodeID][entryID] = true
				dataStack = append(dataStack, edges.dataReverse[dataNodeID]...)
			}
			execStack = append(execStack, edges.execAdj[nodeID]...)
		}
	}

	if len(entries) > 0 {
		for nodeID := range executable {
			if reachable[nodeID] {
				continue
			}
			label := nodes[nodeID].Properties.Label
			if label == "" {
				label = nodeID
			}
			issues = append(issues, ValidationIssue{Severity: "error", Code: "flow.unreachable-node", Message: "结点不可达，不可能从任何入口执行到：" + label, NodeID: nodeID})
		}
	}
	for nodeID, definition := range ports {
		if hasPortType(definition.Inputs, "exec") || hasPortType(definition.Outputs, "exec") || liveData[nodeID] {
			continue
		}
		issues = append(issues, ValidationIssue{Severity: "warning", Code: "flow.unused-data-node", Message: "纯数据结点未被任何可达执行路径使用", NodeID: nodeID})
	}
	for _, connection := range validConnections {
		sourceType := ports[connection.Source].Outputs[connection.SourceOutput]
		targetType := ports[connection.Target].Inputs[connection.TargetInput]
		if sourceType == "exec" || targetType == "exec" {
			continue
		}
		if !entrySetsOverlap(entryReachable[connection.Source], entryReachable[connection.Target]) {
			issues = append(issues, ValidationIssue{Severity: "error", Code: "flow.cross-entry-data", Message: "不同入口分支之间存在参数交叉连接", NodeID: connection.Target})
		}
	}
	return markAndSortCoreIssues(issues)
}

func markAndSortCoreIssues(issues []ValidationIssue) []ValidationIssue {
	for index := range issues {
		if issues[index].Severity == "error" && issues[index].Target == "" && coreIssueBlocksSave(issues[index].Code) {
			issues[index].BlocksSave = true
		}
	}
	sort.SliceStable(issues, func(left, right int) bool {
		if issues[left].Code != issues[right].Code {
			return issues[left].Code < issues[right].Code
		}
		leftID := issues[left].NodeID
		if leftID == "" && len(issues[left].NodeIDs) > 0 {
			leftID = issues[left].NodeIDs[0]
		}
		rightID := issues[right].NodeID
		if rightID == "" && len(issues[right].NodeIDs) > 0 {
			rightID = issues[right].NodeIDs[0]
		}
		return strings.Compare(leftID, rightID) < 0
	})
	return issues
}

func stableUniqueNodeIDs(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func coreCycleMessage(kind string, ids []string) string {
	return fmt.Sprintf("%s：%s", kind, strings.Join(ids, ", "))
}

func normalizedExecAdjacency(connections []coreExecConnection) map[string][]string {
	base := make(map[string][]string)
	indegree := make(map[string]int)
	for _, connection := range connections {
		if connection.breakCandidate {
			continue
		}
		base[connection.Source] = append(base[connection.Source], connection.Target)
		indegree[connection.Target]++
		if _, exists := indegree[connection.Source]; !exists {
			indegree[connection.Source] = 0
		}
	}
	roots := make([]string, 0)
	for nodeID, degree := range indegree {
		if degree == 0 {
			roots = append(roots, nodeID)
		}
	}
	sort.Strings(roots)

	type loopReachability struct {
		bodyStarts  []string
		withoutBody map[string][]string
	}
	loopCache := make(map[string]loopReachability)
	result := cloneCoreAdjacency(base)
	for _, connection := range connections {
		if !connection.breakCandidate {
			continue
		}
		loopID := connection.Target
		analysis, cached := loopCache[loopID]
		if !cached {
			analysis.withoutBody = make(map[string][]string)
			for _, edge := range connections {
				if edge.breakCandidate {
					continue
				}
				if edge.Source == loopID && edge.SourceOutput == "body" {
					analysis.bodyStarts = append(analysis.bodyStarts, edge.Target)
					continue
				}
				analysis.withoutBody[edge.Source] = append(analysis.withoutBody[edge.Source], edge.Target)
			}
			loopCache[loopID] = analysis
		}
		valid := connection.Source == loopID && connection.SourceOutput == "body"
		if !valid {
			valid = coreReachable(analysis.bodyStarts, connection.Source, base) &&
				!coreReachable(roots, connection.Source, analysis.withoutBody)
		}
		if !valid {
			result[connection.Source] = append(result[connection.Source], connection.Target)
		}
	}
	return result
}

func coreReachable(starts []string, target string, adjacency map[string][]string) bool {
	if target == "" {
		return false
	}
	visited := make(map[string]bool)
	stack := append([]string(nil), starts...)
	for len(stack) > 0 {
		nodeID := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if visited[nodeID] {
			continue
		}
		visited[nodeID] = true
		if nodeID == target {
			return true
		}
		stack = append(stack, adjacency[nodeID]...)
	}
	return false
}

func cloneCoreAdjacency(source map[string][]string) map[string][]string {
	result := make(map[string][]string, len(source))
	for nodeID, targets := range source {
		result[nodeID] = append([]string(nil), targets...)
	}
	return result
}

func coreCycleIssues(code, label string, adjacency map[string][]string, nodes map[string]GraphNode) []ValidationIssue {
	components := stronglyConnectedCoreComponents(adjacency)
	issues := make([]ValidationIssue, 0)
	for _, component := range components {
		if !coreComponentIsCycle(component, adjacency) {
			continue
		}
		issueCode := code
		severity := "error"
		messageLabel := label
		if coreComponentHasOpaqueNode(component, nodes) {
			issueCode = "flow.possible-cycle"
			severity = "warning"
			messageLabel = "不透明结点参与的连线可能形成循环"
		}
		issues = append(issues, ValidationIssue{
			Severity:  severity,
			Code:      issueCode,
			Message:   coreCycleMessage(messageLabel, component),
			NodeID:    component[0],
			NodeIDs:   component,
			BlocksRun: severity == "error",
		})
	}
	return issues
}

func coreComponentHasOpaqueNode(component []string, nodes map[string]GraphNode) bool {
	for _, nodeID := range component {
		node := nodes[nodeID]
		if node.TypeID == "origin.legacy.placeholder" {
			return true
		}
		_, staticallyKnown := graphNodePorts[node.TypeID]
		if !staticallyKnown && (len(node.Properties.LegacyInputs) > 0 || len(node.Properties.LegacyOutputs) > 0) {
			return true
		}
	}
	return false
}

func coreComponentIsCycle(component []string, adjacency map[string][]string) bool {
	if len(component) > 1 {
		return true
	}
	if len(component) == 0 {
		return false
	}
	for _, target := range adjacency[component[0]] {
		if target == component[0] {
			return true
		}
	}
	return false
}

// stronglyConnectedCoreComponents uses iterative Kosaraju passes so deeply nested
// user graphs cannot exhaust the Go call stack.
func stronglyConnectedCoreComponents(adjacency map[string][]string) [][]string {
	vertices := make(map[string]bool)
	reverse := make(map[string][]string)
	for source, targets := range adjacency {
		vertices[source] = true
		for _, target := range targets {
			vertices[target] = true
			reverse[target] = append(reverse[target], source)
		}
	}
	orderedVertices := sortedMapKeys(vertices)
	type frame struct {
		nodeID string
		next   int
	}
	visited := make(map[string]bool, len(vertices))
	finishOrder := make([]string, 0, len(vertices))
	for _, start := range orderedVertices {
		if visited[start] {
			continue
		}
		visited[start] = true
		stack := []frame{{nodeID: start}}
		for len(stack) > 0 {
			current := &stack[len(stack)-1]
			targets := adjacency[current.nodeID]
			if current.next >= len(targets) {
				finishOrder = append(finishOrder, current.nodeID)
				stack = stack[:len(stack)-1]
				continue
			}
			next := targets[current.next]
			current.next++
			if visited[next] {
				continue
			}
			visited[next] = true
			stack = append(stack, frame{nodeID: next})
		}
	}

	assigned := make(map[string]bool, len(vertices))
	components := make([][]string, 0)
	for index := len(finishOrder) - 1; index >= 0; index-- {
		start := finishOrder[index]
		if assigned[start] {
			continue
		}
		component := make([]string, 0)
		stack := []string{start}
		assigned[start] = true
		for len(stack) > 0 {
			nodeID := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			component = append(component, nodeID)
			for _, next := range reverse[nodeID] {
				if assigned[next] {
					continue
				}
				assigned[next] = true
				stack = append(stack, next)
			}
		}
		sort.Strings(component)
		components = append(components, component)
	}
	sort.Slice(components, func(left, right int) bool {
		return components[left][0] < components[right][0]
	})
	return components
}
