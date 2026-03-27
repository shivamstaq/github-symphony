// Package opencode implements the Agent interface for the OpenCode CLI.
package opencode

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/creack/pty"
	"github.com/google/uuid"
	"github.com/shivamstaq/github-symphony/internal/agent"
)

// Config configures the OpenCode agent adapter.
type Config struct {
	Binary     string // path to opencode binary (default: "opencode")
	Model      string // model override
	ConfigFile string // --config path for opencode config
	LogDir     string // directory for agent logs
	SocketDir  string // directory for attach sockets
}

// Agent implements the agent.Agent interface using the OpenCode CLI with PTY.
type Agent struct {
	cfg Config
}

// New creates an OpenCode agent adapter.
func New(cfg Config) *Agent {
	if cfg.Binary == "" {
		cfg.Binary = "opencode"
	}
	return &Agent{cfg: cfg}
}

func (a *Agent) Start(ctx context.Context, cfg agent.StartConfig) (*agent.Session, error) {
	sessionID := uuid.New().String()
	updates := make(chan agent.Update, 100)
	done := make(chan agent.Result, 1)

	args := []string{"-p", "--output-format", "json"}
	if cfg.ResumeID != "" {
		args = append(args, "--resume", cfg.ResumeID)
	}
	if a.cfg.Model != "" {
		args = append(args, "--model", a.cfg.Model)
	}
	if a.cfg.ConfigFile != "" {
		args = append(args, "--config", a.cfg.ConfigFile)
	}

	cmd := exec.CommandContext(ctx, a.cfg.Binary, args...)
	cmd.Dir = cfg.WorkDir
	cmd.Stdin = strings.NewReader(cfg.Prompt)
	cmd.Env = os.Environ()

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("opencode pty start: %w", err)
	}

	itemID := sessionID
	ptySess, ptyErr := agent.NewPTYSession(ptmx, agent.PTYConfig{
		LogDir:    a.cfg.LogDir,
		SocketDir: a.cfg.SocketDir,
		ItemID:    itemID,
		RingSize:  64 * 1024,
	})

	socketPath := ""
	if ptyErr != nil {
		slog.Warn("PTY session setup failed, continuing without attach support", "error", ptyErr)
	} else {
		socketPath = ptySess.SocketPath()
	}

	go func() {
		defer close(updates)
		defer close(done)
		if ptySess != nil {
			defer ptySess.Close()
		}

		updates <- agent.Update{
			Kind:      agent.UpdateTurnStarted,
			Message:   "opencode session started",
			Timestamp: time.Now(),
		}

		startTime := time.Now()
		err := cmd.Wait()
		durationMs := int(time.Since(startTime).Milliseconds())

		result := agent.Result{
			SessionID:  sessionID,
			DurationMs: durationMs,
		}

		if err != nil {
			if ctx.Err() != nil {
				result.StopReason = agent.StopCancelled
			} else {
				result.StopReason = agent.StopFailed
				result.Error = err
			}
		} else {
			result.StopReason = agent.StopCompleted
			updates <- agent.Update{
				Kind:      agent.UpdateTurnDone,
				Message:   "opencode turn completed",
				Timestamp: time.Now(),
			}
		}

		result.HasCommits = agent.HasNewCommits(cfg.WorkDir)
		done <- result
	}()

	return &agent.Session{
		ID:         sessionID,
		PTY:        ptmx,
		SocketPath: socketPath,
		Updates:    updates,
		Done:       done,
	}, nil
}

// CheckDependencies verifies that the opencode binary is available on PATH.
func CheckDependencies() error {
	if _, err := exec.LookPath("opencode"); err != nil {
		return fmt.Errorf("opencode not found on PATH: %w", err)
	}
	return nil
}

// Ensure Agent implements the interface at compile time.
var _ agent.Agent = (*Agent)(nil)
