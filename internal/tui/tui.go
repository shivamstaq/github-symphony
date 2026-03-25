package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/shivamstaq/github-symphony/internal/orchestrator"
)

// StateProvider gives the TUI access to orchestrator state.
type StateProvider interface {
	GetState() orchestrator.State
}

// Config for the TUI.
type Config struct {
	StateProvider StateProvider
	EventBus      *orchestrator.EventBus
	StartedAt     time.Time
}

// tickMsg triggers periodic state refresh.
type tickMsg time.Time

// eventMsg wraps an orchestrator event.
type eventMsg orchestrator.Event

// Model is the Bubble Tea model for the Symphony TUI.
type Model struct {
	cfg       Config
	state     orchestrator.State
	events    []orchestrator.Event
	eventSub  chan orchestrator.Event
	width     int
	height    int
	quitting  bool
}

// New creates a new TUI model.
func New(cfg Config) Model {
	return Model{
		cfg:      cfg,
		eventSub: cfg.EventBus.Subscribe(),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(),
		waitForEvent(m.eventSub),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "r":
			m.state = m.cfg.StateProvider.GetState()
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tickMsg:
		m.state = m.cfg.StateProvider.GetState()
		return m, tickCmd()

	case eventMsg:
		e := orchestrator.Event(msg)
		m.events = append(m.events, e)
		if len(m.events) > 50 {
			m.events = m.events[len(m.events)-50:]
		}
		return m, waitForEvent(m.eventSub)
	}

	return m, nil
}

func (m Model) View() string {
	if m.quitting {
		return "Shutting down...\n"
	}

	var b strings.Builder

	// Header
	b.WriteString(m.renderHeader())
	b.WriteString("\n")

	// Running agents
	b.WriteString(m.renderRunningAgents())
	b.WriteString("\n")

	// Retry queue
	b.WriteString(m.renderRetryQueue())
	b.WriteString("\n")

	// Recent events
	b.WriteString(m.renderEvents())
	b.WriteString("\n")

	// Footer
	b.WriteString(m.renderFooter())

	return b.String()
}

// Styles
var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	goodStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

func (m Model) renderHeader() string {
	uptime := time.Since(m.cfg.StartedAt).Truncate(time.Second)
	running := len(m.state.Running)
	maxAgents := m.state.MaxConcurrentAgents
	retrying := len(m.state.RetryAttempts)
	handedOff := 0
	if m.state.HandedOff != nil {
		handedOff = len(m.state.HandedOff)
	}

	header := titleStyle.Render("🎵 Symphony") +
		dimStyle.Render(fmt.Sprintf("  Uptime: %s", uptime))

	stats := fmt.Sprintf(
		"Agents: %s  │  Dispatched: %d  │  Handed Off: %s  │  Retrying: %s  │  Errors: %d",
		agentCount(running, maxAgents),
		m.state.DispatchTotal,
		goodStyle.Render(fmt.Sprintf("%d", handedOff)),
		retryCount(retrying),
		m.state.ErrorTotal,
	)

	return header + "\n" + stats + "\n" + strings.Repeat("─", max(80, m.width))
}

func (m Model) renderRunningAgents() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("RUNNING AGENTS") + "\n")

	if len(m.state.Running) == 0 {
		b.WriteString(dimStyle.Render("  (none)") + "\n")
		return b.String()
	}

	// Header row
	b.WriteString(fmt.Sprintf("  %-20s %-14s %-8s %-10s %s\n",
		"Issue", "Phase", "Time", "Tokens", "Session"))
	b.WriteString("  " + strings.Repeat("─", 70) + "\n")

	for _, entry := range m.state.Running {
		elapsed := time.Since(entry.StartedAt).Truncate(time.Second)
		issue := entry.IssueIdentifier
		if issue == "" {
			issue = entry.WorkItem.WorkItemID[:min(20, len(entry.WorkItem.WorkItemID))]
		}
		phase := string(entry.Phase)
		if phase == "" {
			phase = "running"
		}
		tokens := entry.InputTokens + entry.OutputTokens

		b.WriteString(fmt.Sprintf("  %-20s %-14s %-8s %-10s %s\n",
			truncStr(issue, 20),
			truncStr(phase, 14),
			elapsed,
			formatTokens(tokens),
			truncStr(entry.SessionID, 12),
		))
	}

	return b.String()
}

func (m Model) renderRetryQueue() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("RETRY QUEUE") + "\n")

	if len(m.state.RetryAttempts) == 0 {
		b.WriteString(dimStyle.Render("  (empty)") + "\n")
		return b.String()
	}

	for _, entry := range m.state.RetryAttempts {
		dueIn := time.Until(entry.DueAt).Truncate(time.Second)
		status := fmt.Sprintf("due in %s", dueIn)
		if dueIn <= 0 {
			status = warnStyle.Render("firing")
		}
		issue := entry.IssueIdentifier
		if issue == "" {
			issue = entry.WorkItemID[:min(20, len(entry.WorkItemID))]
		}

		errStr := ""
		if entry.Error != "" {
			errStr = dimStyle.Render(" — " + truncStr(entry.Error, 40))
		}

		b.WriteString(fmt.Sprintf("  %s → %s (attempt %d)%s\n",
			issue, status, entry.Attempt, errStr))
	}

	return b.String()
}

func (m Model) renderEvents() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("RECENT EVENTS") + "\n")

	events := m.events
	if len(events) == 0 {
		// Fall back to event bus recent buffer
		events = m.cfg.EventBus.Recent()
	}

	if len(events) == 0 {
		b.WriteString(dimStyle.Render("  (no events yet)") + "\n")
		return b.String()
	}

	// Show last 10
	start := 0
	if len(events) > 10 {
		start = len(events) - 10
	}

	for _, e := range events[start:] {
		ts := e.Time.Format("15:04:05")
		issue := e.Issue
		if issue == "" {
			issue = e.WorkItemID
		}

		style := dimStyle
		switch e.Kind {
		case orchestrator.EventError:
			style = errorStyle
		case orchestrator.EventHandoff, orchestrator.EventPRCreated:
			style = goodStyle
		case orchestrator.EventBlocked:
			style = warnStyle
		}

		b.WriteString(fmt.Sprintf("  %s  %s  %s\n",
			dimStyle.Render(ts),
			style.Render(truncStr(issue, 25)),
			truncStr(e.Message, 50),
		))
	}

	return b.String()
}

func (m Model) renderFooter() string {
	return strings.Repeat("─", max(80, m.width)) + "\n" +
		dimStyle.Render("[q] Quit  [r] Refresh")
}

// Commands
func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func waitForEvent(ch chan orchestrator.Event) tea.Cmd {
	return func() tea.Msg {
		e := <-ch
		return eventMsg(e)
	}
}

// Helpers
func agentCount(running, max int) string {
	if running > 0 {
		return goodStyle.Render(fmt.Sprintf("%d/%d", running, max))
	}
	return dimStyle.Render(fmt.Sprintf("%d/%d", running, max))
}

func retryCount(n int) string {
	if n > 0 {
		return warnStyle.Render(fmt.Sprintf("%d", n))
	}
	return dimStyle.Render("0")
}

func formatTokens(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

func truncStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
