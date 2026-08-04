// Package taskexecutor subscribes to task.assigned and executes locally.
package taskexecutor

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	APIChat "github.com/cpmores/lucinda/api/v1/domain/chat"
	APIEvent "github.com/cpmores/lucinda/api/v1/messaging/event"
	APIModule "github.com/cpmores/lucinda/api/v1/registry/module"
	APINode "github.com/cpmores/lucinda/api/v1/domain/node"
	APIProvider "github.com/cpmores/lucinda/api/v1/domain/provider"
	APITask "github.com/cpmores/lucinda/api/v1/domain/task"
	APITaskmsg "github.com/cpmores/lucinda/api/v1/messaging/taskmsg"
	taskpostman "github.com/cpmores/lucinda/internal/task_management_layer/task_postman"
	taskstatemanager "github.com/cpmores/lucinda/internal/task_management_layer/task_state_manager"
	tasktracer "github.com/cpmores/lucinda/internal/task_management_layer/task_tracer"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
	providerctrl "github.com/cpmores/lucinda/pkg/infrastructure_layer/provider"
	"github.com/cpmores/lucinda/pkg/infrastructure_layer/transport"
)

const TaskBoardProtocol = "/lucinda/taskboard/1.0.0"

type TaskExecutor interface {
	RegisterWithManager(m modulemanager.ModuleManager) error
	Start(ctx context.Context) error
	Stop() error
}

type executor struct {
	mu     sync.Mutex
	pm     taskpostman.Postman
	pc     providerctrl.ProviderController
	tp     transport.Transport
	tt     tasktracer.TaskTracer
	sm     taskstatemanager.TaskStateManager
	cancel context.CancelFunc
}

func NewTaskExecutor(mm modulemanager.ModuleManager, sm taskstatemanager.TaskStateManager) TaskExecutor {
	postmans := mm.GetByType(APIModule.TaskPostman)
	if len(postmans) == 0 {
		log.Fatal("executor: no TaskPostman found")
	}
	pm := postmans[0].(taskpostman.Postman)

	transports := mm.GetByType(APIModule.Transport)
	if len(transports) == 0 {
		log.Fatal("executor: no Transport found")
	}
	tp := transports[0].(transport.Transport)

	providers := mm.GetByType(APIModule.ProviderController)
	if len(providers) == 0 {
		log.Fatal("executor: no ProviderController found")
	}
	pc := providers[0].(providerctrl.ProviderController)

	tracers := mm.GetByType(APIModule.TaskTracer)
	if len(tracers) == 0 {
		log.Fatal("executor: no TaskTracer found")
	}
	tt := tracers[0].(tasktracer.TaskTracer)

	return &executor{pm: pm, pc: pc, tp: tp, tt: tt, sm: sm}
}

func (e *executor) Start(ctx context.Context) error {
	ctx, e.cancel = context.WithCancel(ctx)

	e.pm.Watch(ctx, APIEvent.TaskAssigned, func(data any) error {
		msg, ok := data.(APITaskmsg.TaskAssignMsg)
		if !ok {
			return nil
		}
		// Dispatch in a goroutine so multiple assignments can execute
		// in parallel. Otherwise, later nodes sit in Claimed state too
		// long and the 30s lease expires before they're started.
		go e.execute(ctx, &msg)
		return nil
	})

	log.Println("executor: started")
	return nil
}

func (e *executor) Stop() error {
	if e.cancel != nil {
		e.cancel()
	}
	log.Println("executor: stopped")
	return nil
}

func (e *executor) execute(ctx context.Context, msg *APITaskmsg.TaskAssignMsg) error {
	e.tt.Assigned(&APITask.Task{
		Meta: APITask.TaskMeta{ID: msg.NodeID},
		Spec: msg.Spec,
	})

	// Find a provider that supports the requested model.
	var prov APIProvider.Provider
	for _, p := range e.pc.List() {
		for _, m := range p.GetModels() {
			if m == msg.Spec.Model {
				prov = p
				break
			}
		}
		if prov != nil {
			break
		}
	}
	if prov == nil {
		list := e.pc.List()
		if len(list) > 0 {
			prov = list[0]
		}
	}
	if prov == nil {
		e.tt.Remove(msg.NodeID)
		return fmt.Errorf("no provider available")
	}

	// Determine whether this is a local task (we own the plan) or a
	// remote task (we are executing on behalf of another node).
	localTask := msg.OriginNodeID == "" || msg.OriginNodeID == string(e.tp.ID())

	// Signal StateManager — transitions from Claimed to Running.
	// Only for local tasks; remote nodes don't have the plan.
	if localTask && e.sm != nil {
		if err := e.sm.Start(msg.NodeID); err != nil {
			log.Printf("executor: sm.Start(%s): %v", msg.NodeID, err)
		}
	}

	model := msg.Spec.Model
	if model == "" {
		models := prov.GetModels()
		if len(models) > 0 {
			model = models[0]
		}
	}

	// Execute.
	resp, err := prov.Generate(ctx, &APIChat.ChatRequest{
		Model: model,
		Messages: []APIChat.ChatMessage{{
			Role:    "user",
			Content: []APIChat.ContentPart{{Type: APIChat.ContentText, Text: msg.Prompt}},
		}},
		Options: APIChat.ModelOptions{
			MaxTokens:   msg.Spec.BudgetTokens,
			Temperature: 0.7,
		},
	})
	if err != nil {
		e.tt.Remove(msg.NodeID)
		return fmt.Errorf("generate: %w", err)
	}

	result := APITaskmsg.TaskResultMsg{
		NodeID: msg.NodeID,
		Output: textFromResponse(resp),
	}

	if localTask {
		// Local execution — publish to EventBus. The board handler
		// calls SetOutput + Complete (which cascades and may notify).
		e.pm.Publish(APIEvent.TaskDone, APIEvent.NewEvent(APIEvent.TaskDone, result))
	} else {
		// Remote execution — send result back to the origin node via
		// Transport. The origin's Postman.Deliver bridges this to its
		// local EventBus as a TaskDone event, where the board handler
		// picks it up.
		e.tp.Send(ctx, APINode.NodeID(msg.OriginNodeID),
			APINode.NewNodeMessage(TaskBoardProtocol,
				string(APIEvent.TaskDone),
				e.tp.ID(),
				APINode.NodeID(msg.OriginNodeID),
				result,
			))
	}
	return nil
}

func textFromResponse(resp *APIChat.ChatResponse) string {
	var b strings.Builder
	for _, part := range resp.Message.Content {
		if part.Type == APIChat.ContentText {
			b.WriteString(part.Text)
		}
	}
	return b.String()
}

func (e *executor) GetModuleType() APIModule.ModuleType { return APIModule.TaskExecutor }
func (e *executor) GetModuleID() APIModule.ModuleID {
	return APIModule.NewModuleID(e.GetModuleType(), "default")
}
func (e *executor) CheckHealth() APIModule.ModuleHealth {
	return APIModule.NewModuleHealth(e.GetModuleID(), e.GetModuleType(), APIModule.Running)
}
func (e *executor) RegisterWithManager(m modulemanager.ModuleManager) error { return m.Register(e) }
