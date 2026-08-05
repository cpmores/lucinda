// Package logger provides a simple logging interface for the application.
package logger

import (
	"io"
	"log/slog"
	"os"

	"github.com/lmittmann/tint"

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
	Format string // text | json | colored (colored is for terminals only; files should use text/json)
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
	switch opts.Format {
	case "json":
		h = slog.NewJSONHandler(w, &slog.HandlerOptions{Level: lv})
	case "colored":
		// Color only for terminal output; file output stays plain so
		// ANSI escape codes never pollute log files.
		noColor := opts.Output != "" && opts.Output != "stdout" && opts.Output != "stderr"
		h = tint.NewTextHandler(w, &tint.Options{
			Level:      lv,
			TimeFormat: "15:04:05.000",
			NoColor:    noColor,
			// tint's defaults are bright colors (92/93/91) which some
			// terminals remap; use basic ANSI colors for reliable rendering.
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				if a.Key == slog.LevelKey && len(groups) == 0 {
					if level, ok := a.Value.Any().(slog.Level); ok {
						switch {
						case level < slog.LevelInfo:
							return tint.Attr(6, a) // cyan
						case level < slog.LevelWarn:
							return tint.Attr(2, a) // green
						case level < slog.LevelError:
							return tint.Attr(3, a) // yellow
						default:
							return tint.Attr(1, a) // red
						}
					}
				}
				return a
			},
		})
	default: // "text"
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
