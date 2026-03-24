package adapter

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
)

// SubprocessConfig configures a subprocess-based adapter.
type SubprocessConfig struct {
	Command string
	Args    []string
	Cwd     string
	Env     []string
}

// SubprocessAdapter manages a subprocess communicating via JSON-RPC over stdio.
type SubprocessAdapter struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	stderr  io.ReadCloser
	enc     *Encoder
	dec     *Decoder
	mu      sync.Mutex
	updates chan *Message
	done    chan struct{}
}

// NewSubprocessAdapter starts a subprocess and returns an adapter.
func NewSubprocessAdapter(cfg SubprocessConfig) (*SubprocessAdapter, error) {
	cmd := exec.Command(cfg.Command, cfg.Args...)
	if cfg.Cwd != "" {
		cmd.Dir = cfg.Cwd
	}
	if len(cfg.Env) > 0 {
		cmd.Env = append(os.Environ(), cfg.Env...)
	}

	// Set kill-on-parent-exit to prevent orphaned agent processes
	setPdeathsig(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("adapter subprocess: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("adapter subprocess: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("adapter subprocess: stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("adapter subprocess: start: %w", err)
	}

	a := &SubprocessAdapter{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		enc:     NewEncoder(stdin),
		dec:     NewDecoder(stdout),
		updates: make(chan *Message, 100),
		done:    make(chan struct{}),
	}

	// Drain stderr in background
	go a.drainStderr()

	return a, nil
}

// SendRequest sends a request and waits for the corresponding response.
// Notifications received while waiting are forwarded to the updates channel.
func (a *SubprocessAdapter) SendRequest(ctx context.Context, req Request) (*Message, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if err := a.enc.Encode(req); err != nil {
		return nil, fmt.Errorf("adapter send: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		msg, err := a.dec.Decode()
		if err != nil {
			if err == io.EOF {
				return nil, fmt.Errorf("process_exit: adapter process exited")
			}
			return nil, fmt.Errorf("adapter recv: %w", err)
		}

		// If it's a notification, forward it and keep reading
		if msg.IsNotification() {
			select {
			case a.updates <- msg:
			default:
				slog.Warn("adapter update channel full, dropping notification")
			}
			continue
		}

		// If it's a response matching our request
		if msg.ID == req.ID {
			if msg.Error != nil {
				return nil, fmt.Errorf("response_error: %s (code %d)", msg.Error.Message, msg.Error.Code)
			}
			return msg, nil
		}

		// For requests from adapter (permission/tool/input), auto-respond
		if msg.IsRequest() {
			// Forward to updates channel for observability
			select {
			case a.updates <- msg:
			default:
				slog.Warn("adapter update channel full, dropping request")
			}

			// Auto-respond to callback requests per harness policy
			a.handleCallbackRequest(msg)
			continue
		}
	}
}

// Updates returns the channel of notifications and adapter-to-client requests.
func (a *SubprocessAdapter) Updates() <-chan *Message {
	return a.updates
}

// Close terminates the subprocess.
func (a *SubprocessAdapter) Close() error {
	_ = a.stdin.Close()
	err := a.cmd.Process.Kill()
	_ = a.cmd.Wait()
	close(a.done)
	return err
}

// PID returns the subprocess PID if available.
func (a *SubprocessAdapter) PID() int {
	if a.cmd.Process != nil {
		return a.cmd.Process.Pid
	}
	return 0
}

// handleCallbackRequest auto-responds to adapter-to-client requests
// (permission, tool, input) per configured harness policy.
func (a *SubprocessAdapter) handleCallbackRequest(msg *Message) {
	switch msg.Method {
	case "session/request_permission":
		// Auto-approve permissions
		slog.Info("auto-approving permission request", "id", msg.ID)
		resp := Request{
			ID:     msg.ID,
			Method: "session/respond_permission",
			Params: map[string]any{
				"approved": true,
				"optionId": "allow-once",
			},
		}
		a.mu.Lock()
		_ = a.enc.Encode(resp)
		a.mu.Unlock()

	case "session/request_tool":
		// Execute tool request — forward to updates for the orchestrator to handle
		slog.Info("received tool request", "id", msg.ID)
		// For now, return unsupported
		resp := Request{
			ID:     msg.ID,
			Method: "session/respond_tool",
			Params: map[string]any{
				"success": false,
				"error":   "unsupported tool",
			},
		}
		a.mu.Lock()
		_ = a.enc.Encode(resp)
		a.mu.Unlock()

	case "session/request_input":
		// Deny input requests in automated mode
		slog.Info("denying input request", "id", msg.ID)
		resp := Request{
			ID:     msg.ID,
			Method: "session/respond_input",
			Params: map[string]any{
				"cancelled": true,
			},
		}
		a.mu.Lock()
		_ = a.enc.Encode(resp)
		a.mu.Unlock()

	default:
		slog.Warn("unknown adapter request method", "method", msg.Method, "id", msg.ID)
	}
}

func (a *SubprocessAdapter) drainStderr() {
	data, err := io.ReadAll(a.stderr)
	if len(data) > 0 {
		slog.Warn("adapter stderr", "output", string(data))
	}
	_ = err
}
