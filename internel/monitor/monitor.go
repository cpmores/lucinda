package monitor

import (
	"github.com/cpmores/lucinda/api/v1"
	"github.com/cpmores/lucinda/internel/provider"
)

type LucindaMonitor interface {
	NodeMonitor
	ProviderMonitor
	TaskMonitor
	HardwareMonitor
}

type NodeMonitor interface {
	GetNodeStatus() api.NodeStatus
	GetNodeProviderStatus() api.NodeProviderStatus
}
type ProviderMonitor interface {
	GetProviders() (map[string]api.ProviderStatus, error)
	GetProvider(ProviderID string) (api.ProviderStatus, bool)
}

type TaskMonitor interface {
	GetTasks() (map[api.TaskID]api.Task, error)
	GetTask(TaskID api.TaskID) (api.Task, bool)
	GetCurrentTaskRuntime(ProviderID string) (TaskRuntimeStatus, error)
}

type HardwareMonitor interface {
	GetHardwareStatus() api.HardwareStatus
}

type defaultMonitor struct{}
type TaskRuntimeStatus struct {
	CurrentTaskID string `json:"current_task_id"`
	QueueDepth    int64  `json:"queue_depth"`
	ForecastTime  int64  `json:"forecast_time"` // in milliseconds
}

var Monitor = &defaultMonitor{}

func (monitor *defaultMonitor) GetNodeStatus() api.NodeStatus {
	// Implementation for getting node status
	return api.NodeStatus{}
}

func (monitor *defaultMonitor) GetNodeProviderStatus() api.NodeProviderStatus {
	// Implementation for getting node provider status
	return api.NodeProviderStatus{}
}

func (monitor *defaultMonitor) GetProviders() (map[string]api.ProviderStatus, error) {
	ids, statuses, err := provider.ProviderController.GetStatus()
	if err != nil {
		return nil, err
	}

	providerStatuses := make(map[string]api.ProviderStatus)
	for _, id := range ids {
		providerStatuses[id] = statuses[id]
	}

	return providerStatuses, nil
}

func (monitor *defaultMonitor) GetProvider(ProviderID string) (api.ProviderStatus, bool) {
	providers, err := monitor.GetProviders()
	if err != nil {
		return api.ProviderStatus{}, false
	}

	provider, exists := providers[ProviderID]
	return provider, exists
}

func (monitor *defaultMonitor) GetTasks() (map[api.TaskID]api.Task, error) {
	// Implementation for getting tasks
	return nil, nil
}

func (monitor *defaultMonitor) GetTask(TaskID api.TaskID) (api.Task, bool) {
	// Implementation for getting a specific task
	return api.Task{}, false
}

func GetCurrentTaskRuntime(ProviderID string) (TaskRuntimeStatus, error) {
	// Implementation for getting taskruntime status
	return TaskRuntimeStatus{}, nil
}

func (monitor *defaultMonitor) GetHardwareStatus() api.HardwareStatus {
	// Implementation for getting hardware status
	return api.HardwareStatus{}
}
