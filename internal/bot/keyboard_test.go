package bot

import (
	"strings"
	"testing"
)

func callbackData(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func TestApproveDecisionKeyboard(t *testing.T) {
	kb := ApproveDecisionKeyboard("decision-abc")

	if len(kb.InlineKeyboard) != 1 {
		t.Fatalf("Expected 1 row, got %d", len(kb.InlineKeyboard))
	}
	row := kb.InlineKeyboard[0]
	if len(row) != 2 {
		t.Fatalf("Expected 2 buttons, got %d", len(row))
	}

	approveData := callbackData(row[0].CallbackData)
	if !strings.HasPrefix(approveData, "approve:") {
		t.Errorf("Approve button callback data = %q, should start with 'approve:'", approveData)
	}
	if !strings.HasSuffix(approveData, "decision-abc") {
		t.Errorf("Approve button callback data = %q, want suffix 'decision-abc'", approveData)
	}

	noteData := callbackData(row[1].CallbackData)
	if !strings.HasPrefix(noteData, "note:") {
		t.Errorf("Note button callback data = %q, should start with 'note:'", noteData)
	}
	if !strings.HasSuffix(noteData, "decision-abc") {
		t.Errorf("Note button callback data = %q, want suffix 'decision-abc'", noteData)
	}
}

func TestApproveDecisionKeyboardEmptyID(t *testing.T) {
	kb := ApproveDecisionKeyboard("")
	if len(kb.InlineKeyboard) != 1 {
		t.Fatalf("Expected 1 row, got %d", len(kb.InlineKeyboard))
	}
	if len(kb.InlineKeyboard[0]) != 2 {
		t.Fatalf("Expected 2 buttons, got %d", len(kb.InlineKeyboard[0]))
	}
}

func TestConfirmKeyboard(t *testing.T) {
	kb := ConfirmKeyboard("delete-vault")

	if len(kb.InlineKeyboard) != 1 {
		t.Fatalf("Expected 1 row, got %d", len(kb.InlineKeyboard))
	}
	row := kb.InlineKeyboard[0]
	if len(row) != 2 {
		t.Fatalf("Expected 2 buttons, got %d", len(row))
	}

	yesData := callbackData(row[0].CallbackData)
	if !strings.HasPrefix(yesData, "confirm:") {
		t.Errorf("Yes button callback data = %q, should start with 'confirm:'", yesData)
	}
	if !strings.HasSuffix(yesData, "delete-vault") {
		t.Errorf("Yes button callback data = %q, want suffix 'delete-vault'", yesData)
	}

	noData := callbackData(row[1].CallbackData)
	if !strings.HasPrefix(noData, "cancel:") {
		t.Errorf("No button callback data = %q, should start with 'cancel:'", noData)
	}
}

func TestVaultListKeyboard(t *testing.T) {
	vaults := map[string]string{
		"work":     "/home/user/.same/work",
		"personal": "/home/user/.same/personal",
	}
	kb := VaultListKeyboard(vaults, "work")

	if len(kb.InlineKeyboard) != len(vaults) {
		t.Errorf("Expected %d rows, got %d", len(vaults), len(kb.InlineKeyboard))
	}

	foundCurrent := false
	for _, row := range kb.InlineKeyboard {
		if len(row) != 1 {
			t.Errorf("Expected 1 button per row, got %d", len(row))
		}
		btn := row[0]
		if strings.HasPrefix(btn.Text, "• ") && strings.Contains(btn.Text, "work") {
			foundCurrent = true
		}
		data := callbackData(btn.CallbackData)
		if !strings.HasPrefix(data, "vault:") {
			t.Errorf("Vault button callback data = %q, should start with 'vault:'", data)
		}
	}
	if !foundCurrent {
		t.Error("Current vault 'work' should be marked with '• ' prefix")
	}
}

func TestVaultListKeyboardEmpty(t *testing.T) {
	kb := VaultListKeyboard(map[string]string{}, "")
	if len(kb.InlineKeyboard) != 0 {
		t.Errorf("Expected empty keyboard for empty vault list, got %d rows", len(kb.InlineKeyboard))
	}
}

func TestVaultListKeyboardNoCurrent(t *testing.T) {
	vaults := map[string]string{
		"work": "/home/user/.same/work",
	}
	kb := VaultListKeyboard(vaults, "nonexistent")

	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if strings.HasPrefix(btn.Text, "• ") {
				t.Errorf("No button should be marked as current, but got: %q", btn.Text)
			}
		}
	}
}
