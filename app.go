package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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
	ctx        context.Context
	closeMu    sync.Mutex
	allowClose bool
}

type FileResult struct {
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
	RecentFiles        []string `json:"recentFiles"`
	LastGraphDirectory string   `json:"lastGraphDirectory"`
}

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
	a.recordRecent(path)
	return FileResult{Path: path, Content: string(data)}, nil
}

func (a *App) SaveGraph(path, content string) (string, error) {
	var err error
	if path == "" {
		path, err = runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
			Title: "Save Graph", DefaultDirectory: a.lastGraphDirectory(), DefaultFilename: "Untitled.vgf",
			Filters: graphFilters(),
		})
		if err != nil || path == "" {
			return "", err
		}
	}
	if filepath.Ext(path) == "" {
		path += ".vgf"
	}
	data, err := graphContentForPath(path, content)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	a.recordRecent(path)
	return path, nil
}

func graphContentForPath(path, content string) ([]byte, error) {
	if exportsLegacyGraph(filepath.Ext(path)) {
		var document GraphDocument
		if err := json.Unmarshal([]byte(content), &document); err == nil && document.SchemaVersion == GraphSchemaVersion {
			return exportLegacyGraph(document)
		}
	}
	return []byte(content), nil
}

func exportsLegacyGraph(ext string) bool {
	return strings.EqualFold(ext, ".vgf") || strings.EqualFold(ext, ".obp")
}

func (a *App) ChooseWorkspace() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Select Workspace"})
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
		if !item.IsDir() && ext != ".obp" && ext != ".vgf" {
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
		if ext != ".vgf" && ext != ".obp" {
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
			if node.TypeID == typeID {
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

func (a *App) ExportPNG(dataURL string) (string, error) {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title: "Export Graph Image", DefaultFilename: "OriginBlueprint.png",
		Filters: []runtime.FileFilter{{DisplayName: "PNG Image (*.png)", Pattern: "*.png"}},
	})
	if err != nil || path == "" {
		return "", err
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
	return path, os.WriteFile(path, data, 0644)
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

func writeAppConfig(config appConfig) {
	path := configPath()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	data, _ := json.MarshalIndent(config, "", "  ")
	_ = os.WriteFile(path, data, 0644)
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

func (a *App) recordRecent(path string) {
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
	writeAppConfig(config)
}
