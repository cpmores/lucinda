// github.com/cpmores/lucinda/api/v1/types.go
package api

type Task struct {
	ID        string
	ReducedID string
	TraceID   string
	Request   *ChatRequest
}
