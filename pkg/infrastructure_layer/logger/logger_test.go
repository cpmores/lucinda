package logger

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	APIModule "github.com/cpmores/lucinda/api/v1/registry/module"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
)

// Child attaches the canonical "component" attribute; verify it lands in output.
func TestChildAddsComponent(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{slog.New(slog.NewTextHandler(&buf, nil))}

	l.Child("transport").Info("started")

	out := buf.String()
	if !strings.Contains(out, "component=transport") {
		t.Fatalf("expected component=transport in output, got: %s", out)
	}
}

// Discard must never panic and must not fail when derived further.
func TestDiscardIsSafe(t *testing.T) {
	l := Discard()
	l.Child("monitor").Warn("dropped", "topic", "x")
	l.Error("boom", "err", "something")
}

// The logger implements AvailableModule and can register with ModuleManager.
func TestRegisterWithManager(t *testing.T) {
	mm := modulemanager.NewModuleManager()
	l := Discard()

	if err := l.RegisterWithManager(mm); err != nil {
		t.Fatalf("RegisterWithManager: %v", err)
	}

	mods := mm.GetByType(APIModule.Logger)
	if len(mods) != 1 {
		t.Fatalf("expected 1 logger module, got %d", len(mods))
	}

	if l.GetModuleType() != APIModule.Logger {
		t.Fatalf("expected ModuleType %s, got %s", APIModule.Logger, l.GetModuleType())
	}

	health, err := mm.Health(l.GetModuleID())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if health.Status != APIModule.Running {
		t.Fatalf("expected Running health, got %s", health.Status)
	}
}

// New resolves the output to a file and writes structured JSON there.
func TestNewWritesToFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")
	l, err := New(Options{Level: "info", Format: "json", Output: path})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	l.Info("hello", "key", "value")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Contains(data, []byte(`"msg":"hello"`)) {
		t.Fatalf("expected msg=hello in file, got: %s", data)
	}
}

// An invalid level string must not panic and must fall back gracefully.
func TestNewInvalidLevelFallsBack(t *testing.T) {
	l, err := New(Options{Level: "bogus", Format: "text", Output: filepath.Join(t.TempDir(), "x.log")})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	l.Info("should not panic")
}

// The colored format emits ANSI escapes for terminal (stdout) output.
func TestNewColoredTerminalHasAnsi(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	l, err := New(Options{Level: "info", Format: "colored", Output: "stdout"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	l.Info("hello")

	w.Close()
	data, _ := io.ReadAll(r)
	if !bytes.Contains(data, []byte("\x1b[")) {
		t.Fatalf("expected ANSI escape in terminal colored output, got: %q", data)
	}
}

// The colored format stays plain when writing to a file, so ANSI escape
// codes never pollute log files.
func TestNewColoredFileIsPlain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "colored.log")
	l, err := New(Options{Level: "info", Format: "colored", Output: path})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	l.Info("hello")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if bytes.Contains(data, []byte("\x1b[")) {
		t.Fatalf("expected no ANSI escape in file output, got: %q", data)
	}
}
