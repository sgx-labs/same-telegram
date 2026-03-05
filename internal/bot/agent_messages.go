package bot

import (
	"fmt"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/sgx-labs/same-telegram/internal/msgbox"
)

// pendingReply tracks which agent message a CEO reply should be attributed to.
type pendingReply struct {
	filename string
	agent    string
}

// replyTracker maps Telegram message IDs to pending agent messages.
type replyTracker struct {
	mu      sync.Mutex
	pending map[int]pendingReply // Telegram message ID -> pending reply info
}

func newReplyTracker() *replyTracker {
	return &replyTracker{
		pending: make(map[int]pendingReply),
	}
}

func (rt *replyTracker) track(msgID int, filename, agent string) {
	rt.mu.Lock()
	rt.pending[msgID] = pendingReply{filename: filename, agent: agent}
	rt.mu.Unlock()
}

func (rt *replyTracker) pop(replyToMsgID int) (pendingReply, bool) {
	rt.mu.Lock()
	pr, ok := rt.pending[replyToMsgID]
	if ok {
		delete(rt.pending, replyToMsgID)
	}
	rt.mu.Unlock()
	return pr, ok
}

// SendAgentMessage sends an agent's message to all allowed Telegram users.
// Returns the Telegram message ID for reply tracking.
func (b *Bot) SendAgentMessage(msg *msgbox.Message, filename string) {
	text := formatAgentMessage(msg)
	text = b.filter.Sanitize(text)

	for userID := range b.allowedUsers {
		tgMsg := tgbotapi.NewMessage(userID, text)
		tgMsg.ParseMode = "Markdown"
		sent, err := b.api.Send(tgMsg)
		if err != nil {
			b.logger.Printf("Failed to send agent message to %d: %v", userID, err)
			continue
		}
		b.replies.track(sent.MessageID, filename, msg.From)
	}
}

// HandleReply checks if an incoming message is a reply to an agent message
// and writes the CEO response to the inbound directory.
func (b *Bot) HandleReply(msg *tgbotapi.Message) bool {
	if msg.ReplyToMessage == nil {
		return false
	}

	pr, ok := b.replies.pop(msg.ReplyToMessage.MessageID)
	if !ok {
		return false
	}

	reply := &msgbox.Reply{
		To:        pr.agent,
		InReplyTo: pr.filename,
		Body:      msg.Text,
		Timestamp: msg.Time(),
	}

	if err := msgbox.WriteReply(reply); err != nil {
		b.logger.Printf("Failed to write reply: %v", err)
		b.sendMarkdown(msg.Chat.ID, fmt.Sprintf("Failed to save reply: %s", err))
		return true
	}

	b.sendMarkdown(msg.Chat.ID, fmt.Sprintf("Reply saved for agent *%s*", escapeMarkdown(pr.agent)))
	return true
}

func formatAgentMessage(msg *msgbox.Message) string {
	var b strings.Builder

	// Type-specific emoji and label
	emoji := "📨"
	label := "Message"
	switch msg.Type {
	case msgbox.MsgTypeQuestion:
		emoji = "❓"
		label = "Question"
	case msgbox.MsgTypeStatus:
		emoji = "📊"
		label = "Status Update"
	case msgbox.MsgTypeBlocker:
		emoji = "🚨"
		label = "Blocker"
	}

	b.WriteString(fmt.Sprintf("%s *Agent %s from %s*\n", emoji, label, escapeMarkdown(msg.From)))
	if msg.Subject != "" {
		b.WriteString(fmt.Sprintf("\n*Subject:* %s\n", escapeMarkdown(msg.Subject)))
	}
	b.WriteString(fmt.Sprintf("\n%s", escapeMarkdown(msg.Body)))
	b.WriteString("\n\n_Reply to this message to respond to the agent._")
	return b.String()
}
