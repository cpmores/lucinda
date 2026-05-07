// internel/router/router.go
package taskrouter

import (
	"context"
	"fmt"

	"github.com/cpmores/lucinda/api/v1"
	"github.com/cpmores/lucinda/internel/node"
	"github.com/cpmores/lucinda/internel/routeController/policy"
	taskdivider "github.com/cpmores/lucinda/internel/routeController/taskDivider"
)

type TaskRouter interface {
	Route(ctx context.Context, policyKey string, plan *taskdivider.ExecutionPlan) (*RoutedTasks, error)
	Post(ctx context.Context, policyKey string, routedTasks *RoutedTasks) (*RoutedResults, error)
}

type RoutedTasks struct {
	TraceID string
	Plans   []*RouterPlan
}

type RouterPlan struct {
	NodeID  node.NodeID
	SubTask *taskdivider.SubTask
}

type RoutedResults struct {
	TraceID string
	Plans   []*RouterResponse
}

type RouterResponse struct {
	NodeID      node.NodeID
	RespChannel chan *api.ChatResponse
}

var RoutePolicies map[string]*RoutePolicy = make(map[string]*RoutePolicy)
var PostPolicies map[string]*PostPolicy = make(map[string]*PostPolicy)

type RoutePolicy interface {
	Route(ctx context.Context, plan *taskdivider.ExecutionPlan) (*RoutedTasks, error)
	GetMetaData() policy.Policy
}

type PostPolicy interface {
	Post(ctx context.Context, routedTasks *RoutedTasks) (*RoutedResults, error)
	GetMetaData() policy.Policy
}

func IfPolicyInRoute(policy policy.Policy) bool {
	_, ok := RoutePolicies[policy.GetPolicyKey()]
	if !ok {
		return false
	}

	return true
}

func IfPolicyInPost(policy policy.Policy) bool {
	_, ok := PostPolicies[policy.GetPolicyKey()]
	if !ok {
		return false
	}

	return true
}

type DefaultTaskRouter struct{}

func (router *DefaultTaskRouter) Route(ctx context.Context, policyKey string, plan *taskdivider.ExecutionPlan) (*RoutedTasks, error) {
	routePolicy, ok := RoutePolicies[policyKey]
	if !ok {
		return nil, fmt.Errorf("no route %s policy founded", policyKey)
	}

	routedTasks, err := (*routePolicy).Route(ctx, plan)
	if err != nil {
		return nil, err
	}

	return routedTasks, nil
}

func (router *DefaultTaskRouter) Post(ctx context.Context, policyKey string, routedTasks *RoutedTasks) (*RoutedResults, error) {
	postPolicy, ok := PostPolicies[policyKey]
	if !ok {
		return nil, fmt.Errorf("no post %s policy founded", policyKey)
	}

	routedResults, err := (*postPolicy).Post(ctx, routedTasks)
	if err != nil {
		return nil, err
	}

	return routedResults, nil
}

func RegisterRoutePolicy(p RoutePolicy) error {
	if p.GetMetaData().Type != policy.PolicyTypeRoute {
		return fmt.Errorf("register route policy error, type: %s", p.GetMetaData().Type)
	}
	RoutePolicies[p.GetMetaData().GetPolicyKey()] = &p
	return nil
}
