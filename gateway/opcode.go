package gateway

// Opcode identifies the kind of a gateway frame. It is carried in the "op"
// field of every envelope and determines how the "d" payload is interpreted.
type Opcode int

const (
	// OpDispatch (receive) delivers an event. The envelope carries the event
	// name in "t" and a monotonically increasing sequence number in "s".
	OpDispatch Opcode = 0
	// OpHeartbeat (send/receive) keeps the connection alive. The client sends
	// it with the last sequence number; the server may send it to request an
	// immediate heartbeat.
	OpHeartbeat Opcode = 1
	// OpIdentify (send) authenticates a new session and declares the intents
	// the client wishes to receive.
	OpIdentify Opcode = 2
	// OpResume (send) replays missed events on a previously established session
	// after a reconnect.
	OpResume Opcode = 6
	// OpReconnect (receive) instructs the client to reconnect and resume.
	OpReconnect Opcode = 7
	// OpInvalidSession (receive) signals that the session is invalid. The
	// boolean payload reports whether the session may still be resumed.
	OpInvalidSession Opcode = 9
	// OpHello (receive) is the first frame from the server and carries the
	// heartbeat interval.
	OpHello Opcode = 10
	// OpHeartbeatACK (receive) acknowledges a client heartbeat.
	OpHeartbeatACK Opcode = 11
)

// String returns a human-readable name for the opcode.
func (o Opcode) String() string {
	switch o {
	case OpDispatch:
		return "Dispatch"
	case OpHeartbeat:
		return "Heartbeat"
	case OpIdentify:
		return "Identify"
	case OpResume:
		return "Resume"
	case OpReconnect:
		return "Reconnect"
	case OpInvalidSession:
		return "InvalidSession"
	case OpHello:
		return "Hello"
	case OpHeartbeatACK:
		return "HeartbeatACK"
	default:
		return "Opcode(" + itoa(int(o)) + ")"
	}
}

// itoa is a tiny strconv.Itoa to keep this file import-free.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
