package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newRecoveryTestApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("ORIGIN_BLUEPRINT_CONFIG_PATH", filepath.Join(t.TempDir(), "config", "config.json"))
	return NewApp()
}

func TestRecoverySnapshotsRetainNewestFiveAtomically(t *testing.T) {
	app := newRecoveryTestApp(t)
	var first RecoverySnapshotResult
	for index := 0; index < 7; index++ {
		got, err := app.SaveRecoverySnapshot("graph.obp", fmt.Sprintf("tab-%d", index), fmt.Sprintf(`{"version":%d}`, index), `[]`)
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			first = got
		} else if filepath.Dir(got.Path) != filepath.Dir(first.Path) {
			t.Fatalf("same source produced different recovery keys: %q and %q", first.Path, got.Path)
		}
	}
	got, err := app.ListRecoverySnapshots()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("snapshots = %d, want 5: %#v", len(got), got)
	}
}

func TestRecoverySnapshotWriteFailureLeavesNoPartialFile(t *testing.T) {
	app := newRecoveryTestApp(t)
	app.atomicWrite = func(string, []byte, os.FileMode) error { return errors.New("injected write failure") }
	if _, err := app.SaveRecoverySnapshot("graph.obp", "tab", `{}`, `[]`); err == nil {
		t.Fatal("SaveRecoverySnapshot succeeded, want injected failure")
	}
	app.atomicWrite = nil
	got, err := app.ListRecoverySnapshots()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("snapshots = %#v, want none after failed atomic write", got)
	}
}

func TestRecoverySnapshotsExpireAfterThirtyDays(t *testing.T) {
	app := newRecoveryTestApp(t)
	snapshot, err := app.SaveRecoverySnapshot("graph.obp", "tab", `{}`, `[]`)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-31 * 24 * time.Hour)
	if err := os.Chtimes(snapshot.Path, old, old); err != nil {
		t.Fatal(err)
	}
	got, err := app.ListRecoverySnapshots()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expired snapshots = %#v, want none", got)
	}
	if _, err := os.Stat(snapshot.Path); !os.IsNotExist(err) {
		t.Fatalf("expired snapshot still exists: %v", err)
	}
}

func TestRecoverySnapshotRefusesPathsOutsideRecoveryRoot(t *testing.T) {
	app := newRecoveryTestApp(t)
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"secret":true}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ReadRecoverySnapshot(outside); err == nil {
		t.Fatal("ReadRecoverySnapshot accepted an outside path")
	}
	if err := app.DeleteRecoverySnapshot(outside); err == nil {
		t.Fatal("DeleteRecoverySnapshot accepted an outside path")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside file changed: %v", err)
	}
}

func TestRecoverySnapshotRefusesSymlinkedKeyDirectory(t *testing.T) {
	app := newRecoveryTestApp(t)
	key, _, err := recoverySnapshotKey("graph.obp", "tab")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(recoveryRoot(), 0700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(recoveryRoot(), key)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := app.SaveRecoverySnapshot("graph.obp", "tab", `{}`, `[]`); err == nil {
		t.Fatal("SaveRecoverySnapshot accepted a key directory resolving outside recovery root")
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("outside directory was modified: %#v", entries)
	}
}

func TestDeleteRecoverySnapshotsUsesSourceOrTabKey(t *testing.T) {
	app := newRecoveryTestApp(t)
	if _, err := app.SaveRecoverySnapshot("graph.obp", "first-tab", `{}`, `[]`); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SaveRecoverySnapshot("", "unsaved-tab", `{}`, `[]`); err != nil {
		t.Fatal(err)
	}
	if err := app.DeleteRecoverySnapshots("graph.obp", "different-tab"); err != nil {
		t.Fatal(err)
	}
	got, err := app.ListRecoverySnapshots()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].TabID != "unsaved-tab" {
		t.Fatalf("snapshots after source cleanup = %#v", got)
	}
	if err := app.DeleteRecoverySnapshots("", "unsaved-tab"); err != nil {
		t.Fatal(err)
	}
	got, err = app.ListRecoverySnapshots()
	if err != nil || len(got) != 0 {
		t.Fatalf("snapshots after tab cleanup = %#v, err = %v", got, err)
	}
}
