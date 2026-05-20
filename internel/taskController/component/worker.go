package component

import "context"

type Worker struct {
}

func NewTaskWorker() *Worker {
	return &Worker{}
}

func (w *Worker) StartWrapper(ctx context.Context) error {
	return nil
}

func (w *Worker) StartDivider(ctx context.Context) error {
	return nil
}

func (w *Worker) StartPublisher(ctx context.Context) error {
	return nil
}

func (w *Worker) StartInterviewer(ctx context.Context) error {
	return nil
}

func (w *Worker) StartReducer(ctx context.Context) error {
	return nil
}
