// Package logger provides a simple logging interface for the application.
package logger

import (
	"io"
	"log/slog"
	"os"

	APIModule "github.com/cpmores/lucinda/api/v1/registry/module"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
)

// ── Logger Structure ──────────────────────────────────────────────────────────

// Logger : Derived from slog.Logger
type Logger struct {
	*slog.Logger
}

type Options struct {
	Level  string // debug | info | warn | error
	Format string // text | json
	Output string // stdout | stderr | file
}

// ── Init ──────────────────────────────────────────────────────────

// New : init Logger
func New(opts Options) (*Logger, error) {
	w, err := resolveWriter(opts.Output)
	if err != nil {
		return nil, err
	}

	var lv slog.Level
	err = lv.UnmarshalText([]byte(opts.Level))
	if err != nil {
		lv = slog.LevelInfo
	}

	var h slog.Handler
	if opts.Format == "json" {
		h = slog.NewJSONHandler(w, &slog.HandlerOptions{Level: lv})
	} else {
		h = slog.NewTextHandler(w, &slog.HandlerOptions{Level: lv})
	}

	return &Logger{
		slog.New(
			h,
		),
	}, nil
}

func resolveWriter(output string) (io.Writer, error) {
	switch output {
	case "stdout", "":
		return io.Writer(os.Stdout), nil
	case "stderr":
		return io.Writer(os.Stderr), nil
	default:
		f, err := os.OpenFile(output, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, err
		}
		return f, nil
	}
}

// ── Children Logger ──────────────────────────────────────────────────────────

// Discard returns a Logger that drops all output. Useful in tests.
func Discard() *Logger {
	return &Logger{slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func (l *Logger) Child(under string) *Logger {
	return l.With("component", under)
}

func (l *Logger) With(args ...any) *Logger {
	return &Logger{
		l.Logger.With(args...),
	}
}

// ── Level Msg ──────────────────────────────────────────────────────────

func (l *Logger) Info(msg string, args ...any) {
	l.Logger.Info(msg, args...)
}

func (l *Logger) Debug(msg string, args ...any) {
	l.Logger.Debug(msg, args...)
}

func (l *Logger) Warn(msg string, args ...any) {
	l.Logger.Warn(msg, args...)
}

func (l *Logger) Error(msg string, args ...any) {
	l.Logger.Error(msg, args...)
}

// ── AvailableModule Interface ──────────────────────────────────────────────────────────

func (l *Logger) GetModuleType() APIModule.ModuleType {
	return APIModule.Logger
}

func (l *Logger) GetModuleID() APIModule.ModuleID {
	return APIModule.NewModuleID(l.GetModuleType(), "default")
}

func (l *Logger) CheckHealth() APIModule.ModuleHealth {
	return APIModule.NewModuleHealth(l.GetModuleID(), l.GetModuleType(), APIModule.Running)
}

func (l *Logger) RegisterWithManager(manager modulemanager.ModuleManager) error {
	return manager.Register(l)
}

func (l *Logger) DependsOn() map[APIModule.ModuleType]string {
	return nil
}

func (l *Logger) DependsEnable() error {
	return nil
}
