package api

type CapabilityLabel string

// model capability labels
const (
	Chat            CapabilityLabel = "chat"
	ImageGeneration CapabilityLabel = "image-generation"
	ToolUse         CapabilityLabel = "tool-use"
)

// NodeProviderStatus is a CV for Subtask
type NodeProviderStatus struct {
	NodeID           string                    `json:"node_id"`
	Timestamp        int64                     `json:"timestamp"`
	ProviderIDs      []string                  `json:"provider_ids"`
	ProviderStatuses map[string]ProviderStatus `json:"provider_statuses"`
	NodeHardware     NodeHardware              `json:"node_hardware"`
}

type NodeHardware struct {
	TotalVram int64 `json:"total_vram"`
	FreeVram  int64 `json:"free_vram"`
}

// ProviderStatus handles logic-level status
type ProviderStatus struct {
	ID        string `json:"id"`
	Timestamp int64  `json:"timestamp"`
	State     int    `json:"state"` // 1 means healthy, 0 means unhealthy

	AIModelInfo       AIModelInfo       `json:"ai_model_info"`
	TaskRuntimeStatus TaskRuntimeStatus `json:"task_runtime_status"`
}

type AIModelInfo struct {
	ActiveModels []string                     `json:"active_models"`
	Capabilities map[string][]CapabilityLabel `json:"capabilities"` // model name -> capability label array
	Performance  map[string]ModelPerformance  `json:"performance"`
	ModelVram    map[string]int64             `json:"model_vram"` // model name -> vram usage in bytes
}

type ModelPerformance struct {
	AvgTPS        float64 `json:"avg_tps"`         // average tokens per second
	MaxContextLen int64   `json:"max_context_len"` // maximum context length
}

type TaskRuntimeStatus struct {
	CurrentTaskId string `json:"current_task_id"`
	QueueDepth    int64  `json:"queue_depth"`
	ForecastTime  int64  `json:"forecast_time"` // in seconds
}

// adding ai model info
func AddAIModelInfoToProviderStatus(ps *ProviderStatus, psResp *OllamaPsResponse) *ProviderStatus {
	AIModelInfo := AIModelInfo{
		ActiveModels: []string{},
		Capabilities: make(map[string][]CapabilityLabel),
		Performance:  make(map[string]ModelPerformance),
	}

	for _, model := range psResp.Models {
		AIModelInfo.ActiveModels = append(AIModelInfo.ActiveModels, model.Name)
		AIModelInfo.Capabilities[model.Name] = []CapabilityLabel{Chat} // default all models have chat capability
		AIModelInfo.Performance[model.Name] = ModelPerformance{
			AvgTPS:        0, // TODO:monitor
			MaxContextLen: model.ContextLength,
		}
		AIModelInfo.ModelVram[model.Name] = model.SizeVRAM
	}

	ps.AIModelInfo = AIModelInfo
	return ps
}
