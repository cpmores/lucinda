// Package apinode provides structures for node information and node messages.
package apinode

// NodeID is a unique identifier for a node in the network
type NodeID string

// Protocol is the protocol used for communication between nodes
type Protocol string

// NodeMessage represents a message sent between nodes in the network
type NodeMessage struct {
	Protocol  Protocol
	Topic     string // decide eventbus channel
	Timestamp int64
	From      NodeID
	To        NodeID
	Body      any
}

func NewNodeMessage(protocol Protocol, topic string, from NodeID, to NodeID, body any) NodeMessage {
	return NodeMessage{
		Protocol:  protocol,
		Topic:     topic,
		Timestamp: 0, // This should be set to the current time when sending
		From:      from,
		To:        to,
		Body:      body,
	}
}
