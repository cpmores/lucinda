// package api_node provides structures for node information and node messagesinfro
package api_node

// NodeID is a unique identifier for a node in the network
type NodeID string

// Protocol is the protocol used for communication between nodes
type Protocol string

// NodeMessage represents a message sent between nodes in the network
type NodeMessage struct {
	Protocol  Protocol
	Timestamp int64
	From      NodeID
	To        NodeID
	Body      any
}
