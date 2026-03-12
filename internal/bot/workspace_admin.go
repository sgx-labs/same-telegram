package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// handleUpdateCommand processes /update — admin-only command to update all
// workspace machines to a new Docker image in-place, preserving volumes.
//
// Usage:
//
//	/update                — update all machines to the configured FlyImage
//	/update <image>        — update all machines to a specific image
//	/update <machineID> <image> — update a single machine
func (b *Bot) handleUpdateCommand(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	// Admin-only.
	ownerID := b.cfg.Bot.EffectiveOwnerID()
	if ownerID != 0 && msg.From.ID != ownerID {
		b.sendMarkdown(chatID, "This command is only available to the admin.")
		return
	}

	if b.orchestrator == nil {
		b.sendMarkdown(chatID, "Workspace mode is not enabled.")
		return
	}

	args := strings.TrimSpace(msg.CommandArguments())
	parts := strings.Fields(args)

	switch len(parts) {
	case 0:
		// Update all machines to the configured image.
		b.updateAllMachines(chatID, b.cfg.Bot.FlyImage)

	case 1:
		// Could be a machine ID or a new image tag.
		// If it contains "/" or ":", treat it as an image name.
		arg := parts[0]
		if strings.Contains(arg, "/") || strings.Contains(arg, ":") {
			b.updateAllMachines(chatID, arg)
		} else {
			b.sendMarkdown(chatID, "Usage:\n"+
				"`/update` — update all to configured image\n"+
				"`/update <image>` — update all to specific image\n"+
				"`/update <machine_id> <image>` — update one machine")
		}

	case 2:
		// Update a single machine.
		machineID := parts[0]
		newImage := parts[1]
		b.updateSingleMachine(chatID, machineID, newImage)

	default:
		b.sendMarkdown(chatID, "Usage:\n"+
			"`/update` — update all to configured image\n"+
			"`/update <image>` — update all to specific image\n"+
			"`/update <machine_id> <image>` — update one machine")
	}
}

// updateAllMachines updates every workspace machine to a new image.
func (b *Bot) updateAllMachines(chatID int64, newImage string) {
	if newImage == "" {
		b.sendMarkdown(chatID, "No image specified and no default image configured. "+
			"Usage: `/update registry.fly.io/app:tag`")
		return
	}

	b.sendMarkdown(chatID, fmt.Sprintf("Updating all workspaces to `%s`...\n\n"+
		"This may take a few minutes. Machines will be stopped, updated, and restarted.", newImage))

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		results := b.orchestrator.UpdateAllImages(ctx, newImage)

		var succeeded, failed int
		var sb strings.Builder
		sb.WriteString("*Update Results*\n\n")

		for machineID, err := range results {
			if machineID == "_error" {
				sb.WriteString(fmt.Sprintf("Failed to list machines: %v\n", err))
				continue
			}
			if err != nil {
				failed++
				sb.WriteString(fmt.Sprintf("  `%s` — failed: %v\n", truncateID(machineID), err))
			} else {
				succeeded++
				sb.WriteString(fmt.Sprintf("  `%s` — updated\n", truncateID(machineID)))
			}
		}

		sb.WriteString(fmt.Sprintf("\n*%d* updated, *%d* failed", succeeded, failed))
		b.sendMarkdown(chatID, sb.String())
	}()
}

// updateSingleMachine updates one workspace machine to a new image.
func (b *Bot) updateSingleMachine(chatID int64, machineID, newImage string) {
	b.sendMarkdown(chatID, fmt.Sprintf("Updating machine `%s` to `%s`...", truncateID(machineID), newImage))

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		if err := b.orchestrator.UpdateImage(ctx, machineID, newImage); err != nil {
			b.sendMarkdown(chatID, fmt.Sprintf("Update failed for `%s`: %v", truncateID(machineID), err))
			return
		}

		b.sendMarkdown(chatID, fmt.Sprintf("Machine `%s` updated to `%s`.", truncateID(machineID), newImage))
	}()
}

// handleListMachinesCommand processes /machines — admin-only list of all workspace machines.
func (b *Bot) handleListMachinesCommand(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	ownerID := b.cfg.Bot.EffectiveOwnerID()
	if ownerID != 0 && msg.From.ID != ownerID {
		b.sendMarkdown(chatID, "This command is only available to the admin.")
		return
	}

	if b.orchestrator == nil {
		b.sendMarkdown(chatID, "Workspace mode is not enabled.")
		return
	}

	if b.machineStore == nil {
		b.sendMarkdown(chatID, "Machine store is not available.")
		return
	}

	machines, err := b.machineStore.ListAllMachines()
	if err != nil {
		b.sendMarkdown(chatID, fmt.Sprintf("Failed to list machines: %v", err))
		return
	}

	if len(machines) == 0 {
		b.sendMarkdown(chatID, "No workspace machines found.")
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*Workspace Machines* (%d)\n\n", len(machines)))
	for _, m := range machines {
		userLabel := m.UserID
		if uid, err := strconv.ParseInt(m.UserID, 10, 64); err == nil {
			userLabel = fmt.Sprintf("%d", uid)
		}
		sb.WriteString(fmt.Sprintf("`%s` — user %s — %s — last used %s\n",
			truncateID(m.MachineID), userLabel, m.State,
			m.LastUsed.Format("Jan 2 15:04")))
	}

	b.sendMarkdown(chatID, sb.String())
}

// truncateID shortens a Fly machine ID for display (first 12 chars).
func truncateID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
