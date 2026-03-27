package tui

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/shivamstaq/github-symphony/internal/engine"
)

// ViewMode tracks which screen the TUI is showing.
type ViewMode int

const (
	ViewOverview ViewMode = iota
	ViewDetail
	ViewLogs
)

// EngineProvider gives the TUI access to engine state.
type EngineProvider interface {
	GetState() *engine.State
	Emit(engine.EngineEvent)
}

// Config for the TUI.
type Config struct {
	Engine    EngineProvider
	StartedAt time.Time
	LogDir    string // .symphony/logs/ path for log viewer
}

type tickMsg time.Time

// Model is the Bubble Tea model.
type Model struct {
	cfg           Config
	state         *engine.State
	width         int
	height        int
	quitting      bool
	view          ViewMode
	selectedAgent int
	agentIDs      []string
	selectedDetail string

	// Log viewer state
	logLines    []string
	logFilter   string
	logScroll   int
	logOffset   int64 // file offset for tailing
}

// New creates the TUI model.
func New(cfg Config) Model {
	return Model{
		cfg:       cfg,
		state:     cfg.Engine.GetState(),
		view:      ViewOverview,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tickMsg:
		m.state = m.cfg.Engine.GetState()
		m.updateAgentIDs()
		readLogLines(&m)
		return m, tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
			return tickMsg(t)
		})
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global keys
	switch msg.Type {
	case tea.KeyCtrlC:
		m.quitting = true
		return m, tea.Quit
	}

	switch m.view {
	case ViewOverview:
		return m.handleOverviewKey(msg)
	case ViewDetail:
		return m.handleDetailKey(msg)
	case ViewLogs:
		return m.handleLogsKey(msg)
	}
	return m, nil
}

func (m Model) handleOverviewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		m.quitting = true
		return m, tea.Quit
	case "up", "k":
		if m.selectedAgent > 0 {
			m.selectedAgent--
		}
	case "down", "j":
		if m.selectedAgent < len(m.agentIDs)-1 {
			m.selectedAgent++
		}
	case "enter":
		if m.selectedAgent < len(m.agentIDs) {
			m.selectedDetail = m.agentIDs[m.selectedAgent]
			m.view = ViewDetail
		}
	case "l":
		m.view = ViewLogs
	case "p":
		if m.selectedAgent < len(m.agentIDs) {
			id := m.agentIDs[m.selectedAgent]
			m.cfg.Engine.Emit(engine.NewEvent(engine.EvtPauseRequested, id, nil))
		}
	case "R":
		if m.selectedAgent < len(m.agentIDs) {
			id := m.agentIDs[m.selectedAgent]
			m.cfg.Engine.Emit(engine.NewEvent(engine.EvtResumeRequested, id, nil))
		}
	case "K":
		if m.selectedAgent < len(m.agentIDs) {
			id := m.agentIDs[m.selectedAgent]
			m.cfg.Engine.Emit(engine.NewEvent(engine.EvtCancelRequested, id, nil))
		}
	case "r":
		// Force refresh handled by engine
	}
	return m, nil
}

func (m Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "escape":
		m.view = ViewOverview
	case "p":
		m.cfg.Engine.Emit(engine.NewEvent(engine.EvtPauseRequested, m.selectedDetail, nil))
	case "R":
		m.cfg.Engine.Emit(engine.NewEvent(engine.EvtResumeRequested, m.selectedDetail, nil))
	case "K":
		m.cfg.Engine.Emit(engine.NewEvent(engine.EvtCancelRequested, m.selectedDetail, nil))
	}
	return m, nil
}

func (m Model) handleLogsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "escape":
		m.view = ViewOverview
	case "up", "k":
		if m.logScroll > 0 {
			m.logScroll--
		}
	case "down", "j":
		m.logScroll++
	}
	return m, nil
}

func (m *Model) updateAgentIDs() {
	if m.state == nil {
		m.agentIDs = nil
		return
	}
	ids := make([]string, 0, len(m.state.Running))
	for id := range m.state.Running {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	m.agentIDs = ids
	if m.selectedAgent >= len(ids) {
		m.selectedAgent = max(0, len(ids)-1)
	}
}

// readLogLines reads new lines from the orchestrator log file since last offset.
func readLogLines(m *Model) {
	if m.cfg.LogDir == "" {
		return
	}
	logPath := filepath.Join(m.cfg.LogDir, "orchestrator.jsonl")
	f, err := os.Open(logPath)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	if m.logOffset > 0 {
		_, _ = f.Seek(m.logOffset, 0)
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		m.logLines = append(m.logLines, line)
	}

	// Get current position for next read
	info, err := f.Stat()
	if err == nil {
		m.logOffset = info.Size()
	}

	// Cap to prevent unbounded growth
	const maxLogLines = 1000
	if len(m.logLines) > maxLogLines {
		m.logLines = m.logLines[len(m.logLines)-maxLogLines:]
	}
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	switch m.view {
	case ViewOverview:
		return m.viewOverview()
	case ViewDetail:
		return m.viewDetail()
	case ViewLogs:
		return m.viewLogs()
	}
	return ""
}

// viewOverview renders the main dashboard.
func (m Model) viewOverview() string {
	var b strings.Builder

	// Header
	uptime := time.Since(m.cfg.StartedAt).Truncate(time.Second)
	running := 0
	if m.state != nil {
		running = m.state.RunningCount()
	}
	header := titleStyle.Render(fmt.Sprintf(
		"Symphony    Agents: %d    Uptime: %s",
		running, uptime,
	))
	b.WriteString(header + "\n")
	b.WriteString(headerStyle.Render(strings.Repeat("─", min(m.width, 80))) + "\n")

	// Running agents
	if m.state != nil && len(m.agentIDs) > 0 {
		b.WriteString("\n RUNNING AGENTS\n")
		for i, id := range m.agentIDs {
			entry := m.state.Running[id]
			if entry == nil {
				continue
			}
			elapsed := time.Since(entry.StartedAt).Truncate(time.Second)
			phase := string(entry.Phase)
			if entry.Paused {
				phase = "PAUSED"
			}
			tokens := fmt.Sprintf("%dk", entry.TotalTokens/1000)
			line := fmt.Sprintf("  %-30s %-15s %8s %8s",
				truncate(entry.WorkItem.IssueIdentifier, 30),
				phase,
				elapsed,
				tokens,
			)
			if i == m.selectedAgent {
				b.WriteString(selectedStyle.Render(line) + "\n")
			} else {
				b.WriteString(line + "\n")
			}
		}
	} else {
		b.WriteString("\n " + dimStyle.Render("No agents running") + "\n")
	}

	// Retry queue
	if m.state != nil && len(m.state.RetryQueue) > 0 {
		b.WriteString("\n RETRY QUEUE\n")
		for _, re := range m.state.RetryQueue {
			dueIn := time.Until(re.DueAt).Truncate(time.Second)
			fmt.Fprintf(&b, "  %-30s due in %s (attempt %d)\n",
				re.IssueIdentifier, dueIn, re.Attempt)
		}
	}

	// Metrics
	if m.state != nil {
		fmt.Fprintf(&b, "\n Dispatched: %d  Handed off: %d  Errors: %d  Tokens: %dk\n",
			m.state.DispatchTotal,
			m.state.HandoffTotal,
			m.state.ErrorTotal,
			m.state.Totals.TotalTokens/1000,
		)
	}

	// Status bar
	b.WriteString("\n" + statusBarStyle.Render(
		" [q]uit [l]ogs [p]ause [R]esume [K]ill [Enter]detail [r]efresh",
	))

	return b.String()
}

// viewDetail renders the detail view for a specific agent.
func (m Model) viewDetail() string {
	var b strings.Builder

	entry, ok := m.state.Running[m.selectedDetail]
	if !ok {
		b.WriteString(titleStyle.Render("Agent Detail") + "\n\n")
		b.WriteString(dimStyle.Render("Agent no longer running") + "\n")
		b.WriteString("\n" + statusBarStyle.Render(" [q]back"))
		return b.String()
	}

	b.WriteString(titleStyle.Render("Agent Detail: "+entry.WorkItem.IssueIdentifier) + "\n")
	b.WriteString(headerStyle.Render(strings.Repeat("─", min(m.width, 80))) + "\n\n")

	elapsed := time.Since(entry.StartedAt).Truncate(time.Second)
	phase := string(entry.Phase)
	if entry.Paused {
		phase = warnStyle.Render("PAUSED")
	}

	fmt.Fprintf(&b, "  Title:     %s\n", entry.WorkItem.Title)
	fmt.Fprintf(&b, "  Phase:     %s\n", phase)
	fmt.Fprintf(&b, "  Elapsed:   %s\n", elapsed)
	fmt.Fprintf(&b, "  Turns:     %d\n", entry.TurnsCompleted)
	fmt.Fprintf(&b, "  Tokens:    %d (in: %d, out: %d)\n",
		entry.TotalTokens, entry.InputTokens, entry.OutputTokens)
	fmt.Fprintf(&b, "  Cost:      $%.4f\n", entry.CostUSD)
	fmt.Fprintf(&b, "  Attempt:   %d\n", entry.RetryAttempt)

	if entry.WorkItem.Repository != nil {
		fmt.Fprintf(&b, "  Repo:      %s\n", entry.WorkItem.Repository.FullName)
	}

	if entry.Session != nil && entry.Session.SocketPath != "" {
		fmt.Fprintf(&b, "  Socket:    %s\n", entry.Session.SocketPath)
	}

	fmt.Fprintf(&b, "\n  Last activity: %s ago\n",
		time.Since(entry.LastActivityAt).Truncate(time.Second))

	b.WriteString("\n" + statusBarStyle.Render(" [q]back [p]ause [R]esume [K]ill"))

	return b.String()
}

// viewLogs renders the log viewer.
func (m Model) viewLogs() string {
	var b strings.Builder

	filterInfo := "all"
	if m.logFilter != "" {
		filterInfo = m.logFilter
	}
	b.WriteString(titleStyle.Render("Logs") + "  " +
		dimStyle.Render(fmt.Sprintf("[filter: %s]", filterInfo)) + "\n")
	b.WriteString(headerStyle.Render(strings.Repeat("─", min(m.width, 80))) + "\n\n")

	if len(m.logLines) == 0 {
		b.WriteString(dimStyle.Render("  No log entries. Logs are written to .symphony/logs/") + "\n")
	} else {
		maxLines := m.height - 6
		if maxLines < 5 {
			maxLines = 5
		}
		start := m.logScroll
		if start >= len(m.logLines) {
			start = max(0, len(m.logLines)-1)
		}
		end := min(start+maxLines, len(m.logLines))
		for _, line := range m.logLines[start:end] {
			styled := line
			if strings.Contains(line, "ERROR") {
				styled = errorStyle.Render(line)
			} else if strings.Contains(line, "WARN") {
				styled = warnStyle.Render(line)
			}
			b.WriteString("  " + styled + "\n")
		}
	}

	b.WriteString("\n" + statusBarStyle.Render(" [q]back [j/k]scroll"))

	return b.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

