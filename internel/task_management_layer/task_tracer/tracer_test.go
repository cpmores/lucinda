package tasktracer

import (
	"testing"

	APITask "github.com/cpmores/lucinda/api/v1/task"
)

func makeTask(id APITask.TaskID, prompt string) *APITask.Task {
	return &APITask.Task{
		Meta: APITask.TaskMeta{ID: id, Owner: "test-owner"},
		Spec: APITask.TaskSpec{Model: "gemma3"},
		Prompt: prompt,
	}
}

func TestImport(t *testing.T) {
	tr := NewTaskTracer()
	task := makeTask("task-1", "parse data")

	if err := tr.Import(task); err != nil {
		t.Fatalf("Import: %v", err)
	}

	got, err := tr.GetLocal("task-1")
	if err != nil {
		t.Fatalf("GetLocal: %v", err)
	}
	if got.Meta.ID != "task-1" {
		t.Fatalf("expected task-1, got %s", got.Meta.ID)
	}
}

func TestImportDuplicate(t *testing.T) {
	tr := NewTaskTracer()
	task := makeTask("task-1", "hi")
	tr.Import(task)

	if err := tr.Import(task); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestAssigned(t *testing.T) {
	tr := NewTaskTracer()
	task := makeTask("task-2", "remote work")

	if err := tr.Assigned(task); err != nil {
		t.Fatalf("Assigned: %v", err)
	}

	got, err := tr.GetAssigned("task-2")
	if err != nil {
		t.Fatalf("GetAssigned: %v", err)
	}
	if got.Meta.ID != "task-2" {
		t.Fatalf("expected task-2, got %s", got.Meta.ID)
	}
}

func TestAssignedDuplicate(t *testing.T) {
	tr := NewTaskTracer()
	task := makeTask("task-2", "hi")
	tr.Assigned(task)

	if err := tr.Assigned(task); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestRemove(t *testing.T) {
	tr := NewTaskTracer()
	tr.Import(makeTask("t1", "a"))
	tr.Assigned(makeTask("t2", "b"))

	if err := tr.Remove("t1"); err != nil {
		t.Fatalf("Remove local: %v", err)
	}
	if err := tr.Remove("t2"); err != nil {
		t.Fatalf("Remove assigned: %v", err)
	}
	if err := tr.Remove("nope"); err == nil {
		t.Fatal("expected error for missing task")
	}
}

func TestGetLocalNotFound(t *testing.T) {
	tr := NewTaskTracer()
	if _, err := tr.GetLocal("nope"); err == nil {
		t.Fatal("expected error")
	}
}

func TestGetAssignedNotFound(t *testing.T) {
	tr := NewTaskTracer()
	if _, err := tr.GetAssigned("nope"); err == nil {
		t.Fatal("expected error")
	}
}

func TestListLocal(t *testing.T) {
	tr := NewTaskTracer()
	tr.Import(makeTask("a", "one"))
	tr.Import(makeTask("b", "two"))
	tr.Assigned(makeTask("c", "three"))

	list := tr.ListLocal()
	if len(list) != 2 {
		t.Fatalf("expected 2 local, got %d", len(list))
	}
}

func TestListAssigned(t *testing.T) {
	tr := NewTaskTracer()
	tr.Import(makeTask("a", "one"))
	tr.Assigned(makeTask("b", "two"))
	tr.Assigned(makeTask("c", "three"))

	list := tr.ListAssigned()
	if len(list) != 2 {
		t.Fatalf("expected 2 assigned, got %d", len(list))
	}
}

func TestSeparateStorage(t *testing.T) {
	tr := NewTaskTracer()
	tr.Import(makeTask("shared-id", "local version"))
	tr.Assigned(makeTask("shared-id", "assigned version"))

	local, _ := tr.GetLocal("shared-id")
	assigned, _ := tr.GetAssigned("shared-id")

	if local.Prompt != "local version" {
		t.Fatal("local and assigned storage are not separate")
	}
	if assigned.Prompt != "assigned version" {
		t.Fatal("local and assigned storage are not separate")
	}
}
