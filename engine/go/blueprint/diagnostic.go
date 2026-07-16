package blueprint

import (
	"errors"
	"fmt"
	"strings"
)

type BlueprintStage string

const (
	BlueprintStageParse   BlueprintStage = "parse"
	BlueprintStageCompile BlueprintStage = "compile"
	BlueprintStageExecute BlueprintStage = "execute"
	BlueprintStageResume  BlueprintStage = "resume"
)

// BlueprintError 为解析、编译和执行错误补充稳定的定位字段，并保留 errors.Is/As 链。
type BlueprintError struct {
	Stage       BlueprintStage
	SourcePath  string
	GraphName   string
	GraphID     int64
	EntranceID  int64
	ExecutionID uint64
	NodeID      string
	NodeName    string
	PC          PC
	Cause       error
}

func (e *BlueprintError) Error() string {
	if e == nil {
		return "blueprint error"
	}
	parts := make([]string, 0, 8)
	if e.Stage != "" {
		parts = append(parts, "stage="+string(e.Stage))
	}
	if e.SourcePath != "" {
		parts = append(parts, "source="+e.SourcePath)
	}
	if e.GraphName != "" {
		parts = append(parts, "graph="+e.GraphName)
	}
	if e.GraphID != 0 {
		parts = append(parts, fmt.Sprintf("graphID=%d", e.GraphID))
	}
	if e.EntranceID != 0 {
		parts = append(parts, fmt.Sprintf("entranceID=%d", e.EntranceID))
	}
	if e.ExecutionID != 0 {
		parts = append(parts, fmt.Sprintf("executionID=%d", e.ExecutionID))
	}
	if e.NodeID != "" {
		parts = append(parts, "node="+e.NodeID)
	}
	if e.PC != InvalidPC {
		parts = append(parts, fmt.Sprintf("pc=%d", e.PC))
	}
	prefix := "blueprint error"
	if len(parts) != 0 {
		prefix += " [" + strings.Join(parts, " ") + "]"
	}
	if e.Cause == nil {
		return prefix
	}
	return prefix + ": " + e.Cause.Error()
}

func (e *BlueprintError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// BlueprintDiagnosticSink 接收异步或同步执行的终态失败；未配置时没有回调开销。
type BlueprintDiagnosticSink interface {
	ReportBlueprintError(BlueprintError)
}

func wrapBlueprintStageError(stage BlueprintStage, sourcePath string, cause error) error {
	if cause == nil {
		return nil
	}
	var existing *BlueprintError
	if errors.As(cause, &existing) && existing != nil {
		result := *existing
		if result.Stage == "" {
			result.Stage = stage
		}
		if result.SourcePath == "" {
			result.SourcePath = sourcePath
		}
		return &result
	}
	return &BlueprintError{Stage: stage, SourcePath: sourcePath, PC: InvalidPC, Cause: cause}
}

func (b *Blueprint) getDiagnosticSink() BlueprintDiagnosticSink {
	if b == nil {
		return nil
	}
	b.mu.RLock()
	sink := b.diagnosticSink
	b.mu.RUnlock()
	return sink
}

func newBlueprintNodeError(stage BlueprintStage, graph *Graph, node *ExecNode, pc PC, cause error) *BlueprintError {
	result := &BlueprintError{Stage: stage, PC: pc, Cause: cause}
	if graph != nil {
		result.GraphName = graph.name
		result.GraphID = graph.graphID
	}
	if node != nil {
		result.NodeID = node.ID
		if node.Definition != nil {
			result.NodeName = node.Definition.Name
		}
	}
	return result
}

func enrichExecutionError(execution *Execution, cause error) error {
	if cause == nil || execution == nil {
		return cause
	}
	var result BlueprintError
	var existing *BlueprintError
	if errors.As(cause, &existing) && existing != nil {
		result = *existing
	} else {
		result = BlueprintError{Stage: BlueprintStageExecute, PC: InvalidPC, Cause: cause}
	}
	if result.Stage == "" {
		result.Stage = BlueprintStageExecute
	}
	if result.GraphName == "" && execution.instance != nil {
		result.GraphName = execution.instance.name
	}
	if result.GraphID == 0 {
		result.GraphID = execution.graphID
	}
	if result.EntranceID == 0 {
		result.EntranceID = execution.entranceID
	}
	if result.ExecutionID == 0 {
		result.ExecutionID = execution.id
	}
	return &result
}

func reportBlueprintDiagnostic(sink BlueprintDiagnosticSink, value BlueprintError) {
	if sink == nil {
		return
	}
	defer func() { _ = recover() }()
	sink.ReportBlueprintError(value)
}
