// internel/router/taskDivider/taskDivider.go
package taskdivider

import (
	"context"
	"fmt"

	"github.com/cpmores/lucinda/api/v1"
	"github.com/cpmores/lucinda/internel/routeController/policy"
)

type TaskDivider interface {
	Divide(ctx context.Context, policyKey string, traceID string, req *api.ChatRequest) (*ExecutionPlan, error)
}

// one single task divided into multiple sub-tasks
type SubTask struct {
	ID        string
	ReducedID string
	Request   *api.ChatRequest
	DependsOn []string
}

type ExecutionPlan struct {
	TraceID    string
	Tasks      []*SubTask
	IsParallel bool
}

func (e ExecutionPlan) GetTraceID() string {
	return e.TraceID
}

func (e *ExecutionPlan) GetTasks() []*SubTask {
	return e.Tasks
}

var DividePolicies map[string]*DividerPolicy = make(map[string]*DividerPolicy)

type DividerPolicy interface {
	Divide(ctx context.Context, traceId string, req *api.ChatRequest) (*ExecutionPlan, error)
	GetMetaData() policy.Policy
}

func RegisterDividePolicy(p DividerPolicy) error {
	if p.GetMetaData().Type != policy.PolicyTypeDivide {
		return fmt.Errorf("register divide policy error, type: %s", p.GetMetaData().Type)
	}
	DividePolicies[p.GetMetaData().GetPolicyKey()] = &p
	return nil
}

func IfPolicyInDivide(policy policy.Policy) bool {
	_, ok := DividePolicies[policy.GetPolicyKey()]
	if !ok {
		return false
	}

	return true
}

type DefaultTaskDivier struct{}

func (divider *DefaultTaskDivier) Divide(ctx context.Context, policyKey string, traceID string, req *api.ChatRequest) (*ExecutionPlan, error) {
	dividerPolicy, ok := DividePolicies[policyKey]
	if !ok {
		return nil, fmt.Errorf("no divide %s policy founded", policyKey)
	}

	executionPlan, err := (*dividerPolicy).Divide(ctx, traceID, req)
	if err != nil {
		return nil, err
	}

	return executionPlan, nil
}
