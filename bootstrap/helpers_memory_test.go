package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yurika0211/luckyagent-runtime/config"
)

func TestResolveMemoryRuntimeOffSkipsAestusHome(t *testing.T) {
	home := t.TempDir()
	mgr, err := config.NewManagerWithDir(home)
	if err != nil {
		t.Fatal(err)
	}
	cfg := mgr.Get()
	cfg.Memory.Backend = "off"
	rt, err := ResolveMemoryRuntime(mgr, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rt.Store.Source().Kind != "disabled" {
		t.Fatalf("source %+v", rt.Store.Source())
	}
	if _, err := os.Stat(filepath.Join(home, "memory")); !os.IsNotExist(err) {
		t.Fatalf("aestus dir should be absent: %v", err)
	}
}

func TestResolveMemoryRuntimeRejectsUnknownBackend(t *testing.T) {
	home := t.TempDir()
	mgr, err := config.NewManagerWithDir(home)
	if err != nil {
		t.Fatal(err)
	}
	cfg := mgr.Get()
	cfg.Memory.Backend = "sqlite"
	if _, err := ResolveMemoryRuntime(mgr, cfg, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveMemoryRuntimeEmptyBackendOpensAestus(t *testing.T) {
	home := t.TempDir()
	mgr, err := config.NewManagerWithDir(home)
	if err != nil {
		t.Fatal(err)
	}
	cfg := mgr.Get()
	cfg.Memory.Backend = ""
	rt, err := ResolveMemoryRuntime(mgr, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Store.Close()
	if rt.Store.Source().Kind == "disabled" {
		t.Fatal("empty backend should stay aestus")
	}
	if _, err := os.Stat(filepath.Join(home, "memory")); err != nil {
		t.Fatalf("expected aestus dir: %v", err)
	}
}
