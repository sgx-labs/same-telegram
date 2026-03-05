package notify

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNotificationJSON(t *testing.T) {
	n := Notification{
		Type:      TypeSessionEnd,
		Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		SessionID: "test-123",
		Summary:   "Session completed",
		Details:   "Worked on feature X",
	}

	data, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded Notification
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Type != TypeSessionEnd {
		t.Errorf("Type = %q, want %q", decoded.Type, TypeSessionEnd)
	}
	if decoded.SessionID != "test-123" {
		t.Errorf("SessionID = %q, want test-123", decoded.SessionID)
	}
}

func TestResponseJSON(t *testing.T) {
	r := Response{OK: true}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded Response
	json.Unmarshal(data, &decoded)
	if !decoded.OK {
		t.Error("Expected OK=true")
	}
}
