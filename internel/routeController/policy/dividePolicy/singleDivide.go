package dividepolicy

import (
	"context"
	"log"

	"github.com/cpmores/lucinda/api/v1"
	"github.com/cpmores/lucinda/internel/routeController/policy"
	taskdivider "github.com/cpmores/lucinda/internel/routeController/taskDivider"
)

type SingleDividerPolicy struct {
	Metadata policy.Policy
}

func (policy *SingleDividerPolicy) Divide(ctx context.Context, traceID string, reducedID string, req *api.ChatRequest) (*taskdivider.ExecutionPlan, error) {
	subtask := &taskdivider.SubTask{
		ID:        "singleTest",
		ReducedID: reducedID,
		Request:   req,
		DependsOn: make([]string, 0),
	}

	executionPlan := &taskdivider.ExecutionPlan{
		Tasks:      []*taskdivider.SubTask{subtask},
		IsParallel: false,
		TraceID:    traceID,
	}

	return executionPlan, nil
}

func (policy *SingleDividerPolicy) GetMetaData() policy.Policy {
	return policy.Metadata
}

func DefaultSingleDividePolicy() *SingleDividerPolicy {
	return &SingleDividerPolicy{
		Metadata: policy.Policy{
			ID:      "Default",
			Version: "1.0",
			Type:    policy.PolicyTypeDivide,
			Desc:    "only for test",
		},
	}
}

func init() {
	policy := DefaultSingleDividePolicy()
	err := taskdivider.RegisterDividePolicy(policy)
	if err != nil {
		log.Printf("Exception occurs initing SingleDivide policy, error: %s", err.Error())
	}
}
