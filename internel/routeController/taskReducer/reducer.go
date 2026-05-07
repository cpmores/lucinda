package taskreducer

import (
	"context"

	"github.com/cpmores/lucinda/api/v1"
	taskrouter "github.com/cpmores/lucinda/internel/routeController/taskRouter"
)

type TaskReducer interface {
	Reduce(ctx context.Context, policyId string, routedResults taskrouter.RoutedResults) (*api.ChatResponse, error)
}
