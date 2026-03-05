package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/sgx-labs/same-telegram/internal/msgbox"
)

var (
	msgFrom    string
	msgType    string
	msgSubject string
	msgBody    string
)

var messageCmd = &cobra.Command{
	Use:   "message",
	Short: "Send a message to the CEO via Telegram",
	Long: `Writes a JSON message to the outbound directory for the daemon to pick up
and send to the CEO via Telegram.

Example:
  same-telegram message --from backend-dev --type question --subject "Redis?" --body "Can we use Redis for caching?"`,
	RunE: runMessage,
}

func init() {
	messageCmd.Flags().StringVar(&msgFrom, "from", "", "Agent name (required)")
	messageCmd.Flags().StringVar(&msgType, "type", "question", "Message type: question, status, blocker")
	messageCmd.Flags().StringVar(&msgSubject, "subject", "", "Short summary (required)")
	messageCmd.Flags().StringVar(&msgBody, "body", "", "Full message body (required)")

	messageCmd.MarkFlagRequired("from")
	messageCmd.MarkFlagRequired("subject")
	messageCmd.MarkFlagRequired("body")
}

func runMessage(cmd *cobra.Command, args []string) error {
	// Validate type
	switch msgType {
	case msgbox.MsgTypeQuestion, msgbox.MsgTypeStatus, msgbox.MsgTypeBlocker:
		// valid
	default:
		return fmt.Errorf("invalid message type %q: must be question, status, or blocker", msgType)
	}

	msg := &msgbox.Message{
		From:      msgFrom,
		Type:      msgType,
		Subject:   msgSubject,
		Body:      msgBody,
		Timestamp: time.Now(),
	}

	filename, err := msgbox.WriteMessage(msg)
	if err != nil {
		return fmt.Errorf("write message: %w", err)
	}

	fmt.Printf("Message sent: %s\n", filename)
	fmt.Printf("  From:    %s\n", msg.From)
	fmt.Printf("  Type:    %s\n", msg.Type)
	fmt.Printf("  Subject: %s\n", msg.Subject)
	fmt.Println("The daemon will pick this up and forward to Telegram.")
	return nil
}
