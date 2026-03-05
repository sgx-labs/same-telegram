package notify

import "time"

// Notification types sent from hook -> daemon via unix socket.
const (
	TypeSessionEnd  = "session_end"
	TypeDecision    = "decision"
	TypeHandoff     = "handoff"
	TypeDigestReq   = "digest_request"
	TypeCustom      = "custom"
)

// Notification is the wire format for hook -> daemon messages over the unix socket.
type Notification struct {
	Type      string            `json:"type"`
	Timestamp time.Time         `json:"timestamp"`
	SessionID string            `json:"session_id,omitempty"`
	Summary   string            `json:"summary,omitempty"`
	Details   string            `json:"details,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// Response is sent back from daemon to hook over the unix socket.
type Response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}
