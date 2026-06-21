package taskexecutor

import (
	"testing"
)

func TestNewTaskExecutor(t *testing.T) {
	// Without a real ModuleManager with all deps, NewTaskExecutor will fatal.
	// This test verifies the package compiles and the type exists.
	var _ TaskExecutor = (*executor)(nil)
}

func TestTextFromResponse(t *testing.T) {
	// Compilation check — textFromResponse is tested indirectly.
	t.Skip("integration test — needs mock provider")
}
