package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func TestFilesystemWriteRootsDefaultAndLegacyLoad(t *testing.T) {
	if got := DefaultConfig().Tools.Filesystem.AllowedWriteRoots; len(got) != 0 {
		t.Fatalf("default write roots=%v, want no extra roots", got)
	}
	for _, raw := range []string{
		`{}`,
		`{"tools":{"filesystem":{"allowed_read_roots":["/reference"]}}}`,
		`{"tools":{"filesystem":{"allowed_write_roots":[]}}}`,
	} {
		t.Run(raw, func(t *testing.T) {
			cfg, err := parseConfigData([]byte(raw))
			if err != nil {
				t.Fatal(err)
			}
			if got := cfg.Tools.Filesystem.AllowedWriteRoots; len(got) != 0 {
				t.Fatalf("legacy/empty write roots=%v; read roots must not grant write access", got)
			}
		})
	}
}

func TestFilesystemWriteRootsLoadSaveAndSet(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "config.json")
	const raw = `{"tools":{"filesystem":{"allowed_read_roots":["/reference"],"allowed_write_roots":["/workspace", "G:\\Projects"]}}}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	mgr, err := NewManagerWithDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Load(); err != nil {
		t.Fatal(err)
	}
	if got := mgr.Get().Tools.Filesystem.AllowedWriteRoots; !reflect.DeepEqual(got, []string{"/workspace", `G:\Projects`}) {
		t.Fatalf("loaded write roots=%v", got)
	}
	if err := mgr.Set("tools.filesystem.allowed_write_roots", " /project-a, /project-b , ,"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Save(); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Load(); err != nil {
		t.Fatal(err)
	}
	got := mgr.Get().Tools.Filesystem
	if !reflect.DeepEqual(got.AllowedWriteRoots, []string{"/project-a", "/project-b"}) ||
		!reflect.DeepEqual(got.AllowedReadRoots, []string{"/reference"}) {
		t.Fatalf("read/write roots changed or mixed during persistence: %+v", got)
	}
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var persisted struct {
		Tools struct {
			Filesystem struct {
				Write []string `json:"allowed_write_roots"`
			} `json:"filesystem"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(stored, &persisted); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(persisted.Tools.Filesystem.Write, got.AllowedWriteRoots) {
		t.Fatalf("canonical allowed_write_roots key did not persist: %s", stored)
	}
	if err := mgr.Set("tools.filesystem.allowed_write_roots", ""); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Save(); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Load(); err != nil {
		t.Fatal(err)
	}
	if got := mgr.Get().Tools.Filesystem; len(got.AllowedWriteRoots) != 0 || len(got.AllowedReadRoots) != 1 {
		t.Fatalf("clearing write roots affected read roots: %+v", got)
	}
}

func TestFilesystemWriteRootsSnapshotsAreIndependentAndRequireRestart(t *testing.T) {
	mgr, err := NewManagerWithDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	original := DefaultConfig()
	original.Tools.Filesystem.AllowedReadRoots = []string{"/reference"}
	original.Tools.Filesystem.AllowedWriteRoots = []string{"/workspace"}
	mgr.SetConfig(original)
	original.Tools.Filesystem.AllowedWriteRoots[0] = "/mutated-original"
	first := mgr.Get()
	first.Tools.Filesystem.AllowedWriteRoots[0] = "/mutated-snapshot"
	first.Tools.Filesystem.AllowedReadRoots[0] = "/mutated-reference"
	second := mgr.Get()
	if second.Tools.Filesystem.AllowedWriteRoots[0] != "/workspace" || second.Tools.Filesystem.AllowedReadRoots[0] != "/reference" {
		t.Fatalf("filesystem roots share mutable backing slices: %+v", second.Tools.Filesystem)
	}
	copy := Clone(second)
	copy.Tools.Filesystem.AllowedWriteRoots[0] = "/another-workspace"
	if second.Tools.Filesystem.AllowedWriteRoots[0] != "/workspace" {
		t.Fatal("Clone shares the write roots backing slice")
	}
	hot, restart := ReloadClassification(second, copy)
	if slices.Contains(hot, "request_runtime") || !slices.Contains(restart, "runtime_services") {
		t.Fatalf("write-root updates must rebuild services: hot=%v restart=%v", hot, restart)
	}
}
