package agent

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
)

// PTYSession manages a pseudo-terminal session for an agent process.
// It captures output to a log file, maintains a ring buffer for TUI display,
// and serves the PTY stream over a Unix socket for `symphony attach`.
type PTYSession struct {
	mu         sync.Mutex
	ptyFile    *os.File    // PTY master fd
	logFile    *os.File    // stdout.log capture
	socketPath string      // Unix socket path
	listener   net.Listener
	ringBuf    *RingBuffer // last N bytes for quick attach
	clients    []net.Conn  // attached clients
	closed     bool
}

// PTYConfig configures a PTY session.
type PTYConfig struct {
	LogDir     string // directory for stdout.log
	SocketDir  string // directory for .sock file
	ItemID     string // work item ID (used in filenames)
	RingSize   int    // ring buffer size in bytes (default 64KB)
}

// NewPTYSession creates a PTY session that tees output to a log file and ring buffer.
// The ptyFile is the master side of the PTY (from creack/pty.Start).
func NewPTYSession(ptyFile *os.File, cfg PTYConfig) (*PTYSession, error) {
	ringSize := cfg.RingSize
	if ringSize <= 0 {
		ringSize = 64 * 1024 // 64KB default
	}

	// Create log file
	logDir := cfg.LogDir
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	logFile, err := os.OpenFile(
		filepath.Join(logDir, "stdout.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644,
	)
	if err != nil {
		return nil, fmt.Errorf("open stdout.log: %w", err)
	}

	// Create Unix socket
	socketDir := cfg.SocketDir
	if err := os.MkdirAll(socketDir, 0755); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("create socket dir: %w", err)
	}
	socketPath := filepath.Join(socketDir, cfg.ItemID+".sock")
	// Remove stale socket
	os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		logFile.Close()
		return nil, fmt.Errorf("listen unix socket: %w", err)
	}

	sess := &PTYSession{
		ptyFile:    ptyFile,
		logFile:    logFile,
		socketPath: socketPath,
		listener:   listener,
		ringBuf:    NewRingBuffer(ringSize),
	}

	// Accept attach connections in background
	go sess.acceptLoop()

	// Tee PTY output in background
	go sess.readLoop()

	return sess, nil
}

// SocketPath returns the Unix socket path for this session.
func (s *PTYSession) SocketPath() string {
	return s.socketPath
}

// readLoop reads from the PTY master and writes to log, ring buffer, and attached clients.
func (s *PTYSession) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := s.ptyFile.Read(buf)
		if n > 0 {
			data := buf[:n]

			// Write to log file
			s.logFile.Write(data)

			// Write to ring buffer
			s.ringBuf.Write(data)

			// Write to attached clients
			s.mu.Lock()
			alive := make([]net.Conn, 0, len(s.clients))
			for _, c := range s.clients {
				if _, werr := c.Write(data); werr == nil {
					alive = append(alive, c)
				} else {
					c.Close()
				}
			}
			s.clients = alive
			s.mu.Unlock()
		}
		if err != nil {
			if err != io.EOF {
				// PTY closed or process exited
			}
			return
		}
	}
}

// acceptLoop accepts new attach connections on the Unix socket.
func (s *PTYSession) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return // listener closed
		}

		s.mu.Lock()
		if s.closed {
			conn.Close()
			s.mu.Unlock()
			return
		}
		// Send ring buffer contents (recent history) to new client
		recent := s.ringBuf.Bytes()
		if len(recent) > 0 {
			conn.Write(recent)
		}
		s.clients = append(s.clients, conn)
		s.mu.Unlock()
	}
}

// Close shuts down the PTY session, closing all resources.
func (s *PTYSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	// Close attached clients
	for _, c := range s.clients {
		c.Close()
	}
	s.clients = nil

	// Close listener and remove socket file
	s.listener.Close()
	os.Remove(s.socketPath)

	// Close log file
	s.logFile.Close()

	return nil
}

// RingBuffer is a fixed-size circular byte buffer.
type RingBuffer struct {
	mu   sync.Mutex
	buf  []byte
	size int
	pos  int
	full bool
}

// NewRingBuffer creates a ring buffer with the given capacity.
func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		buf:  make([]byte, size),
		size: size,
	}
}

// Write appends data to the ring buffer, overwriting oldest data if full.
func (r *RingBuffer) Write(data []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, b := range data {
		r.buf[r.pos] = b
		r.pos = (r.pos + 1) % r.size
		if r.pos == 0 {
			r.full = true
		}
	}
}

// Bytes returns the current contents of the ring buffer in order.
func (r *RingBuffer) Bytes() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.full {
		out := make([]byte, r.pos)
		copy(out, r.buf[:r.pos])
		return out
	}

	// Buffer is full — data wraps around
	out := make([]byte, r.size)
	copy(out, r.buf[r.pos:])
	copy(out[r.size-r.pos:], r.buf[:r.pos])
	return out
}
