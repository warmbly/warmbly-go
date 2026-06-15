package gateway

import "encoding/json"

// envelope is the wire frame exchanged with the gateway. Every message, in
// either direction, is an envelope: an opcode, an optional opcode-specific
// payload, and — for dispatched events — a sequence number and event name.
type envelope struct {
	Op   Opcode          `json:"op"`
	Data json.RawMessage `json:"d,omitempty"`
	Seq  *int            `json:"s,omitempty"`
	Type string          `json:"t,omitempty"`
}

// helloData is the [OpHello] payload.
type helloData struct {
	// HeartbeatInterval is the heartbeat period in milliseconds.
	HeartbeatInterval int `json:"heartbeat_interval"`
}

// identifyData is the [OpIdentify] payload.
type identifyData struct {
	Token      string     `json:"token"`
	Intents    Intent     `json:"intents"`
	Properties properties `json:"properties"`
}

// resumeData is the [OpResume] payload.
type resumeData struct {
	Token     string `json:"token"`
	SessionID string `json:"session_id"`
	Seq       int    `json:"seq"`
}

// properties describes the connecting client; sent during identify for
// observability.
type properties struct {
	OS      string `json:"os"`
	Lib     string `json:"lib"`
	Version string `json:"version"`
}
