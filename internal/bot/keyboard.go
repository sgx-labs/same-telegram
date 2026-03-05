package bot

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

// ApproveDecisionKeyboard creates an inline keyboard for decision approval.
func ApproveDecisionKeyboard(decisionID string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Approve", "approve:"+decisionID),
			tgbotapi.NewInlineKeyboardButtonData("📝 Add Note", "note:"+decisionID),
		),
	)
}

// ConfirmKeyboard creates a simple yes/no inline keyboard.
func ConfirmKeyboard(action string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Yes", "confirm:"+action),
			tgbotapi.NewInlineKeyboardButtonData("No", "cancel:"+action),
		),
	)
}

// VaultListKeyboard creates a keyboard with vault names as buttons.
func VaultListKeyboard(vaults map[string]string, current string) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	for name := range vaults {
		label := name
		if name == current {
			label = "• " + name
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, "vault:"+name),
		))
	}
	return tgbotapi.InlineKeyboardMarkup{InlineKeyboard: rows}
}
