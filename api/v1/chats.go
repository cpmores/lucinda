// github.com/cpmores/lucinda/api/v1/types.go
package api

import "time"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest sent to models
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

// ChatResponse received from models
type ChatResponse struct {
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	Message   Message   `json:"message"`
	Done      bool      `json:"done"`
	Provider  string    `json:"provider"`
	Metadata  any       `json:"metadata"`
}
