package capabilitymetric

import (
	"context"
	"time"

	"github.com/cpmores/lucinda/internel/node"
)

type Metric struct {
	Value     float64
	Timestamp time.Time
}

type NodeCapabilityInformer interface {
	// rolling for other nodes' capabilities
	Run(ctx context.Context) error

	Get(nodeId node.NodeID) (NodeCapability, bool)
	GetLocal() NodeCapability

	// new node came in
	Watch() (<-chan NodeCapability, error)
}

type NodeCapability struct {
	NodeID    node.NodeID
	BaseScore float64
	Models    map[string]string
	Metrics   struct {
		Static   Metric
		RealTime Metric
	}
}
