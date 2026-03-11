// Package machines provides a client for the Fly Machines API.
//
// It handles per-user machine lifecycle: create, start, stop, hibernate,
// and destroy. Each user gets an isolated Fly Machine with their own
// vault volume, workspace server, and AI tools.
//
// The client is intentionally thin — just enough to orchestrate machines,
// easy to swap for a different provider later (Hetzner, AWS, etc.).
package machines

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultBaseURL = "https://api.machines.dev/v1"
	defaultTimeout = 30 * time.Second
)

// Client talks to the Fly Machines REST API.
type Client struct {
	AppName  string // Fly app name (e.g., "samevault-workspaces")
	APIToken string // Fly API token (from `fly tokens deploy`)
	Image    string // Docker image for workspace containers
	Region   string // Default region (e.g., "dfw")
	BaseURL  string // API base URL (default: https://api.machines.dev/v1)

	http *http.Client
}

// NewClient creates a Fly Machines API client.
func NewClient(appName, apiToken, image, region string) *Client {
	return &Client{
		AppName: appName,
		APIToken: apiToken,
		Image:   image,
		Region:  region,
		BaseURL: defaultBaseURL,
		http:    &http.Client{Timeout: defaultTimeout},
	}
}

// Machine represents a Fly Machine instance.
type Machine struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	State     string         `json:"state"` // created, started, stopped, suspended, destroyed
	Region    string         `json:"region"`
	PrivateIP string         `json:"private_ip"`
	Config    *MachineConfig `json:"config,omitempty"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at"`
}

// MachineConfig defines what runs inside the machine.
type MachineConfig struct {
	Image    string            `json:"image"`
	Env      map[string]string `json:"env,omitempty"`
	Guest    *GuestConfig      `json:"guest,omitempty"`
	Services []ServiceConfig   `json:"services,omitempty"`
	Mounts   []MountConfig     `json:"mounts,omitempty"`
	Restart  *RestartConfig    `json:"restart,omitempty"`
	AutoStop *AutoStopConfig   `json:"auto_stop,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// GuestConfig sets machine resources.
type GuestConfig struct {
	CPUKind  string `json:"cpu_kind"`  // "shared" or "performance"
	CPUs     int    `json:"cpus"`
	MemoryMB int    `json:"memory_mb"`
}

// ServiceConfig exposes a port through Fly's proxy.
type ServiceConfig struct {
	Protocol     string       `json:"protocol"`
	InternalPort int          `json:"internal_port"`
	Ports        []PortConfig `json:"ports"`
}

// PortConfig defines an external port mapping.
type PortConfig struct {
	Port     int      `json:"port"`
	Handlers []string `json:"handlers"`
}

// MountConfig attaches a persistent volume.
type MountConfig struct {
	Volume string `json:"volume"`
	Path   string `json:"path"`
}

// RestartConfig controls restart behavior.
type RestartConfig struct {
	Policy string `json:"policy"` // "no", "on-failure", "always"
}

// AutoStopConfig controls automatic hibernation.
type AutoStopConfig struct {
	Enabled bool   `json:"enabled"`
	Signal  string `json:"signal,omitempty"`
}

// Volume represents a persistent Fly volume.
type Volume struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Region string `json:"region"`
	SizeGB int    `json:"size_gb"`
	State  string `json:"state"`
}

// --- Machine Lifecycle ---

// CreateMachine provisions a new machine for a user.
// It creates a volume, then a machine with the volume attached.
// The token is set as WORKSPACE_TOKEN so the workspace-server can authenticate connections.
func (c *Client) CreateMachine(ctx context.Context, userID, token string) (*Machine, error) {
	// Create a persistent volume for the user's vault data.
	vol, err := c.CreateVolume(ctx, fmt.Sprintf("vault_%s", userID), 1)
	if err != nil {
		return nil, fmt.Errorf("could not create storage for your workspace: %w", err)
	}

	config := &MachineConfig{
		Image: c.Image,
		Env: map[string]string{
			"SAME_HOME":        "/data",
			"VAULT_PATH":       "/data/vault",
			"WORKSPACE_TOKEN":  token,
		},
		Guest: &GuestConfig{
			CPUKind:  "shared",
			CPUs:     1,
			MemoryMB: 256,
		},
		Services: []ServiceConfig{
			{
				Protocol:     "tcp",
				InternalPort: 8080,
				Ports: []PortConfig{
					{Port: 443, Handlers: []string{"tls", "http"}},
				},
			},
		},
		Mounts: []MountConfig{
			{Volume: vol.ID, Path: "/data"},
		},
		Restart: &RestartConfig{Policy: "on-failure"},
		Metadata: map[string]string{
			"user_id":    userID,
			"managed_by": "same-telegram",
		},
	}

	body := map[string]any{
		"name":   fmt.Sprintf("ws-%s", userID),
		"region": c.Region,
		"config": config,
	}

	var machine Machine
	if err := c.do(ctx, "POST", c.machinesURL(""), body, &machine); err != nil {
		return nil, fmt.Errorf("could not create your workspace: %w", err)
	}

	return &machine, nil
}

// WaitForState blocks until a machine reaches the desired state or ctx expires.
// Uses the Fly Machines /wait endpoint (server-side long poll).
func (c *Client) WaitForState(ctx context.Context, machineID, state string, timeout time.Duration) error {
	url := fmt.Sprintf("%s&timeout=%d", c.machinesURL("/"+machineID+"/wait?state="+state), int(timeout.Seconds()))
	if err := c.do(ctx, "GET", url, nil, nil); err != nil {
		return fmt.Errorf("timed out waiting for workspace to reach %s state: %w", state, err)
	}
	return nil
}

// StartMachine wakes a stopped/suspended machine.
func (c *Client) StartMachine(ctx context.Context, machineID string) error {
	if err := c.do(ctx, "POST", c.machinesURL("/"+machineID+"/start"), nil, nil); err != nil {
		return fmt.Errorf("could not start your workspace: %w", err)
	}
	return nil
}

// StopMachine gracefully stops a running machine. The machine can be restarted.
func (c *Client) StopMachine(ctx context.Context, machineID string) error {
	body := map[string]any{
		"signal": "SIGTERM",
	}
	if err := c.do(ctx, "POST", c.machinesURL("/"+machineID+"/stop"), body, nil); err != nil {
		return fmt.Errorf("could not stop your workspace: %w", err)
	}
	return nil
}

// SuspendMachine hibernates a machine (lower cost than stopped).
func (c *Client) SuspendMachine(ctx context.Context, machineID string) error {
	if err := c.do(ctx, "POST", c.machinesURL("/"+machineID+"/suspend"), nil, nil); err != nil {
		return fmt.Errorf("could not hibernate your workspace: %w", err)
	}
	return nil
}

// DestroyMachine permanently removes a machine. The volume is NOT deleted.
func (c *Client) DestroyMachine(ctx context.Context, machineID string) error {
	if err := c.do(ctx, "DELETE", c.machinesURL("/"+machineID), nil, nil); err != nil {
		return fmt.Errorf("could not remove your workspace: %w", err)
	}
	return nil
}

// GetMachine returns the current state of a machine.
func (c *Client) GetMachine(ctx context.Context, machineID string) (*Machine, error) {
	var machine Machine
	if err := c.do(ctx, "GET", c.machinesURL("/"+machineID), nil, &machine); err != nil {
		return nil, fmt.Errorf("could not check workspace status: %w", err)
	}
	return &machine, nil
}

// ListMachines returns all machines in the app, optionally filtered by metadata.
func (c *Client) ListMachines(ctx context.Context) ([]Machine, error) {
	var machines []Machine
	if err := c.do(ctx, "GET", c.machinesURL(""), nil, &machines); err != nil {
		return nil, fmt.Errorf("could not list workspaces: %w", err)
	}
	return machines, nil
}

// FindMachineByUser finds a machine by user ID metadata.
func (c *Client) FindMachineByUser(ctx context.Context, userID string) (*Machine, error) {
	machines, err := c.ListMachines(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range machines {
		if m.Config != nil && m.Config.Metadata["user_id"] == userID {
			return &m, nil
		}
	}
	return nil, nil // no machine found for this user
}

// --- Exec ---

// ExecCommand runs a command inside a running machine via the Fly Machines exec endpoint.
func (c *Client) ExecCommand(ctx context.Context, machineID string, cmd []string) error {
	body := map[string]any{
		"command": cmd,
	}
	if err := c.do(ctx, "POST", c.machinesURL("/"+machineID+"/exec"), body, nil); err != nil {
		return fmt.Errorf("could not exec command in workspace: %w", err)
	}
	return nil
}

// --- Volumes ---

// CreateVolume creates a persistent volume for vault data.
func (c *Client) CreateVolume(ctx context.Context, name string, sizeGB int) (*Volume, error) {
	body := map[string]any{
		"name":   name,
		"region": c.Region,
		"size_gb": sizeGB,
	}

	var vol Volume
	if err := c.do(ctx, "POST", c.volumesURL(""), body, &vol); err != nil {
		return nil, fmt.Errorf("could not create storage volume: %w", err)
	}
	return &vol, nil
}

// DestroyVolume permanently deletes a persistent volume.
func (c *Client) DestroyVolume(ctx context.Context, volumeID string) error {
	if err := c.do(ctx, "DELETE", c.volumesURL("/"+volumeID), nil, nil); err != nil {
		return fmt.Errorf("could not destroy volume: %w", err)
	}
	return nil
}

// --- HTTP helpers ---

func (c *Client) machinesURL(path string) string {
	return fmt.Sprintf("%s/apps/%s/machines%s", c.baseURL(), c.AppName, path)
}

func (c *Client) volumesURL(path string) string {
	return fmt.Sprintf("%s/apps/%s/volumes%s", c.baseURL(), c.AppName, path)
}

func (c *Client) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return defaultBaseURL
}

func (c *Client) do(ctx context.Context, method, url string, body any, result any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.APIToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request failed — check your network connection: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
	}

	return nil
}

// APIError represents a non-2xx response from the Fly API.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	switch e.StatusCode {
	case 401:
		return "authentication failed — is your Fly API token valid?"
	case 404:
		return "workspace not found — it may have been removed"
	case 422:
		return fmt.Sprintf("invalid configuration: %s", e.Body)
	case 429:
		return "rate limited by Fly API — try again in a moment"
	default:
		return fmt.Sprintf("Fly API error (%d): %s", e.StatusCode, e.Body)
	}
}
