package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx       context.Context
	sessions  map[string]context.CancelFunc
	sessionMu sync.Mutex
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

func NewApp() *App { return &App{sessions: make(map[string]context.CancelFunc)} }

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

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
		path, err = runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Open Graph", Filters: graphFilters()})
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
			Title: "Save Graph", DefaultFilename: "Untitled.obp",
			Filters: []runtime.FileFilter{{DisplayName: "Origin Blueprint (*.obp)", Pattern: "*.obp"}},
		})
		if err != nil || path == "" {
			return "", err
		}
	}
	if filepath.Ext(path) == "" {
		path += ".obp"
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}
	a.recordRecent(path)
	return path, nil
}

func (a *App) ChooseWorkspace() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Select Workspace"})
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
	err := os.Remove(configPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (a *App) Quit() {
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
		ext := strings.ToLower(filepath.Ext(item.Name()))
		if !item.IsDir() && ext != ".obp" && ext != ".vgf" && ext != ".json" {
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
	dir, _ := os.UserConfigDir()
	return filepath.Join(dir, "OriginBlueprint", "recent.json")
}

func (a *App) GetRecentFiles() []string {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return []string{}
	}
	var result []string
	if json.Unmarshal(data, &result) != nil {
		return []string{}
	}
	filtered := result[:0]
	for _, path := range result {
		if _, err := os.Stat(path); err == nil {
			filtered = append(filtered, path)
		}
	}
	return filtered
}

func (a *App) recordRecent(path string) {
	items := a.GetRecentFiles()
	result := []string{path}
	for _, item := range items {
		if item != path && len(result) < 10 {
			result = append(result, item)
		}
	}
	config := configPath()
	_ = os.MkdirAll(filepath.Dir(config), 0755)
	data, _ := json.MarshalIndent(result, "", "  ")
	_ = os.WriteFile(config, data, 0644)
}
