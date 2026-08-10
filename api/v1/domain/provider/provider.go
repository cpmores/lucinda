// Package apiprovider
package apiprovider

import (
	"context"
	"slices"
	"strconv"
	"strings"

	APIChat "github.com/cpmores/lucinda/api/v1/domain/chat"
	APIHardware "github.com/cpmores/lucinda/api/v1/domain/hardware"
)

// ── Provider Interface ─────────────────────────────────────────────────

// Provider is the interface every inference backend driver must implement.
type Provider interface {
	GetID() string
	GetType() ProviderType
	GetModels() []ModelInfo
	GetInfo() ProviderInfo

	// MaxContextTokens returns the maximum context size (input + output tokens)
	// the model can handle. Used by the planner and reducer to stay within limits.
	MaxContextTokens() int

	GPU() (APIHardware.GPUSnapshot, error)
	// Health reports backend liveness (reachable or not). Expensive: does
	// network I/O. Check periodically, not per request.
	Health() ProviderHealth
	// Status reports availability (occupied or not). Cheap: in-memory.
	// A provider is Busy while a model inside it is running inference.
	Status() ProviderStatus

	Generate(ctx context.Context, req *APIChat.ChatRequest) (*APIChat.ChatResponse, error)
	Stream(ctx context.Context, req *APIChat.ChatRequest) (<-chan *APIChat.StreamChunk, error)
	Warm(model string) error
}

// ── Provider ──────────────────────────────────────────────────────────

// ProviderType represents the type of a provider,
// such as "LOCAL", "CLOUD"
type ProviderType string

const (
	Local  ProviderType = "local"
	Cloud  ProviderType = "cloud"
	Hybrid ProviderType = "hybrid"
)

// ProviderInfo represents the information about a provider,
// including its ID and the models it supports.
type ProviderInfo struct {
	ID   string       `json:"id"`
	Type ProviderType `json:"type"`
	// Models lists the models this provider serves, each with labels
	// and capabilities for capability-based request matching.
	Models    []ModelInfo `json:"models"`
	CreatedAt int64       `json:"created_at"`
}

// ModelInfo describes a model served by a provider, along with the labels
// and capabilities used for capability-based request matching (e.g. the
// TaskBoard's Capability CV bidding against a TaskSpec).
type ModelInfo struct {
	// ID is the model name, e.g. "qwen-2.5-gptq".
	ID string `json:"id" mapstructure:"id"`

	// Labels are arbitrary capability tags, e.g. {modality: text, quant: gptq}.
	// Requesters match on these (via TaskSpec.Labels).
	Labels map[string]string `json:"labels,omitempty" mapstructure:"labels"`

	// ParamsB is the parameter count in billions, e.g. 7, 14, 72.
	ParamsB float64 `json:"params_b,omitempty" mapstructure:"params_b"`

	// ContextTokens is this model's context window (input + output tokens).
	ContextTokens int `json:"context_tokens" mapstructure:"context_tokens"`

	// MinVRAM is the VRAM bytes required to serve this model, used for
	// compute-aware placement (matched against TaskSpec.MinVRAM).
	MinVRAM int64 `json:"min_vram,omitempty" mapstructure:"min_vram"`
}

// ModelFilter selects models by their labels/capabilities, Kubernetes-affinity
// style: the model must match at least one Required Term (OR; empty means no
// hard constraint). Within a Term, expressions are AND'd. Preferred terms are
// soft (scored, not excluding).
type ModelFilter struct {
	Required  []Term      `json:"required" mapstructure:"required"`
	Preferred []Preferred `json:"preferred" mapstructure:"preferred"`
}

// Matches reports whether the model satisfies the hard filter: it must match
// at least one Required Term (OR). Empty Required means no constraint.
func (f ModelFilter) Matches(model ModelInfo) bool {
	if len(f.Required) == 0 {
		return true
	}
	for _, t := range f.Required {
		if t.Matches(model) {
			return true
		}
	}
	return false
}

// Term is a set of expressions that must ALL match (AND). Multiple terms in a
// filter are OR'd — satisfying any one Term is enough.
type Term struct {
	MatchExpression []MatchExpression `json:"match_expression" mapstructure:"match_expression"`
}

// Preferred is a soft (weighted) preference. The higher the weight, the more
// the scheduler prefers models satisfying the term.
type Preferred struct {
	MatchExpression []MatchExpression `json:"match_expression" mapstructure:"match_expression"`
}

// MatchExpression is a single label-matching condition, modeled after
// Kubernetes affinity matchExpressions. Operator is one of
// In | NotIn | Exists | DoesNotExist | Gt | Lt.
type MatchExpression struct {
	Key      string   `json:"key" mapstructure:"key"`
	Operator string   `json:"operator" mapstructure:"operator"`
	Values   []string `json:"values,omitempty" mapstructure:"values"`
}

// ModelMatch pairs a provider with the models of its that matched a filter.
// TermMap maps a ModelInfos index to the indices of the Required terms that
// the model satisfied (a model may match more than one term).
type ModelMatch struct {
	Provider   Provider      `json:"provider" mapstructure:"provider"`
	ModelInfos []ModelInfo   `json:"model_infos" mapstructure:"model_infos"`
	TermMap    map[int][]int `json:"term_map,omitempty" mapstructure:"term_map"`
}

// Matches reports whether the model satisfies every expression in the term (AND).
func (t Term) Matches(model ModelInfo) bool {
	for _, e := range t.MatchExpression {
		if !e.Matches(model) {
			return false
		}
	}
	return true
}

// Matches reports whether the model satisfies a single label/capability
// condition. Operator semantics follow Kubernetes affinity:
//
//	In            model has the key and its value is in Values
//	NotIn         model lacks the key, or its value is not in Values
//	Exists        model has the key (non-empty)
//	DoesNotExist  model lacks the key
//	Gt / Lt       numeric comparison of the value against Values[0]
func (e MatchExpression) Matches(model ModelInfo) bool {
	vals := e.valuesOf(model)

	switch e.Operator {
	case "", "In":
		for _, v := range vals {
			if slices.Contains(e.Values, v) {
				return true
			}
		}
		return false
	case "NotIn":
		for _, v := range vals {
			if slices.Contains(e.Values, v) {
				return false
			}
		}
		return true
	case "Exists":
		return len(vals) > 0
	case "DoesNotExist":
		return len(vals) == 0
	case "Gt", "Lt":
		if len(vals) == 0 {
			return false
		}
		v, err := strconv.ParseFloat(vals[0], 64)
		if err != nil {
			return false
		}
		target, err := strconv.ParseFloat(e.Values[0], 64)
		if err != nil {
			return false
		}
		if e.Operator == "Gt" {
			return v > target
		}
		return v < target
	}
	return false
}

// valuesOf resolves the expression key against the model. Structured fields
// (id, params_b, context_tokens, min_vram) map to single values; label keys
// resolve to a set of values, splitting on commas so a label like
// employer="TaskPlanner,TaskCommander" matches In [TaskPlanner].
func (e MatchExpression) valuesOf(model ModelInfo) []string {
	switch e.Key {
	case "id":
		return []string{model.ID}
	case "params_b":
		return []string{strconv.FormatFloat(model.ParamsB, 'f', -1, 64)}
	case "context_tokens":
		return []string{strconv.Itoa(model.ContextTokens)}
	case "min_vram":
		return []string{strconv.FormatInt(model.MinVRAM, 10)}
	default:
		raw, ok := model.Labels[e.Key]
		if !ok || raw == "" {
			return nil
		}
		parts := strings.Split(raw, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts
	}
}

type ProviderStatus string

const (
	Initializing ProviderStatus = "initializing"
	Free         ProviderStatus = "free"
	Busy         ProviderStatus = "busy"
	Pending      ProviderStatus = "pending"
	Error        ProviderStatus = "error"
)

// ProviderHealth represents the health status of a provider,
// including its ID, status, timestamp, and any error message if applicable.
// return from Provider.Health
type ProviderHealth struct {
	ID        string         `json:"id"`
	Status    ProviderStatus `json:"status"`
	Timestamp int64          `json:"timestamp"`
	Error     string         `json:"error,omitempty"`
}

// ── Provider Controller ──────────────────────────────────────────────────────────

// ProviderConfig represents the configuration for registering a provider,
type ProviderConfig struct {
	ID               string            `yaml:"id" mapstructure:"id"`
	Type             ProviderType      `yaml:"type,omitempty" mapstructure:"type"`
	Driver           string            `yaml:"driver,omitempty" mapstructure:"driver"` // "ollama", "openai", "anthropic"
	Host             string            `yaml:"host,omitempty" mapstructure:"host"`
	Port             int               `yaml:"port,omitempty" mapstructure:"port"`
	BaseURL          string            `yaml:"base_url,omitempty" mapstructure:"base_url"` // optional, overrides host:port
	APIKey           string            `yaml:"api_key,omitempty" mapstructure:"api_key"`   // empty for ollama
	Headers          map[string]string `yaml:"headers,omitempty" mapstructure:"headers"`   // extra headers
	Models           []ModelInfo       `yaml:"models,omitempty" mapstructure:"models"`
	TotalVRAM        int64             `yaml:"total_vram,omitempty" mapstructure:"total_vram"`         // physical GPU VRAM in bytes
	MaxContextTokens int               `yaml:"max_context_tokens,omitempty" mapstructure:"max_context_tokens"` // model context window (default 2048)
	Timeout          int               `yaml:"timeout,omitempty" mapstructure:"timeout"`                     // seconds between polls/retries
}
