package protocol

import "encoding/json"

// Frame is the JSON envelope exchanged by center and agents.
type Frame struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	TS      string          `json:"ts"`
	Payload json.RawMessage `json:"payload,omitempty"`
}
