package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
)

var attachCmd = &cobra.Command{
	Use:   "attach <item-id>",
	Short: "Attach to a running agent's PTY session",
	Long:  `Connects to the Unix socket of a running agent and streams its terminal output. Press Ctrl+C to detach.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runAttach,
}

func init() {
	rootCmd.AddCommand(attachCmd)
}

func runAttach(cmd *cobra.Command, args []string) error {
	itemID := args[0]
	cwd, _ := os.Getwd()
	socketPath := filepath.Join(cwd, ".symphony", "sockets", itemID+".sock")

	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		return fmt.Errorf("no active session for %q — socket not found at %s", itemID, socketPath)
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return fmt.Errorf("connect to agent session: %w", err)
	}
	defer func() { _ = conn.Close() }()

	fmt.Fprintf(os.Stderr, "Attached to %s (Ctrl+C to detach)\n", itemID)

	// Handle Ctrl+C to detach cleanly
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nDetached.")
		_ = conn.Close()
		os.Exit(0)
	}()

	// Stream socket output to stdout
	_, err = io.Copy(os.Stdout, conn)
	if err != nil {
		return nil // connection closed normally
	}

	fmt.Fprintln(os.Stderr, "\nSession ended.")
	return nil
}
