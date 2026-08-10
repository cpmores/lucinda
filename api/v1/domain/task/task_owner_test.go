package apitask

import "testing"

func TestTaskToTaskAdCopiesOwner(t *testing.T) {
	task := Task{
		Meta: TaskMeta{ID: "t-1", Owner: "node-A"},
		Spec: TaskSpec{Prompt: "secret prompt", Model: "m1"},
	}

	ad := TaskToTaskAd(&task)

	if ad.Owner != "node-A" {
		t.Fatalf("ad owner = %q, want %q", ad.Owner, "node-A")
	}
	if ad.Spec.Prompt != "" {
		t.Fatalf("ad must not carry the prompt, got %q", ad.Spec.Prompt)
	}
	if ad.Spec.Model != "m1" {
		t.Fatalf("ad spec lost model: %q", ad.Spec.Model)
	}
}

func TestPlanToTasksPropagatesOwner(t *testing.T) {
	plan := &TaskPlan{
		ID:    "plan-1",
		Owner: "node-A",
		Nodes: map[TaskID]*TaskNode{
			"n1": {ID: "n1", Spec: TaskSpec{Model: "m1", Stage: StageExecute}},
			"n2": {ID: "n2", Spec: TaskSpec{Model: "m1", Stage: StageExecute}},
			"n3": {ID: "n3", Spec: TaskSpec{Model: "m1", Stage: StageExecute}},
		},
	}

	tasks := plan.ToTasks()

	if len(tasks) != 3 {
		t.Fatalf("got %d tasks, want 3", len(tasks))
	}
	for _, task := range tasks {
		if task.Meta.Owner != "node-A" {
			t.Errorf("task %s owner = %q, want %q (shared with plan)", task.Meta.ID, task.Meta.Owner, "node-A")
		}
	}
}
