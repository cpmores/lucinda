// github.com/cpmores/lucinda/api/v1/types.go
package api

// Transporter
type NodeID string

func (id NodeID) String() string {
	return string(id)
}

type NodeMessage struct {
	ProtocolID string `json:"protocol_id"`
	Payload    []byte `json:"payload"`
	From       NodeID `json:"from"`
}
type NodeMessageHandler func(msg *NodeMessage) error

type NodeStatus struct {
	NodeID NodeID
	Peers  []NodeID
}
