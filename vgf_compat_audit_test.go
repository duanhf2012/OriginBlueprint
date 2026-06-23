package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type vgfAuditFile struct {
	Path      string
	RelPath   string
	NodeCount int
	EdgeCount int
	VarCount  int
	Score     int
}

type vgfAuditIssue struct {
	File    string `json:"file"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

type vgfAuditReport struct {
	Root         string          `json:"root"`
	TotalFiles   int             `json:"totalFiles"`
	SampledFiles int             `json:"sampledFiles"`
	SampleRatio  float64         `json:"sampleRatio"`
	Issues       []vgfAuditIssue `json:"issues"`
	Warnings     []vgfAuditIssue `json:"warnings"`
	Files        []vgfAuditFile  `json:"files"`
}

type strictLegacyRuntimeGraph struct {
	GraphName string                    `json:"graph_name"`
	Time      string                    `json:"time"`
	Nodes     []strictLegacyRuntimeNode `json:"nodes"`
	Edges     []strictLegacyRuntimeEdge `json:"edges"`
	Variables []map[string]interface{} `json:"variables"`
}

type strictLegacyRuntimeNode struct {
	ID           string                 `json:"id"`
	Class        string                 `json:"class"`
	Module       string                 `json:"module"`
	PortDefaults map[string]interface{} `json:"port_defaultv"`
}

type strictLegacyRuntimeEdge struct {
	EdgeID       string `json:"edge_id"`
	SourceNodeID string `json:"source_node_id"`
	TargetNodeID string `json:"des_node_id"`
	SourcePortID int    `json:"source_port_id"`
	TargetPortID int    `json:"des_port_id"`
}

func TestVGFCompatibilityAudit(t *testing.T) {
	if os.Getenv("ORIGIN_BLUEPRINT_COMPAT_AUDIT") != "1" {
		t.Skip("set ORIGIN_BLUEPRINT_COMPAT_AUDIT=1 to run the legacy VGF compatibility audit")
	}

	root := filepath.Join("build", "bin", "vgf")
	files, err := collectVGFAuditFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no .vgf files found under %s", root)
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].Score != files[j].Score {
			return files[i].Score > files[j].Score
		}
		return strings.ToLower(files[i].RelPath) < strings.ToLower(files[j].RelPath)
	})

	sampleCount := int(math.Ceil(float64(len(files)) * 0.8))
	sampled := files[:sampleCount]
	report := vgfAuditReport{
		Root:         root,
		TotalFiles:   len(files),
		SampledFiles: len(sampled),
		SampleRatio:  0.8,
		Files:        sampled,
	}

	outputRoot := filepath.Join(os.TempDir(), "origin-blueprint-vgf-compat-audit")
	if err := os.RemoveAll(outputRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outputRoot, 0755); err != nil {
		t.Fatal(err)
	}

	for _, file := range sampled {
		issues, warnings := auditVGFRoundTrip(file, root, outputRoot)
		report.Issues = append(report.Issues, issues...)
		report.Warnings = append(report.Warnings, warnings...)
	}

	reportPath := filepath.Join(outputRoot, "report.json")
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reportPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	t.Logf("audited %d/%d vgf files (80%% sample, complex files first)", len(sampled), len(files))
	t.Logf("report: %s", reportPath)
	for i, file := range sampled {
		if i >= 10 {
			break
		}
		t.Logf("top[%02d] nodes=%d edges=%d vars=%d %s", i+1, file.NodeCount, file.EdgeCount, file.VarCount, file.RelPath)
	}
	if len(report.Issues) > 0 {
		for _, issue := range report.Issues {
			t.Logf("%s: %s: %s", issue.File, issue.Kind, issue.Message)
		}
		t.Fatalf("vgf compatibility audit found %d issue(s)", len(report.Issues))
	}
	for _, warning := range report.Warnings {
		t.Logf("warning %s: %s: %s", warning.File, warning.Kind, warning.Message)
	}
}

func collectVGFAuditFiles(root string) ([]vgfAuditFile, error) {
	var files []vgfAuditFile
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".vgf") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var graph legacyGraph
		if err := json.Unmarshal(data, &graph); err != nil {
			return fmt.Errorf("%s: decode legacy graph: %w", path, err)
		}
		rel, _ := filepath.Rel(root, path)
		nodeCount := len(graph.Nodes)
		edgeCount := len(graph.Edges)
		varCount := len(graph.Variables)
		files = append(files, vgfAuditFile{
			Path:      path,
			RelPath:   filepath.ToSlash(rel),
			NodeCount: nodeCount,
			EdgeCount: edgeCount,
			VarCount:  varCount,
			Score:     nodeCount*100 + edgeCount*10 + varCount,
		})
		return nil
	})
	return files, err
}

func auditVGFRoundTrip(file vgfAuditFile, inputRoot, outputRoot string) ([]vgfAuditIssue, []vgfAuditIssue) {
	var issues []vgfAuditIssue
	var warnings []vgfAuditIssue
	addIssue := func(kind, format string, args ...interface{}) {
		issues = append(issues, vgfAuditIssue{File: file.RelPath, Kind: kind, Message: fmt.Sprintf(format, args...)})
	}
	addWarning := func(kind, format string, args ...interface{}) {
		warnings = append(warnings, vgfAuditIssue{File: file.RelPath, Kind: kind, Message: fmt.Sprintf(format, args...)})
	}

	originalData, err := os.ReadFile(file.Path)
	if err != nil {
		addIssue("read-original", "%v", err)
		return issues, warnings
	}
	var original legacyGraph
	if err := json.Unmarshal(originalData, &original); err != nil {
		addIssue("decode-original", "%v", err)
		return issues, warnings
	}

	document, err := migrateLegacyGraph(originalData)
	if err != nil {
		addIssue("migrate", "%v", err)
		return issues, warnings
	}
	for _, validationIssue := range validateGraph(document) {
		if validationIssue.Severity == "error" {
			addWarning("validate-migrated", "%s: %s", validationIssue.Code, validationIssue.Message)
		}
	}

	exportedData, err := exportLegacyGraph(document)
	if err != nil {
		addIssue("export", "%v", err)
		return issues, warnings
	}
	if !json.Valid(exportedData) {
		addIssue("export-json", "exported vgf is not valid JSON")
		return issues, warnings
	}

	outputPath := filepath.Join(outputRoot, file.RelPath)
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		addIssue("write-export", "%v", err)
		return issues, warnings
	}
	if err := os.WriteFile(outputPath, exportedData, 0644); err != nil {
		addIssue("write-export", "%v", err)
		return issues, warnings
	}

	var exported legacyGraph
	if err := json.Unmarshal(exportedData, &exported); err != nil {
		addIssue("decode-export", "%v", err)
		return issues, warnings
	}
	var strict strictLegacyRuntimeGraph
	if err := json.Unmarshal(exportedData, &strict); err != nil {
		addIssue("decode-export-runtime-shape", "old runtime strict edge parser would reject exported vgf: %v", err)
		return issues, warnings
	}

	compareLegacyGraphs(original, exported, addIssue)
	return issues, warnings
}

func compareLegacyGraphs(original, exported legacyGraph, addIssue func(string, string, ...interface{})) {
	if original.GraphName != exported.GraphName {
		addIssue("graph-name", "graph_name changed from %q to %q", original.GraphName, exported.GraphName)
	}
	if original.Time != exported.Time {
		addIssue("time", "time changed from %q to %q", original.Time, exported.Time)
	}
	if !normalizedJSONEqual(original.Variables, exported.Variables) {
		addIssue("variables", "variables changed")
	}
	if !reflect.DeepEqual(original.Groups, exported.Groups) {
		addIssue("groups", "groups changed")
	}

	originalNodes := map[string]legacyNode{}
	for _, node := range original.Nodes {
		originalNodes[node.ID] = node
	}
	exportedNodes := map[string]legacyNode{}
	for _, node := range exported.Nodes {
		exportedNodes[node.ID] = node
	}
	if len(originalNodes) != len(exportedNodes) {
		addIssue("nodes-count", "node count changed from %d to %d", len(originalNodes), len(exportedNodes))
	}
	for id, originalNode := range originalNodes {
		exportedNode, ok := exportedNodes[id]
		if !ok {
			addIssue("node-missing", "node %s missing after round-trip", id)
			continue
		}
		if originalNode.Class != exportedNode.Class {
			addIssue("node-class", "node %s class changed from %q to %q", id, originalNode.Class, exportedNode.Class)
		}
		if originalNode.Module != exportedNode.Module {
			addIssue("node-module", "node %s module changed from %q to %q", id, originalNode.Module, exportedNode.Module)
		}
		if !floatSlicesEqual(originalNode.Position, exportedNode.Position) {
			addIssue("node-position", "node %s position changed from %v to %v", id, originalNode.Position, exportedNode.Position)
		}
		if !normalizedJSONEqual(normalizedDefaults(originalNode.PortDefaults), normalizedDefaults(exportedNode.PortDefaults)) {
			addIssue("node-defaults", "node %s port_defaultv changed", id)
		}
	}

	originalEdges := legacyEdgeSignatures(original.Edges)
	exportedEdges := legacyEdgeSignatures(exported.Edges)
	if !reflect.DeepEqual(originalEdges, exportedEdges) {
		addIssue("edges", "edge endpoints/port ids changed: original=%d exported=%d", len(originalEdges), len(exportedEdges))
	}
}

func legacyEdgeSignatures(edges []legacyEdge) []string {
	result := make([]string, 0, len(edges))
	for _, edge := range edges {
		sourcePort := legacyPortIndex(edge.SourcePortID, edge.SourceIndex)
		targetPort := legacyPortIndex(edge.TargetPortID, edge.TargetIndex)
		result = append(result, strings.Join([]string{
			edge.SourceNodeID,
			strconv.Itoa(sourcePort),
			edge.TargetNodeID,
			strconv.Itoa(targetPort),
		}, "->"))
	}
	sort.Strings(result)
	return result
}

func normalizedJSONEqual(left, right interface{}) bool {
	leftData, leftErr := json.Marshal(left)
	rightData, rightErr := json.Marshal(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	var leftValue interface{}
	var rightValue interface{}
	if err := json.Unmarshal(leftData, &leftValue); err != nil {
		return false
	}
	if err := json.Unmarshal(rightData, &rightValue); err != nil {
		return false
	}
	leftNormalized, _ := json.Marshal(leftValue)
	rightNormalized, _ := json.Marshal(rightValue)
	return bytes.Equal(leftNormalized, rightNormalized)
}

func normalizedDefaults(values map[string]interface{}) map[string]interface{} {
	if len(values) == 0 {
		return map[string]interface{}{}
	}
	return values
}

func floatSlicesEqual(left, right []float64) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if math.Abs(left[i]-right[i]) > 0.000001 {
			return false
		}
	}
	return true
}
