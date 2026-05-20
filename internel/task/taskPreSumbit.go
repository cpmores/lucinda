package task

import "github.com/cpmores/lucinda/api/v1"

type TaskPreSubmit struct {
	TaskID      api.TaskID
	ChatRequest api.ChatRequest
}

func GenerateTaskPreSumbitEvent(taskID api.TaskID, chatReq api.ChatRequest) api.Event {
	return api.Event{
		Topic: api.TASK_SUBMITTED,
		Data:  TaskPreSubmit{TaskID: taskID, ChatRequest: chatReq},
	}
}

func (t TaskPreSubmit) GetTaskID() api.TaskID {
	return t.TaskID
}
