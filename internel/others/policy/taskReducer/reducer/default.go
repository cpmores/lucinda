package policy

import (
	"context"

	"github.com/cpmores/lucinda/api/v1"
)

type TaskReducerPolicy interface {
	Reduce(ctx context.Context, taskID api.TaskID) (api.ChatResponse, error)
}
