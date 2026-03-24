package workspace

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// RunHook executes a shell hook script in the given directory with a timeout.
// Empty scripts are no-ops. Returns error on failure or timeout.
func RunHook(ctx context.Context, name, script, cwd string, timeout time.Duration) error {
	script = strings.TrimSpace(script)
	if script == "" {
		return nil
	}

	slog.Info("running hook", "hook", name, "cwd", cwd)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-lc", script)
	cmd.Dir = cwd
	cmd.Env = os.Environ()

	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("hook %q timed out after %v", name, timeout)
	}
	if err != nil {
		return fmt.Errorf("hook %q failed: %w\n%s", name, err, truncateOutput(out))
	}

	slog.Info("hook completed", "hook", name)
	return nil
}

func truncateOutput(out []byte) string {
	const maxLen = 4096
	if len(out) <= maxLen {
		return string(out)
	}
	return string(out[:maxLen]) + "\n... (truncated)"
}
