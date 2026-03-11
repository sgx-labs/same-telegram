package machines

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

// Orchestrator manages the lifecycle of per-user workspace machines.
// It sits between the Telegram bot and the Fly Machines API, handling
// the "create on first use, wake on reconnect, hibernate on idle" flow.
type Orchestrator struct {
	client *Client
	store  MachineStore

	mu       sync.Mutex
	starting map[string]bool // userID → currently starting
}

// MachineStore persists the mapping between users and their machines.
// Implemented by the bot's SQLite store.
type MachineStore interface {
	GetUserMachine(userID string) (*UserMachine, error)
	SaveUserMachine(m *UserMachine) error
	DeleteUserMachine(userID string) error
}

// UserMachine tracks a user's workspace machine.
type UserMachine struct {
	UserID    string
	MachineID string
	VolumeID  string
	Region    string
	State     string // "provisioning", "running", "stopped", "suspended"
	Token     string // workspace auth token for WebSocket connections
	CreatedAt time.Time
	LastUsed  time.Time
}

// NewOrchestrator creates a machine orchestrator.
func NewOrchestrator(client *Client, store MachineStore) *Orchestrator {
	return &Orchestrator{
		client:   client,
		store:    store,
		starting: make(map[string]bool),
	}
}

// EnsureRunning guarantees a user has a running workspace machine.
// It creates one if none exists, starts it if stopped, or returns
// the existing running machine. This is the main entry point.
//
// Returns the machine ID, auth token, and a log-only WebSocket URL.
func (o *Orchestrator) EnsureRunning(ctx context.Context, userID string) (machineID, token string, err error) {
	// Prevent concurrent starts for the same user.
	o.mu.Lock()
	if o.starting[userID] {
		o.mu.Unlock()
		log.Printf("EnsureRunning: user %s already has a start in progress, skipping", userID)
		return "", "", fmt.Errorf("your workspace is already starting up — give it a moment")
	}
	o.starting[userID] = true
	o.mu.Unlock()
	defer func() {
		o.mu.Lock()
		delete(o.starting, userID)
		o.mu.Unlock()
	}()

	// Check if user already has a machine.
	log.Printf("EnsureRunning: checking existing workspace for user %s", userID)
	um, err := o.store.GetUserMachine(userID)
	if err != nil {
		return "", "", fmt.Errorf("could not check your workspace status: %w", err)
	}

	if um == nil {
		// First time — provision a new machine.
		log.Printf("EnsureRunning: no existing machine for user %s, creating new workspace", userID)
		um, err = o.provision(ctx, userID)
		if err != nil {
			return "", "", err
		}
	} else {
		// Machine exists — make sure it's running.
		log.Printf("EnsureRunning: machine %s found for user %s in state %q, ensuring started", um.MachineID, userID, um.State)
		err = o.ensureStarted(ctx, um)
		if err != nil {
			return "", "", err
		}
	}

	// Update last-used timestamp.
	um.LastUsed = time.Now()
	if err := o.store.SaveUserMachine(um); err != nil {
		log.Printf("warning: could not update last-used time for user %s: %v", userID, err)
	}

	log.Printf("EnsureRunning: workspace ready for user %s, machine=%s", userID, um.MachineID)
	return um.MachineID, um.Token, nil
}

// provision creates a new Fly Machine and volume for a user.
func (o *Orchestrator) provision(ctx context.Context, userID string) (*UserMachine, error) {
	log.Printf("provisioning workspace for user %s", userID)

	// Generate a workspace auth token. This token authenticates
	// WebSocket connections from the Mini App to this machine.
	// Generated before machine creation so it can be injected as an env var.
	token := generateToken()

	machine, err := o.client.CreateMachine(ctx, userID, token)
	if err != nil {
		return nil, err
	}

	// Wait for the machine to be fully started before returning.
	// This prevents "machine not running" errors when we try to exec into it.
	log.Printf("waiting for machine %s to start for user %s", machine.ID, userID)
	if err := o.client.WaitForState(ctx, machine.ID, "started", 60*time.Second); err != nil {
		log.Printf("warning: machine %s may not be fully started for user %s: %v", machine.ID, userID, err)
	}

	um := &UserMachine{
		UserID:    userID,
		MachineID: machine.ID,
		Region:    machine.Region,
		State:     "running",
		Token:     token,
		CreatedAt: time.Now(),
		LastUsed:  time.Now(),
	}

	// Extract volume ID if mounted.
	if machine.Config != nil && len(machine.Config.Mounts) > 0 {
		um.VolumeID = machine.Config.Mounts[0].Volume
	}

	if err := o.store.SaveUserMachine(um); err != nil {
		// Machine was created but we couldn't save the record.
		// The user will not be able to reconnect to this machine on next /start
		// because the store has no record of it. Log clearly so it can be investigated.
		log.Printf("ERROR: machine %s created for user %s but failed to save record: %v", machine.ID, userID, err)
	}

	log.Printf("workspace provisioned for user %s: machine=%s region=%s", userID, machine.ID, machine.Region)
	return um, nil
}

// ensureStarted checks the machine state and starts it if needed.
func (o *Orchestrator) ensureStarted(ctx context.Context, um *UserMachine) error {
	machine, err := o.client.GetMachine(ctx, um.MachineID)
	if err != nil {
		// If the machine is gone (404), re-provision.
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
			log.Printf("workspace for user %s not found (machine %s gone), re-provisioning", um.UserID, um.MachineID)
			newUM, provErr := o.provision(ctx, um.UserID)
			if provErr != nil {
				return provErr
			}
			*um = *newUM
			return nil
		}
		return err
	}

	switch machine.State {
	case "started":
		// Already running.
		um.State = "running"
		return nil

	case "stopped", "suspended":
		log.Printf("waking workspace for user %s (machine %s was %s)", um.UserID, um.MachineID, machine.State)
		if err := o.client.StartMachine(ctx, um.MachineID); err != nil {
			return err
		}
		if err := o.client.WaitForState(ctx, um.MachineID, "started", 60*time.Second); err != nil {
			log.Printf("warning: machine %s may not be fully started for user %s: %v", um.MachineID, um.UserID, err)
		}
		um.State = "running"
		return nil

	case "destroyed":
		// Machine was destroyed (maybe manually). Re-provision.
		log.Printf("workspace for user %s was destroyed, re-provisioning", um.UserID)
		newUM, err := o.provision(ctx, um.UserID)
		if err != nil {
			return err
		}
		*um = *newUM
		return nil

	default:
		return fmt.Errorf("your workspace is in an unexpected state (%s) — contact support", machine.State)
	}
}

// Stop gracefully stops a user's workspace.
func (o *Orchestrator) Stop(ctx context.Context, userID string) error {
	um, err := o.store.GetUserMachine(userID)
	if err != nil {
		return fmt.Errorf("could not find your workspace: %w", err)
	}
	if um == nil {
		return fmt.Errorf("you don't have a workspace yet — use /start to create one")
	}

	if err := o.client.StopMachine(ctx, um.MachineID); err != nil {
		return err
	}

	um.State = "stopped"
	o.store.SaveUserMachine(um)
	return nil
}

// Status returns the current state of a user's workspace.
func (o *Orchestrator) Status(ctx context.Context, userID string) (*UserMachine, error) {
	um, err := o.store.GetUserMachine(userID)
	if err != nil {
		return nil, fmt.Errorf("could not check your workspace: %w", err)
	}
	if um == nil {
		return nil, nil // no workspace yet
	}

	// Refresh state from Fly API.
	machine, err := o.client.GetMachine(ctx, um.MachineID)
	if err != nil {
		// If 404, the machine is gone — return nil so /start re-provisions.
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
			return nil, nil
		}
		return um, nil // return cached state if API is otherwise down
	}

	um.State = machine.State
	return um, nil
}

// Destroy permanently removes a user's workspace machine and volume.
// The machine record is deleted from the store so /start will re-provision.
func (o *Orchestrator) Destroy(ctx context.Context, userID string) error {
	um, err := o.store.GetUserMachine(userID)
	if err != nil {
		return fmt.Errorf("could not find your workspace: %w", err)
	}
	if um == nil {
		return fmt.Errorf("no workspace found")
	}

	// Stop the machine first (ignore errors — it might already be stopped/gone).
	_ = o.client.StopMachine(ctx, um.MachineID)

	// Destroy the machine.
	log.Printf("destroying workspace machine %s for user %s", um.MachineID, userID)
	if err := o.client.DestroyMachine(ctx, um.MachineID); err != nil {
		// If 404, it's already gone — that's fine.
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != 404 {
			return fmt.Errorf("could not destroy workspace: %w", err)
		}
		log.Printf("machine %s already gone (404), continuing cleanup", um.MachineID)
	}

	// Delete the volume if we have one.
	if um.VolumeID != "" {
		log.Printf("destroying volume %s for user %s", um.VolumeID, userID)
		if err := o.client.DestroyVolume(ctx, um.VolumeID); err != nil {
			log.Printf("warning: could not destroy volume %s: %v", um.VolumeID, err)
		}
	}

	// Remove the machine record from the store.
	if err := o.store.DeleteUserMachine(userID); err != nil {
		log.Printf("warning: could not delete machine record for user %s: %v", userID, err)
	}

	log.Printf("workspace fully destroyed for user %s", userID)
	return nil
}

// SeedVault runs a seed script inside the user's workspace container.
// seedType is the vault seed template (e.g., "research", "project", "same-demo").
// topic is an optional user-provided topic or project description.
// validSeedTypes is the set of accepted seed vault templates.
var validSeedTypes = map[string]bool{
	"same-demo": true,
	"research":  true,
	"project":   true,
	"bot-dev":   true,
	"empty":     true,
}

func (o *Orchestrator) SeedVault(ctx context.Context, userID string, seedType string, topic string) error {
	if seedType == "" || seedType == "empty" {
		// Nothing to seed for empty workspaces.
		return nil
	}

	if !validSeedTypes[seedType] {
		return fmt.Errorf("invalid seed type: %q", seedType)
	}

	um, err := o.store.GetUserMachine(userID)
	if err != nil {
		return fmt.Errorf("could not look up workspace for seeding: %w", err)
	}
	if um == nil {
		return fmt.Errorf("no workspace found for user %s", userID)
	}

	cmd := []string{"bash", "-c", fmt.Sprintf("/workspace/seeds/seed-vault.sh %q %q", seedType, topic)}

	log.Printf("seeding vault for user %s: type=%s topic=%q", userID, seedType, topic)
	if err := o.client.ExecCommand(ctx, um.MachineID, cmd); err != nil {
		return fmt.Errorf("vault seed exec failed: %w", err)
	}

	log.Printf("vault seeding completed for user %s", userID)
	return nil
}

// InjectAPIKey writes an API key into a running workspace so that CLI tools
// (Claude Code, etc.) can use it immediately. The key is:
//   - Written to /data/.env (persistent volume — survives restarts)
//   - Set in the tmux global environment (available to new shells immediately)
//
// The env file is sourced by startup.sh on boot and by .bashrc on every new shell.
func (o *Orchestrator) InjectAPIKey(ctx context.Context, userID, envName, apiKey string) error {
	um, err := o.store.GetUserMachine(userID)
	if err != nil || um == nil {
		return fmt.Errorf("no workspace found for user %s", userID)
	}

	// Use bash printf %q for safe shell quoting of the key value,
	// and write to the persistent env file. Also set in tmux so
	// current sessions pick it up without restart.
	script := fmt.Sprintf(
		`printf 'export %%s=%%q\n' %q %q > /data/.env && tmux set-environment -g %q %q 2>/dev/null; true`,
		envName, apiKey, envName, apiKey,
	)

	cmd := []string{"bash", "-c", script}
	log.Printf("injecting %s into workspace for user %s", envName, userID)
	if err := o.client.ExecCommand(ctx, um.MachineID, cmd); err != nil {
		return fmt.Errorf("could not inject API key: %w", err)
	}
	return nil
}

// generateToken creates a cryptographically random auth token for WebSocket connections.
func generateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// Fall back to less random but still functional token.
		// This should never happen in practice.
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
