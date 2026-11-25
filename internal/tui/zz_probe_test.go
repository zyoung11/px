package tui

import (
	"encoding/json"
	"testing"

	"px/internal/ipc"
)

func TestProbeDaemon(tb *testing.T) {
	resp, err := ipc.Command(ipc.Request{Command: ipc.CmdStatus})
	if err != nil {
		tb.Fatal(err)
	}
	var st map[string]any
	if err := json.Unmarshal(resp.Data, &st); err != nil {
		tb.Fatal(err)
	}
	raw, _ := json.Marshal(st["browse"])
	tb.Logf("page = %v align = %v", st["page"], st["align"])
	tb.Logf("browse = %s", raw)
}
