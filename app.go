package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	stdRuntime "runtime"
	"sort"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx         context.Context
	closeMu     sync.Mutex
	allowClose  bool
	atomicWrite func(path string, data []byte, mode os.FileMode) error
}

type FileResult struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type ProjectSettingsResult struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type WorkspaceEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

type NodeReferenceResult struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Count int    `json:"count"`
}

type appConfig struct {
	RecentFiles         []string `json:"recentFiles"`
	LastGraphDirectory  string   `json:"lastGraphDirectory"`
	LastExportDirectory string   `json:"lastExportDirectory"`
}

const projectSettingsFileName = "originblueprint.project"
const functionReferenceQueryPrefix = "function:"

const defaultProjectSettingsContent = `{
  "version": 1,
  "appearance": {
    "locale": "zh-CN",
    "uiScale": "normal",
    "nodeScale": "normal"
  },
  "layout": {
    "panels": {
      "files": 210,
      "tools": 210,
      "library": 230,
      "variables": 300,
      "test": 155,
      "references": 180
    },
    "visible": {
      "tools": true,
      "library": true,
      "test": false
    }
  },
  "explorer": {
    "expanded": [],
    "selected": "",
    "revealActiveFile": true,
    "hideBuildFolders": false
  },
  "editor": {
    "autoSave": "off",
    "validateBeforeSave": false
  },
  "export": {
    "imageScale": 2,
    "showGrid": true
  }
}`

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

func (a *App) beforeClose(ctx context.Context) bool {
	a.closeMu.Lock()
	allow := a.allowClose
	a.closeMu.Unlock()
	if allow {
		return false
	}
	runtime.EventsEmit(ctx, "origin:before-close")
	return true
}

func graphFilters() []runtime.FileFilter {
	return []runtime.FileFilter{
		{DisplayName: "Origin Blueprint (*.obp)", Pattern: "*.obp"},
		{DisplayName: "Origin Blueprint Function (*.obpf)", Pattern: "*.obpf"},
		{DisplayName: "Legacy Visual Graph (*.vgf)", Pattern: "*.vgf"},
		{DisplayName: "JSON (*.json)", Pattern: "*.json"},
	}
}

func (a *App) OpenGraph(path string) (FileResult, error) {
	var err error
	if path == "" {
		path, err = runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Open Graph", DefaultDirectory: a.lastGraphDirectory(), Filters: graphFilters()})
		if err != nil || path == "" {
			return FileResult{}, err
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return FileResult{}, err
	}
	if err := a.recordRecent(path); err != nil {
		a.reportNonFatalError("record recent graph", err)
	}
	return FileResult{Path: path, Content: string(data)}, nil
}

func (a *App) SaveGraph(path, content string) (string, error) {
	var err error
	var contentDocument GraphDocument
	requiresNative := json.Unmarshal([]byte(content), &contentDocument) == nil &&
		contentDocument.SchemaVersion == GraphSchemaVersion &&
		graphDocumentRequiresNativePersistence(contentDocument)
	if path == "" {
		defaultFilename := "Untitled.vgf"
		if requiresNative {
			defaultFilename = "Untitled.obp"
		}
		path, err = runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
			Title: "Save Graph", DefaultDirectory: a.lastGraphDirectory(), DefaultFilename: defaultFilename,
			Filters: graphFilters(),
		})
		if err != nil || path == "" {
			return "", err
		}
	}
	if filepath.Ext(path) == "" {
		if requiresNative {
			path += ".obp"
		} else {
			path += ".vgf"
		}
	}
	data, err := graphContentForPath(path, content)
	if err != nil {
		return "", err
	}
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", err
		}
	}
	if err := a.writeAtomically(path, data, 0644); err != nil {
		return "", err
	}
	if err := a.recordRecent(path); err != nil {
		a.reportNonFatalError("record recent graph", err)
	}
	return path, nil
}

// ForceSaveGraph intentionally replaces an existing graph after first preserving
// its exact bytes in a sibling .bak file. The frontend only exposes this through
// the compatibility-loss flow with a second destructive confirmation.
func (a *App) ForceSaveGraph(path, content string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("force save requires an existing graph path")
	}
	data, err := graphContentForPath(path, content)
	if err != nil {
		return "", err
	}
	original, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read graph before force save: %w", err)
	}
	mode := os.FileMode(0644)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}
	if err := a.writeAtomically(path+".bak", original, mode); err != nil {
		return "", fmt.Errorf("create graph backup: %w", err)
	}
	if err := a.writeAtomically(path, data, mode); err != nil {
		return "", fmt.Errorf("replace graph after backup: %w", err)
	}
	if err := a.recordRecent(path); err != nil {
		a.reportNonFatalError("record recent graph", err)
	}
	return path, nil
}

func (a *App) writeAtomically(path string, data []byte, mode os.FileMode) error {
	if a != nil && a.atomicWrite != nil {
		return a.atomicWrite(path, data, mode)
	}
	return writeFileAtomically(path, data, mode)
}

func (a *App) reportNonFatalError(operation string, err error) {
	if err == nil {
		return
	}
	if a != nil && a.ctx != nil {
		runtime.LogErrorf(a.ctx, "%s: %v", operation, err)
		return
	}
	fmt.Fprintf(os.Stderr, "OriginBlueprint %s: %v\n", operation, err)
}

func writeFileAtomically(path string, data []byte, mode os.FileMode) (err error) {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	temporary, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(mode); err != nil {
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return atomicReplaceFile(temporaryPath, path)
}

func graphContentForPath(path, content string) ([]byte, error) {
	if exportsLegacyGraph(filepath.Ext(path)) {
		var document GraphDocument
		if err := json.Unmarshal([]byte(content), &document); err == nil && document.SchemaVersion == GraphSchemaVersion {
			if graphDocumentRequiresNativePersistence(document) {
				if strings.EqualFold(filepath.Ext(path), ".vgf") {
					return nil, errors.New("this graph uses native-only nodes and must be saved as .obp or .obpf")
				}
				return []byte(content), nil
			}
			return exportLegacyGraph(document)
		}
	}
	return []byte(content), nil
}

func graphDocumentRequiresNativePersistence(document GraphDocument) bool {
	if len(document.FunctionSignature.Inputs) > 0 || len(document.FunctionSignature.Outputs) > 0 {
		return true
	}
	for _, node := range document.Nodes {
		if strings.HasPrefix(node.TypeID, "origin.function.") ||
			strings.HasPrefix(node.TypeID, "origin.timer.") ||
			node.TypeID == "origin.flow.delay" {
			return true
		}
	}
	for _, variable := range document.Variables {
		if strings.EqualFold(variable.Type, "timerhandle") {
			return true
		}
	}
	return false
}

func exportsLegacyGraph(ext string) bool {
	return strings.EqualFold(ext, ".vgf") || strings.EqualFold(ext, ".obp")
}

func (a *App) ChooseWorkspace() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Select Workspace"})
}

func (a *App) OpenExternalURL(url string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return nil
	}
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return errors.New("unsupported URL")
	}
	runtime.BrowserOpenURL(a.ctx, url)
	return nil
}

func projectSettingsPath(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("workspace path is empty")
	}
	return filepath.Join(root, projectSettingsFileName), nil
}

func (a *App) LoadProjectSettings(root string) (ProjectSettingsResult, error) {
	path, err := projectSettingsPath(root)
	if err != nil {
		return ProjectSettingsResult{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := a.writeAtomically(path, []byte(defaultProjectSettingsContent), 0644); err != nil {
			return ProjectSettingsResult{}, err
		}
		return ProjectSettingsResult{Path: path, Content: defaultProjectSettingsContent}, nil
	}
	if err != nil {
		return ProjectSettingsResult{}, err
	}
	return ProjectSettingsResult{Path: path, Content: string(data)}, nil
}

func (a *App) SaveProjectSettings(root, content string) (string, error) {
	path, err := projectSettingsPath(root)
	if err != nil {
		return "", err
	}
	if err := a.writeAtomically(path, []byte(content), 0644); err != nil {
		return "", err
	}
	return path, nil
}

func (a *App) CurrentWorkingDirectory() (string, error) {
	root, err := os.Getwd()
	if err == nil && root != "" {
		return root, nil
	}
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(executable), nil
}

func (a *App) ChooseDataFile(mode string) (string, error) {
	filters := []runtime.FileFilter{
		{DisplayName: "CSV and text files", Pattern: "*.csv;*.tsv;*.txt;*.json"},
		{DisplayName: "All files", Pattern: "*.*"},
	}
	if mode == "save" {
		return runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "Select Output File", Filters: filters})
	}
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Select Input File", Filters: filters})
}

func (a *App) NewWindow() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return exec.Command(executable).Start()
}

func (a *App) ClearRecentFiles() error {
	for _, path := range []string{configPath(), legacyRecentPath()} {
		err := os.Remove(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (a *App) Quit() {
	a.closeMu.Lock()
	a.allowClose = true
	a.closeMu.Unlock()
	runtime.Quit(a.ctx)
}

func (a *App) ListWorkspace(root string) ([]WorkspaceEntry, error) {
	if root == "" {
		return []WorkspaceEntry{}, nil
	}
	items, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	result := make([]WorkspaceEntry, 0, len(items))
	for _, item := range items {
		if item.IsDir() && isIgnoredWorkspaceDirectory(item.Name()) {
			continue
		}
		ext := strings.ToLower(filepath.Ext(item.Name()))
		if !item.IsDir() && ext != ".obp" && ext != ".vgf" && ext != ".obpf" {
			continue
		}
		result = append(result, WorkspaceEntry{Name: item.Name(), Path: filepath.Join(root, item.Name()), IsDir: item.IsDir()})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].IsDir != result[j].IsDir {
			return result[i].IsDir
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result, nil
}

func (a *App) FindNodeReferences(root, typeID string) ([]NodeReferenceResult, error) {
	if root == "" || strings.TrimSpace(typeID) == "" {
		return []NodeReferenceResult{}, nil
	}
	query, isFunctionReferenceQuery := functionReferenceQuery(typeID)
	results := make([]NodeReferenceResult, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if path != root && isIgnoredWorkspaceDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".vgf" && ext != ".obp" && !(isFunctionReferenceQuery && ext == ".obpf") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		document, err := graphDocumentForReferenceSearch(data)
		if err != nil {
			return nil
		}
		count := 0
		for _, node := range document.Nodes {
			if nodeMatchesReferenceQuery(node, query, isFunctionReferenceQuery) {
				count++
			}
		}
		if count > 0 {
			results = append(results, NodeReferenceResult{Name: entry.Name(), Path: path, Count: count})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(results, func(i, j int) bool {
		return strings.ToLower(results[i].Path) < strings.ToLower(results[j].Path)
	})
	return results, nil
}

func functionReferenceQuery(value string) (string, bool) {
	query := strings.TrimSpace(value)
	if strings.HasPrefix(query, functionReferenceQueryPrefix) {
		return strings.TrimSpace(strings.TrimPrefix(query, functionReferenceQueryPrefix)), true
	}
	return query, false
}

func nodeMatchesReferenceQuery(node GraphNode, query string, isFunctionReferenceQuery bool) bool {
	if !isFunctionReferenceQuery {
		return node.TypeID == query
	}
	if (node.TypeID != "origin.function.call" && node.TypeID != "origin.timer.set-by-function") || query == "" {
		return false
	}
	return node.Properties.FunctionID == query || node.Properties.FunctionName == query
}

func (a *App) RevealInFolder(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("file path is empty")
	}
	if _, err := os.Stat(path); err != nil {
		return err
	}
	switch stdRuntime.GOOS {
	case "windows":
		return exec.Command("explorer.exe", "/select,"+path).Start()
	case "darwin":
		return exec.Command("open", "-R", path).Start()
	default:
		return exec.Command("xdg-open", filepath.Dir(path)).Start()
	}
}

func graphDocumentForReferenceSearch(data []byte) (GraphDocument, error) {
	var document GraphDocument
	if err := json.Unmarshal(data, &document); err == nil && document.SchemaVersion == GraphSchemaVersion {
		return document, nil
	}
	return migrateLegacyGraph(data)
}

func isIgnoredWorkspaceDirectory(name string) bool {
	switch strings.ToLower(name) {
	case ".git", ".gocache", "node_modules":
		return true
	default:
		return false
	}
}

func (a *App) ChooseExportPNGPath(defaultDirectory string) (string, error) {
	defaultDirectory = a.exportDefaultDirectory(defaultDirectory)
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title: "Export Graph Image", DefaultFilename: "OriginBlueprint.png",
		DefaultDirectory: defaultDirectory,
		Filters:          []runtime.FileFilter{{DisplayName: "PNG Image (*.png)", Pattern: "*.png"}},
	})
	if err != nil || path == "" {
		return "", err
	}
	if filepath.Ext(path) == "" {
		path += ".png"
	}
	return path, nil
}

func (a *App) SavePNG(path string, dataURL string) (string, error) {
	if path == "" {
		return "", nil
	}
	if filepath.Ext(path) == "" {
		path += ".png"
	}
	comma := strings.IndexByte(dataURL, ',')
	if comma < 0 {
		return "", errors.New("invalid PNG data")
	}
	data, err := base64.StdEncoding.DecodeString(dataURL[comma+1:])
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	if err := a.recordExportDirectory(path); err != nil {
		a.reportNonFatalError("record export directory", err)
	}
	return path, nil
}

func (a *App) ExportPNG(dataURL string) (string, error) {
	path, err := a.ChooseExportPNGPath("")
	if err != nil || path == "" {
		return "", err
	}
	return a.SavePNG(path, dataURL)
}

func configPath() string {
	if override := os.Getenv("ORIGIN_BLUEPRINT_CONFIG_PATH"); override != "" {
		return override
	}
	dir, _ := os.UserConfigDir()
	return filepath.Join(dir, "OriginBlueprint", "config.json")
}

func legacyRecentPath() string {
	dir, _ := os.UserConfigDir()
	return filepath.Join(dir, "OriginBlueprint", "recent.json")
}

func readAppConfig() appConfig {
	data, err := os.ReadFile(configPath())
	if err != nil {
		if legacyData, legacyErr := os.ReadFile(legacyRecentPath()); legacyErr == nil {
			var legacyRecent []string
			if json.Unmarshal(legacyData, &legacyRecent) == nil {
				return appConfig{RecentFiles: legacyRecent}
			}
		}
		return appConfig{}
	}
	var config appConfig
	if json.Unmarshal(data, &config) == nil {
		return config
	}
	var legacyRecent []string
	if json.Unmarshal(data, &legacyRecent) == nil {
		return appConfig{RecentFiles: legacyRecent}
	}
	return appConfig{}
}

func writeAppConfig(config appConfig) error {
	path := configPath()
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomically(path, data, 0644)
}

func (a *App) GetRecentFiles() []string {
	config := readAppConfig()
	filtered := config.RecentFiles[:0]
	for _, path := range config.RecentFiles {
		if _, err := os.Stat(path); err == nil {
			filtered = append(filtered, path)
		}
	}
	return filtered
}

func (a *App) lastGraphDirectory() string {
	path := readAppConfig().LastGraphDirectory
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return path
	}
	return ""
}

func validDirectory(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return path
	}
	return ""
}

func (a *App) exportDefaultDirectory(fallback string) string {
	if path := validDirectory(readAppConfig().LastExportDirectory); path != "" {
		return path
	}
	return validDirectory(fallback)
}

func (a *App) recordRecent(path string) error {
	config := readAppConfig()
	items := a.GetRecentFiles()
	result := []string{path}
	for _, item := range items {
		if item != path && len(result) < 10 {
			result = append(result, item)
		}
	}
	config.RecentFiles = result
	if dir := filepath.Dir(path); dir != "." {
		config.LastGraphDirectory = dir
	}
	return writeAppConfig(config)
}

func (a *App) recordExportDirectory(path string) error {
	dir := filepath.Dir(path)
	if dir == "." {
		return nil
	}
	config := readAppConfig()
	config.LastExportDirectory = dir
	return writeAppConfig(config)
}
