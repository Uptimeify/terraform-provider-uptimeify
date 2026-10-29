package provider

import (
	"encoding/json"
	"testing"
)

func TestRawJSONOrNull(t *testing.T) {
	if !rawJSONOrNull(nil).IsNull() || !rawJSONOrNull(json.RawMessage("null")).IsNull() {
		t.Fatal("absent and JSON null must be null")
	}
	if got := rawJSONOrNull(json.RawMessage(`{"propagation":"propagating"}`)).ValueString(); got != `{"propagation":"propagating"}` {
		t.Fatalf("got %q", got)
	}
}
