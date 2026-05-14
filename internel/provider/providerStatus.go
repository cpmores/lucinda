package provider

import (
	api "github.com/cpmores/lucinda/api/v1"
)

// adding ai model info
func AddAIModelInfoToProviderStatus(ps *api.ProviderStatus, psResp *api.OllamaPsResponse) *api.ProviderStatus {
	AIModelInfo := api.AIModelInfo{
		ActiveModels: []string{},
		Capabilities: make(map[string][]api.CapabilityLabel),
		Performance:  make(map[string]api.ModelPerformance),
	}

	for _, model := range psResp.Models {
		AIModelInfo.ActiveModels = append(AIModelInfo.ActiveModels, model.Name)
		AIModelInfo.Capabilities[model.Name] = []api.CapabilityLabel{api.Chat} // default all models have chat capability
		AIModelInfo.Performance[model.Name] = api.ModelPerformance{
			AvgTPS:        0, // TODO:monitor
			MaxContextLen: model.ContextLength,
		}
		AIModelInfo.ModelVram[model.Name] = model.SizeVRAM
	}

	ps.AIModelInfo = AIModelInfo
	return ps
}
