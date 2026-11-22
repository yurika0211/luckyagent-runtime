package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yurika0211/luckyagent-runtime/config"
	"github.com/yurika0211/luckyagent-runtime/tool"
)

func TestSupportRuntimeFilesystemWriteRootsAreIndependentAndCopied(t *testing.T) {
	root := externalFilesystemFixture(t)
	readRoot, writeRoot, unrelated := filepath.Join(root, "reference"), filepath.Join(root, "workspace"), filepath.Join(root, "unrelated")
	for _, dir := range []string{readRoot, writeRoot, unrelated} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	readPath := filepath.Join(readRoot, "reference.txt")
	if err := os.WriteFile(readPath, []byte("reference evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.Tools.Filesystem.AllowedReadRoots = []string{readRoot}
	cfg.Tools.Filesystem.AllowedWriteRoots = []string{writeRoot}
	support := InitSupportRuntime(cfg, nil, nil)
	// Services own a startup snapshot, not mutable slices from the caller.
	cfg.Tools.Filesystem.AllowedReadRoots[0] = unrelated
	cfg.Tools.Filesystem.AllowedWriteRoots[0] = unrelated
	support.ToolServices.RegisterCoreTools(support.Tools)
	write, ok := support.Tools.Get("file_write")
	if !ok {
		t.Fatal("InitSupportRuntime services did not register file_write")
	}
	path := filepath.Join(writeRoot, "result.txt")
	if _, err := write.Handler(map[string]any{"path": path, "content": "written through runtime services"}); err != nil {
		t.Fatalf("configured write root was not passed to the registered tool: %v", err)
	}
	if body, err := os.ReadFile(path); err != nil || string(body) != "written through runtime services" {
		t.Fatalf("registered file_write result=%q, %v", body, err)
	}
	read, ok := support.Tools.Get("file_read")
	if !ok {
		t.Fatal("file_read is not registered")
	}
	if out, err := read.Handler(map[string]any{"path": readPath}); err != nil || !strings.Contains(out, "reference evidence") {
		t.Fatalf("read-root snapshot was not preserved: %q, %v", out, err)
	}
	for _, deniedPath := range []string{filepath.Join(readRoot, "forbidden.txt"), filepath.Join(unrelated, "not-in-snapshot.txt")} {
		requireRuntimeWriteDenied(t, write, deniedPath)
	}
}

func TestSupportRuntimeEmptyWriteRootsDoNotInheritReadRoots(t *testing.T) {
	root := externalFilesystemFixture(t)
	cfg := config.DefaultConfig()
	cfg.Tools.Filesystem.AllowedReadRoots = []string{root}
	support := InitSupportRuntime(cfg, nil, nil)
	support.ToolServices.RegisterCoreTools(support.Tools)
	write, ok := support.Tools.Get("file_write")
	if !ok {
		t.Fatal("file_write is not registered")
	}
	requireRuntimeWriteDenied(t, write, filepath.Join(root, "read-root-is-not-write-root.txt"))
	// The empty extension list must still preserve the pre-existing temp root.
	tempPath := filepath.Join(t.TempDir(), "builtin-temp.txt")
	if _, err := write.Handler(map[string]any{"path": tempPath, "content": "builtin access"}); err != nil {
		t.Fatalf("empty write roots removed builtin temp access: %v", err)
	}
	if body, err := os.ReadFile(tempPath); err != nil || string(body) != "builtin access" {
		t.Fatalf("builtin temp write=%q, %v", body, err)
	}
}

func requireRuntimeWriteDenied(t *testing.T, write *tool.Tool, path string) {
	t.Helper()
	if _, err := write.Handler(map[string]any{"path": path, "content": "must remain unwritten"}); err == nil || !strings.Contains(err.Error(), "outside sandbox") {
		t.Fatalf("expected an actual filesystem policy denial for %s, got %v", path, err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("denied write left a filesystem effect at %s: %v", path, err)
	}
}

// t.TempDir is normally an allowed write root, so it cannot demonstrate that
// the configured root made a difference. Prefer a temporary directory beside
// the checkout, then under the user's home, and verify the default denial.
func externalFilesystemFixture(t *testing.T) string {
	t.Helper()
	var bases []string
	if cwd, err := os.Getwd(); err == nil {
		bases = append(bases, cwd)
	}
	if home, err := os.UserHomeDir(); err == nil {
		bases = append(bases, home)
	}
	for _, base := range bases {
		root, err := os.MkdirTemp(base, ".runtime-filesystem-test-")
		if err != nil {
			continue
		}
		_, err = tool.FileWriteTool().Handler(map[string]any{"path": filepath.Join(root, "default-policy-probe.txt"), "content": "probe"})
		if err != nil && strings.Contains(err.Error(), "outside sandbox") {
			t.Cleanup(func() { _ = os.RemoveAll(root) })
			return root
		}
		_ = os.RemoveAll(root)
	}
	t.Skip("no writable fixture location outside the default filesystem roots")
	return ""
}
