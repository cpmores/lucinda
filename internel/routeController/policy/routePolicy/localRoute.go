package routepolicy

import (
	"context"
	"log"

	"github.com/cpmores/lucinda/internel/routeController/policy"
	taskdivider "github.com/cpmores/lucinda/internel/routeController/taskDivider"
	taskrouter "github.com/cpmores/lucinda/internel/routeController/taskRouter"
)

type LocalRouteLolicy struct {
	Metadata policy.Policy
}

func (policy *LocalRouteLolicy) Route(ctx context.Context, plan *taskdivider.ExecutionPlan) (*taskrouter.RoutedTasks, error) {
	traceID, subTasks := plan.GetTraceID(), plan.GetTasks()
	plans := make([]*taskrouter.RouterPlan, 0)
	for _, subtask := range subTasks {
		routerplan := &taskrouter.RouterPlan{
			NodeID:  "Local",
			SubTask: subtask,
		}

		plans = append(plans, routerplan)
	}

	routedTasks := &taskrouter.RoutedTasks{
		TraceID: traceID,
		Plans:   plans,
	}

	return routedTasks, nil
}

func (policy *LocalRouteLolicy) GetMetaData() policy.Policy {
	return policy.Metadata
}

func DefaultLocalRoutePolicy() *LocalRouteLolicy {
	return &LocalRouteLolicy{
		Metadata: policy.Policy{
			ID:      "Default",
			Version: "1.0",
			Type:    policy.PolicyTypeRoute,
			Desc:    "only for test",
		},
	}
}

func init() {
	policy := DefaultLocalRoutePolicy()
	err := taskrouter.RegisterRoutePolicy(policy)
	if err != nil {
		log.Printf("Exception occurs initing LocalRoute policy, error: %s", err.Error())
	}
}
