package protocol_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/protocol"
)

func TestFrameJSONRoundTrip(t *testing.T) {
	original := protocol.Frame{
		Type:    "status_summary",
		ID:      "frame-1",
		TS:      "2026-08-08T12:00:00Z",
		Payload: json.RawMessage(`{"online":true,"peers":3}`),
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded protocol.Frame
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Fatalf("decoded frame = %#v, want %#v", decoded, original)
	}
}
