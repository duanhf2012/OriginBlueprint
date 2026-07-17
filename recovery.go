package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	recoverySnapshotSchemaVersion = 1
	recoverySnapshotsPerKey       = 5
	recoverySnapshotMaxAge        = 30 * 24 * time.Hour
)

type RecoverySnapshotResult struct {
	Path       string `json:"path"`
	SourcePath string `json:"sourcePath,omitempty"`
	TabID      string `json:"tabId,omitempty"`
	CreatedAt  string `json:"createdAt"`
}

type recoverySnapshot struct {
	SchemaVersion  int               `json:"schemaVersion"`
	SourcePath     string            `json:"sourcePath,omitempty"`
	TabID          string            `json:"tabId,omitempty"`
	CreatedAt      string            `json:"createdAt"`
	Document       json.RawMessage   `json:"document"`
	BlockingIssues []ValidationIssue `json:"blockingIssues"`
}

func (a *App) SaveRecoverySnapshot(sourcePath, tabID, documentJSON, issuesJSON string) (RecoverySnapshotResult, error) {
	key, normalizedSource, err := recoverySnapshotKey(sourcePath, tabID)
	if err != nil {
		return RecoverySnapshotResult{}, err
	}
	document := json.RawMessage(documentJSON)
	if !json.Valid(document) {
		return RecoverySnapshotResult{}, errors.New("recovery document is not valid JSON")
	}
	issues := make([]ValidationIssue, 0)
	if strings.TrimSpace(issuesJSON) != "" {
		if err := json.Unmarshal([]byte(issuesJSON), &issues); err != nil {
			return RecoverySnapshotResult{}, fmt.Errorf("decode recovery issues: %w", err)
		}
	}
	createdAt := time.Now().UTC()
	snapshot := recoverySnapshot{
		SchemaVersion:  recoverySnapshotSchemaVersion,
		SourcePath:     normalizedSource,
		TabID:          strings.TrimSpace(tabID),
		CreatedAt:      createdAt.Format(time.RFC3339Nano),
		Document:       document,
		BlockingIssues: issues,
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return RecoverySnapshotResult{}, err
	}
	directory := filepath.Join(recoveryRoot(), key)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return RecoverySnapshotResult{}, err
	}
	directory, err = containedRecoveryPath(directory, true)
	if err != nil {
		return RecoverySnapshotResult{}, err
	}
	path := filepath.Join(directory, createdAt.Format("20060102T150405.000000000Z")+".json")
	if err := a.writeAtomically(path, data, 0600); err != nil {
		return RecoverySnapshotResult{}, fmt.Errorf("write recovery snapshot: %w", err)
	}
	if err := pruneRecoveryDirectory(directory, createdAt); err != nil {
		return RecoverySnapshotResult{}, err
	}
	return RecoverySnapshotResult{Path: path, SourcePath: normalizedSource, TabID: snapshot.TabID, CreatedAt: snapshot.CreatedAt}, nil
}

func (a *App) ListRecoverySnapshots() ([]RecoverySnapshotResult, error) {
	root := recoveryRoot()
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return []RecoverySnapshotResult{}, nil
	}
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	result := make([]RecoverySnapshotResult, 0)
	for _, keyEntry := range entries {
		if !keyEntry.IsDir() || keyEntry.Type()&os.ModeSymlink != 0 {
			continue
		}
		directory := filepath.Join(root, keyEntry.Name())
		if err := pruneRecoveryDirectory(directory, now); err != nil {
			return nil, err
		}
		files, err := os.ReadDir(directory)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, file := range files {
			if file.IsDir() || file.Type()&os.ModeSymlink != 0 || !strings.EqualFold(filepath.Ext(file.Name()), ".json") {
				continue
			}
			path := filepath.Join(directory, file.Name())
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil, readErr
			}
			var snapshot recoverySnapshot
			if json.Unmarshal(data, &snapshot) != nil || snapshot.SchemaVersion != recoverySnapshotSchemaVersion {
				continue
			}
			result = append(result, RecoverySnapshotResult{
				Path: path, SourcePath: snapshot.SourcePath, TabID: snapshot.TabID, CreatedAt: snapshot.CreatedAt,
			})
		}
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].CreatedAt != result[right].CreatedAt {
			return result[left].CreatedAt > result[right].CreatedAt
		}
		return result[left].Path > result[right].Path
	})
	return result, nil
}

func (a *App) ReadRecoverySnapshot(path string) (string, error) {
	safePath, err := recoverySnapshotPath(path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(safePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (a *App) DeleteRecoverySnapshot(path string) error {
	safePath, err := recoverySnapshotPath(path)
	if err != nil {
		return err
	}
	return os.Remove(safePath)
}

func (a *App) DeleteRecoverySnapshots(sourcePath, tabID string) error {
	key, _, err := recoverySnapshotKey(sourcePath, tabID)
	if err != nil {
		return err
	}
	directory := filepath.Join(recoveryRoot(), key)
	safeDirectory, err := containedRecoveryPath(directory, false)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(safeDirectory); err != nil {
		return err
	}
	return nil
}

func recoveryRoot() string {
	return filepath.Join(filepath.Dir(configPath()), "recovery")
}

func recoverySnapshotKey(sourcePath, tabID string) (string, string, error) {
	normalizedSource := ""
	material := ""
	if strings.TrimSpace(sourcePath) != "" {
		absolute, err := filepath.Abs(strings.TrimSpace(sourcePath))
		if err != nil {
			return "", "", err
		}
		normalizedSource = filepath.Clean(absolute)
		material = normalizedSource
		if runtime.GOOS == "windows" {
			material = strings.ToLower(material)
		}
		material = "source:" + material
	} else {
		tabID = strings.TrimSpace(tabID)
		if tabID == "" {
			return "", "", errors.New("recovery snapshot requires a source path or tab id")
		}
		material = "tab:" + tabID
	}
	digest := sha256.Sum256([]byte(material))
	return hex.EncodeToString(digest[:]), normalizedSource, nil
}

func recoverySnapshotPath(path string) (string, error) {
	if !strings.EqualFold(filepath.Ext(strings.TrimSpace(path)), ".json") {
		return "", errors.New("recovery snapshot path must name a JSON file")
	}
	return containedRecoveryPath(path, true)
}

func containedRecoveryPath(path string, resolveExisting bool) (string, error) {
	root, err := filepath.Abs(recoveryRoot())
	if err != nil {
		return "", err
	}
	candidate, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return "", err
	}
	if !pathWithinRoot(root, candidate) || sameValidationPath(root, candidate) {
		return "", errors.New("recovery path is outside the recovery directory")
	}
	if resolveExisting {
		resolvedCandidate, evalErr := filepath.EvalSymlinks(candidate)
		if evalErr != nil {
			return "", evalErr
		}
		resolvedRoot := root
		if value, rootErr := filepath.EvalSymlinks(root); rootErr == nil {
			resolvedRoot = value
		}
		if !pathWithinRoot(resolvedRoot, resolvedCandidate) {
			return "", errors.New("recovery path resolves outside the recovery directory")
		}
		candidate = resolvedCandidate
	}
	return candidate, nil
}

func pathWithinRoot(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

func pruneRecoveryDirectory(directory string, now time.Time) error {
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	type recoveryFile struct {
		path    string
		modTime time.Time
	}
	files := make([]recoveryFile, 0)
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		path := filepath.Join(directory, entry.Name())
		if now.Sub(info.ModTime()) > recoverySnapshotMaxAge {
			if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
				return removeErr
			}
			continue
		}
		files = append(files, recoveryFile{path: path, modTime: info.ModTime()})
	}
	sort.Slice(files, func(left, right int) bool {
		if !files[left].modTime.Equal(files[right].modTime) {
			return files[left].modTime.After(files[right].modTime)
		}
		return files[left].path > files[right].path
	})
	if len(files) <= recoverySnapshotsPerKey {
		return nil
	}
	for _, file := range files[recoverySnapshotsPerKey:] {
		if err := os.Remove(file.path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
