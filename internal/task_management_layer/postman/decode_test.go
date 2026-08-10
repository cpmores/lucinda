package taskpostman

import (
	"encoding/json"
	"testing"

	APIEvent "github.com/cpmores/lucinda/api/v1/messaging/event"
	APITaskmsg "github.com/cpmores/lucinda/api/v1/messaging/taskmsg"
)

// TestDecodeReTypesWireBody verifies that a payload which crossed the libp2p
// wire as a JSON object (map[string]any) is re-typed into the concrete
// message the local consumers assert on.
func TestDecodeReTypesWireBody(t *testing.T) {
	wireBody, err := json.Marshal(APITaskmsg.TaskTracedMsg{
		TaskID: "t-9", State: "done", Output: "decoded", Owner: "node-A",
	})
	if err != nil {
		t.Fatal(err)
	}
	var asMap map[string]any
	if err := json.Unmarshal(wireBody, &asMap); err != nil {
		t.Fatal(err)
	}

	out := decode(APIEvent.TaskTraced, asMap)
	msg, ok := out.(APITaskmsg.TaskTracedMsg)
	if !ok {
		t.Fatalf("decode returned %T, want TaskTracedMsg", out)
	}
	if msg.Output != "decoded" || msg.TaskID != "t-9" {
		t.Fatalf("bad decode: %+v", msg)
	}
}
