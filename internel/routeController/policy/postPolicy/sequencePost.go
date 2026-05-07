package postpolicy

import (
	"context"

	"github.com/cpmores/lucinda/internel/routeController/policy"
	taskrouter "github.com/cpmores/lucinda/internel/routeController/taskRouter"
)

type SequencePost struct {
	Metadata policy.Policy
}

func (policy *SequencePost) Post(ctx context.Context, routedTasks *taskrouter.RoutedTasks) (*taskrouter.RoutedResults, error) {
	// TODO
	
}
