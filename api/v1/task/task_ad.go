package apitask

type TaskAd struct {
	ID    TaskID   `json:"id"`
	Owner string   `json:"owner"`
	Spec  TaskSpec `json:"spec"`
}

func TaskToTaskAd(task *Task) TaskAd {
	return TaskAd{
		ID:    task.Meta.ID,
		Owner: task.Meta.Owner,
		Spec:  task.Spec,
	}
}
