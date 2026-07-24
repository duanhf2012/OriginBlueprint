package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	blueprint "github.com/duanhf2012/OriginBlueprint/engine/go/blueprint"
)

type validationExecNode struct {
	name string
}

type validationSourceMap struct {
	graphsDir     string
	workspaceRoot string
	sourcePath    string
	currentPath   string
}

type validationFunctionReference struct {
	functionID   string
	functionName string
}

type validationWorkspaceFunction struct {
	path       string
	relative   string
	data       []byte
	readErr    error
	references []validationFunctionReference
}

func (n *validationExecNode) GetName() string    { return n.name }
func (n *validationExecNode) Exec() (int, error) { return 0, nil }

var validationNodeIDPattern = regexp.MustCompile(`\bnode\s+([^\s:]+)`)

func (a *App) ValidateGraphForWorkspace(content, workspaceRoot, sourcePath string) ([]ValidationIssue, error) {
	var document GraphDocument
	if err := json.Unmarshal([]byte(content), &document); err != nil {
		return []ValidationIssue{{Severity: "error", Code: "document.decode", Message: fmt.Sprintf("decode graph document: %v", err), BlocksSave: true, BlocksRun: true}}, nil
	}
	issues := validateGraph(document)
	if issue := validateGraphWithEngine(content, workspaceRoot, sourcePath); issue != nil {
		issues = append(issues, *issue)
	}
	return issues, nil
}

func validateGraphWithEngine(content, workspaceRoot, sourcePath string) *ValidationIssue {
	temporaryRoot, err := os.MkdirTemp("", "origin-blueprint-validation-")
	if err != nil {
		return engineValidationIssue("engine.prepare", err)
	}
	defer os.RemoveAll(temporaryRoot)

	nodesDir := filepath.Join(temporaryRoot, "nodes")
	graphsDir := filepath.Join(temporaryRoot, "graphs")
	if err := os.MkdirAll(nodesDir, 0755); err != nil {
		return engineValidationIssue("engine.prepare", err)
	}
	if err := os.MkdirAll(graphsDir, 0755); err != nil {
		return engineValidationIssue("engine.prepare", err)
	}

	loadResult := loadRuntimeNodeSchemaDocumentsWithEmbedded(runtimeNodeDirectories())
	if len(loadResult.Errors) != 0 {
		first := loadResult.Errors[0]
		return engineValidationIssue("engine.definition", fmt.Errorf("%s: %s", first.Path, first.Message))
	}
	for index, document := range loadResult.Documents {
		path := filepath.Join(nodesDir, fmt.Sprintf("%05d.json", index))
		if err := os.WriteFile(path, []byte(document.Content), 0644); err != nil {
			return engineValidationIssue("engine.prepare", err)
		}
	}

	currentPath, err := prepareValidationGraphDocuments(graphsDir, workspaceRoot, sourcePath, content)
	if err != nil {
		return engineValidationIssue("engine.prepare", err)
	}

	engine := &blueprint.Blueprint{}
	for _, name := range validationFactoryNames(loadResult.Documents) {
		factoryName := name
		engine.RegisterExecNode(func() blueprint.IExecNode { return &validationExecNode{name: factoryName} })
	}
	if err := engine.Init(nodesDir, graphsDir, nil); err != nil {
		return engineIssueFromError(err, validationSourceMap{
			graphsDir:     graphsDir,
			workspaceRoot: workspaceRoot,
			sourcePath:    sourcePath,
			currentPath:   currentPath,
		})
	}
	if err := engine.Close(); err != nil {
		return engineValidationIssue("engine.compile", err)
	}
	return nil
}

func validationFactoryNames(documents []RuntimeNodeSchemaDocument) []string {
	seen := map[string]bool{}
	result := make([]string, 0)
	for _, document := range documents {
		for _, definition := range parseLegacyRuntimeNodeDefinitions([]byte(document.Content)) {
			name := validationFactoryName(strings.TrimSpace(definition.Name))
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			result = append(result, name)
		}
	}
	return result
}

func validationFactoryName(name string) string {
	index := strings.LastIndex(name, "_")
	if index < 0 || index == len(name)-1 {
		return name
	}
	for _, char := range name[index+1:] {
		if char < '0' || char > '9' {
			return name
		}
	}
	return name[:index]
}

func prepareValidationGraphDocuments(graphsDir, workspaceRoot, sourcePath, content string) (string, error) {
	root := validationAbsolutePath(workspaceRoot)
	source := validationAbsolutePath(sourcePath)
	if root != "" {
		index, err := indexValidationWorkspaceFunctions(root, source)
		if err != nil {
			return "", err
		}
		var document GraphDocument
		if json.Unmarshal([]byte(content), &document) == nil {
			references := validationFunctionReferences(document)
			if functionID := strings.TrimSpace(document.FunctionID); functionID != "" {
				references = append(references, validationFunctionReference{functionID: functionID})
			}
			graphName := strings.TrimSpace(document.GraphName)
			if graphName == "" && source != "" {
				graphName = strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
			}
			if graphName != "" {
				references = append(references, validationFunctionReference{functionName: graphName})
			}
			if err := copyValidationFunctionClosure(graphsDir, index, references); err != nil {
				return "", err
			}
		}
	}

	relative := "__current.obp"
	if source != "" && strings.EqualFold(filepath.Ext(source), ".obpf") {
		relative = "__current.obpf"
	}
	if root != "" && source != "" {
		if candidate, err := filepath.Rel(root, source); err == nil && candidate != "" && !strings.HasPrefix(candidate, "..") {
			extension := strings.ToLower(filepath.Ext(candidate))
			if extension == ".obp" || extension == ".obpf" || extension == ".vgf" {
				relative = candidate
			}
		}
	}
	if relative == "__current.obp" {
		var probe struct {
			FunctionID string `json:"functionId"`
		}
		if json.Unmarshal([]byte(content), &probe) == nil && strings.TrimSpace(probe.FunctionID) != "" {
			relative = "__current.obpf"
		}
	}
	target := filepath.Join(graphsDir, relative)
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(target, []byte(content), 0644); err != nil {
		return "", err
	}
	return target, nil
}

func indexValidationWorkspaceFunctions(root, source string) (map[string][]*validationWorkspaceFunction, error) {
	index := map[string][]*validationWorkspaceFunction{}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return index, nil
	}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".obpf") {
			return nil
		}
		absolute, err := filepath.Abs(path)
		if err != nil || (source != "" && sameValidationPath(absolute, source)) {
			return nil
		}
		relative, err := filepath.Rel(root, absolute)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			return nil
		}
		record := &validationWorkspaceFunction{path: absolute, relative: relative}
		record.data, record.readErr = os.ReadFile(absolute)

		aliases := []string{filepath.ToSlash(relative)}
		if record.readErr == nil {
			var identity struct {
				FunctionID string `json:"functionId"`
				GraphName  string `json:"graphName"`
			}
			if json.Unmarshal(record.data, &identity) == nil {
				if functionID := normalizeValidationFunctionID(identity.FunctionID); functionID != "" {
					aliases = append(aliases, functionID)
				}
				if graphName := strings.TrimSpace(identity.GraphName); graphName != "" {
					aliases = append(aliases, graphName)
				}
			}
			var document GraphDocument
			if json.Unmarshal(record.data, &document) == nil {
				record.references = validationFunctionReferences(document)
			}
		}
		seenAliases := map[string]bool{}
		for _, alias := range aliases {
			if alias == "" || seenAliases[alias] {
				continue
			}
			seenAliases[alias] = true
			index[alias] = append(index[alias], record)
		}
		return nil
	})
	return index, err
}

func copyValidationFunctionClosure(graphsDir string, index map[string][]*validationWorkspaceFunction, queue []validationFunctionReference) error {
	selected := map[string]bool{}
	for len(queue) != 0 {
		reference := queue[0]
		queue = queue[1:]
		owners := index[normalizeValidationFunctionID(reference.functionID)]
		if len(owners) == 0 {
			owners = index[strings.TrimSpace(reference.functionName)]
		}
		for _, function := range owners {
			pathKey := strings.ToLower(filepath.Clean(function.path))
			if selected[pathKey] {
				continue
			}
			selected[pathKey] = true
			if function.readErr != nil {
				return function.readErr
			}
			target := filepath.Join(graphsDir, function.relative)
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if err := os.WriteFile(target, function.data, 0644); err != nil {
				return err
			}
			queue = append(queue, function.references...)
		}
	}
	return nil
}

func validationFunctionReferences(document GraphDocument) []validationFunctionReference {
	references := make([]validationFunctionReference, 0)
	for _, node := range document.Nodes {
		if node.TypeID != "origin.function.call" && node.TypeID != "origin.timer.set-by-function" {
			continue
		}
		functionID := strings.TrimSpace(node.Properties.FunctionID)
		functionName := strings.TrimSpace(node.Properties.FunctionName)
		if functionID == "" && functionName == "" {
			continue
		}
		references = append(references, validationFunctionReference{functionID: functionID, functionName: functionName})
	}
	return references
}

func normalizeValidationFunctionID(value string) string {
	return filepath.ToSlash(strings.TrimSpace(value))
}

func validationAbsolutePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return ""
	}
	return absolute
}

func sameValidationPath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func engineIssueFromError(err error, sources validationSourceMap) *ValidationIssue {
	code := "engine.compile"
	var structured *blueprint.BlueprintError
	if errors.As(err, &structured) && structured != nil && structured.Stage == blueprint.BlueprintStageParse {
		code = "engine.parse"
	}
	if strings.Contains(strings.ToLower(err.Error()), "definition") || strings.Contains(strings.ToLower(err.Error()), "has not been registered") {
		code = "engine.definition"
	}
	issueError := err
	isWorkspaceFunction := false
	if structured != nil && structured.SourcePath != "" {
		isWorkspaceFunction = sources.currentPath != "" && !sameValidationPath(structured.SourcePath, sources.currentPath)
		mapped := *structured
		mapped.SourcePath = sources.originalPath(structured.SourcePath)
		structured = &mapped
		issueError = structured
	}
	issue := engineValidationIssue(code, issueError)
	if structured != nil && structured.NodeID != "" {
		issue.NodeID = structured.NodeID
	} else if match := validationNodeIDPattern.FindStringSubmatch(err.Error()); len(match) == 2 {
		candidate := strings.Trim(match[1], `"`)
		if candidate != "has" && candidate != "definition" {
			issue.NodeID = candidate
		}
	}
	if structured != nil && structured.SourcePath != "" {
		issue.SourcePath = structured.SourcePath
		if isWorkspaceFunction {
			issue.Message = "Workspace function: " + issue.Message
		}
	}
	return issue
}

func engineValidationIssue(code string, err error) *ValidationIssue {
	return &ValidationIssue{Severity: "error", Code: code, Message: err.Error(), BlocksRun: true, Target: "target.go"}
}

func (sources validationSourceMap) originalPath(temporaryPath string) string {
	if temporaryPath == "" {
		return ""
	}
	if sources.currentPath != "" && sameValidationPath(temporaryPath, sources.currentPath) {
		if source := validationAbsolutePath(sources.sourcePath); source != "" {
			return source
		}
	}
	graphsDir := validationAbsolutePath(sources.graphsDir)
	temporary := validationAbsolutePath(temporaryPath)
	if graphsDir != "" && temporary != "" {
		if relative, err := filepath.Rel(graphsDir, temporary); err == nil && relative != "" && relative != "." && !strings.HasPrefix(relative, "..") {
			if root := validationAbsolutePath(sources.workspaceRoot); root != "" {
				return filepath.Join(root, relative)
			}
		}
	}
	return temporaryPath
}
