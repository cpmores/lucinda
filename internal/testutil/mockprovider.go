package testutil

import (
	"context"
	"fmt"
	"strings"
	"sync"

	APIChat "github.com/cpmores/lucinda/api/v1/domain/chat"
	APIHardware "github.com/cpmores/lucinda/api/v1/domain/hardware"
	APIProvider "github.com/cpmores/lucinda/api/v1/domain/provider"
	APIModule "github.com/cpmores/lucinda/api/v1/registry/module"
	modulemanager "github.com/cpmores/lucinda/pkg/infrastructure_layer/module_manager"
	"github.com/spf13/viper"
)

// MockProvider is an in-memory Provider fake. Planning prompts (containing
// the decomposition marker) return a canned 2-node DAG; everything else is
// echoed as a result, so tests exercise the full pipeline without a backend.
type MockProvider struct {
	mu      sync.Mutex
	IDVal   string
	Models  []APIProvider.ModelInfo
	PlanOut string

	// ReActOut is a queue of JSON decisions returned for orchestrator
	// prompts ("task orchestrator"), consumed in order.
	ReActOut []string
	// ErrPrompt, when non-empty, makes Generate fail for prompts containing it.
	ErrPrompt string
	// StreamOut, when non-empty, is the sequence of deltas returned by Stream
	// (a Done chunk is appended). Defaults to ["hello ", "world"].
	StreamOut []string
	// LastReactPrompt records the most recent orchestrator prompt, so tests
	// can assert dependency outputs were fed into the downstream goal.
	LastReactPrompt string

	// Called is signaled (non-blocking) each time Generate is entered.
	// Hold, when non-nil, blocks Generate until closed. Together they let a
	// test deterministically sequence the workflow (e.g. open the SSE stream
	// before the plan executes).
	Called chan struct{}
	Hold   chan struct{}
}

var _ APIProvider.Provider = (*MockProvider)(nil)

func NewMockProvider(id string, models []APIProvider.ModelInfo) *MockProvider {
	if len(models) == 0 {
		models = []APIProvider.ModelInfo{{
			ID: "mock-model",
			Labels: map[string]string{
				"modality": "text",
				"employer": "TaskPlanner,TaskCommander,TaskExecutor",
			},
			ContextTokens: 2048,
		}}
	}
	return &MockProvider{
		IDVal:   id,
		Models:  models,
		PlanOut: `{"transactions":[{"id":"t1","goal":"step one","labels":["cpu"],"deps":[]},{"id":"t2","goal":"step two","labels":["cpu"],"deps":["t1"]}]}`,
	}
}

func (p *MockProvider) GetID() string { return p.IDVal }
func (p *MockProvider) GetType() APIProvider.ProviderType {
	return APIProvider.Local
}
func (p *MockProvider) GetModels() []APIProvider.ModelInfo { return p.Models }
func (p *MockProvider) GetInfo() APIProvider.ProviderInfo {
	return APIProvider.ProviderInfo{ID: p.IDVal, Type: APIProvider.Local, Models: p.Models}
}
func (p *MockProvider) MaxContextTokens() int { return 2048 }
func (p *MockProvider) GPU() (APIHardware.GPUSnapshot, error) {
	return APIHardware.GPUSnapshot{}, nil
}
func (p *MockProvider) Health() APIProvider.ProviderHealth {
	return APIProvider.ProviderHealth{ID: p.IDVal, Status: APIProvider.Free}
}
func (p *MockProvider) Status() APIProvider.ProviderStatus { return APIProvider.Free }

func (p *MockProvider) Generate(_ context.Context, req *APIChat.ChatRequest) (*APIChat.ChatResponse, error) {
	if p.Called != nil {
		select {
		case p.Called <- struct{}{}:
		default:
		}
	}
	if p.Hold != nil {
		<-p.Hold
	}
	prompt := ""
	if len(req.Messages) > 0 && len(req.Messages[0].Content) > 0 {
		prompt = req.Messages[0].Content[0].Text
	}
	if p.ErrPrompt != "" && strings.Contains(prompt, p.ErrPrompt) {
		return nil, fmt.Errorf("mock provider error on %q", p.ErrPrompt)
	}
	var text string
	switch {
	case strings.Contains(prompt, "task orchestrator"):
		p.mu.Lock()
		p.LastReactPrompt = prompt
		if len(p.ReActOut) > 0 {
			text = p.ReActOut[0]
			p.ReActOut = p.ReActOut[1:]
		} else {
			text = `{"action":"done","answer":"no more steps"}`
		}
		p.mu.Unlock()
	case strings.Contains(prompt, "Decompose this request"):
		text = p.PlanOut
	default:
		text = "RESULT:" + strings.TrimSpace(prompt)
	}
	return &APIChat.ChatResponse{
		Model:   req.Model,
		Message: APIChat.ChatMessage{Content: []APIChat.ContentPart{{Type: APIChat.ContentText, Text: text}}},
		Done:    true,
	}, nil
}

func (p *MockProvider) Stream(ctx context.Context, req *APIChat.ChatRequest) (<-chan *APIChat.StreamChunk, error) {
	deltas := p.StreamOut
	if len(deltas) == 0 {
		deltas = []string{"hello ", "world"}
	}
	ch := make(chan *APIChat.StreamChunk, len(deltas)+1)
	go func() {
		defer close(ch)
		for _, d := range deltas {
			select {
			case <-ctx.Done():
				return
			case ch <- &APIChat.StreamChunk{Delta: d}:
			}
		}
		select {
		case <-ctx.Done():
		case ch <- &APIChat.StreamChunk{Done: true}:
		}
	}()
	return ch, nil
}

func (p *MockProvider) Warm(string) error { return nil }

// MockProviderController satisfies the ProviderController interface and
// registers with the module manager as the ProviderController module.
type MockProviderController struct {
	Providers []APIProvider.Provider
}

func NewMockProviderController(providers ...APIProvider.Provider) *MockProviderController {
	return &MockProviderController{Providers: providers}
}

func (c *MockProviderController) LoadProviders(*viper.Viper) error { return nil }
func (c *MockProviderController) Register(APIProvider.ProviderConfig) error {
	return nil
}
func (c *MockProviderController) Get(id string) (APIProvider.Provider, error) {
	for _, p := range c.Providers {
		if p.GetID() == id {
			return p, nil
		}
	}
	return nil, fmt.Errorf("provider not found: %s", id)
}
func (c *MockProviderController) List() []APIProvider.Provider { return c.Providers }

func (c *MockProviderController) GetProvByFilter(f APIProvider.ModelFilter) ([]APIProvider.ModelMatch, error) {
	var matches []APIProvider.ModelMatch
	for _, p := range c.Providers {
		if p.Status() != APIProvider.Free {
			continue
		}
		var modelInfos []APIProvider.ModelInfo
		termMap := map[int][]int{}
		for _, m := range p.GetModels() {
			var matched []int
			for ti, term := range f.Required {
				if term.Matches(m) {
					matched = append(matched, ti)
				}
			}
			if len(matched) > 0 {
				modelInfos = append(modelInfos, m)
				termMap[len(modelInfos)-1] = matched
			}
		}
		if len(modelInfos) > 0 {
			matches = append(matches, APIProvider.ModelMatch{Provider: p, ModelInfos: modelInfos, TermMap: termMap})
		}
	}
	return matches, nil
}

func (c *MockProviderController) Health(id string) (APIProvider.ProviderHealth, error) {
	p, err := c.Get(id)
	if err != nil {
		return APIProvider.ProviderHealth{}, err
	}
	return p.Health(), nil
}
func (c *MockProviderController) HealthAll() []APIProvider.ProviderHealth {
	var out []APIProvider.ProviderHealth
	for _, p := range c.Providers {
		out = append(out, p.Health())
	}
	return out
}
func (c *MockProviderController) GPU() APIHardware.GPUSnapshot { return APIHardware.GPUSnapshot{} }

// ── AvailableModule ────────────────────────────────────────────────────

func (c *MockProviderController) GetModuleType() APIModule.ModuleType {
	return APIModule.ProviderController
}
func (c *MockProviderController) GetModuleID() APIModule.ModuleID {
	return APIModule.NewModuleID(c.GetModuleType(), "default")
}
func (c *MockProviderController) CheckHealth() APIModule.ModuleHealth {
	return APIModule.NewModuleHealth(c.GetModuleID(), c.GetModuleType(), APIModule.Running)
}
func (c *MockProviderController) RegisterWithManager(m modulemanager.ModuleManager) error {
	return m.Register(c)
}
func (c *MockProviderController) DependsOn() map[APIModule.ModuleType]string { return nil }
func (c *MockProviderController) DependsEnable() error                       { return nil }

var _ modulemanager.AvailableModule = (*MockProviderController)(nil)
