package apitask

type TaskAd struct {
	ID    TaskID   `json:"id"`
	Owner string   `json:"owner"`
	Spec  TaskSpec `json:"spec"`
}

func TaskToTaskAd(task *Task) TaskAd {
	ad := TaskAd{
		ID:    task.Meta.ID,
		Owner: task.Meta.Owner,
		Spec:  task.Spec,
	}
	ad.Spec.Prompt = "" // do not include prompt in ads
	return ad
}
