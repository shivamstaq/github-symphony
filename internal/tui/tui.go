package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/shivamstaq/github-symphony/internal/orchestrator"
)

// ViewMode tracks which screen the TUI is showing.
type ViewMode int

const (
	ViewOverview ViewMode = iota
	ViewDetail
)

// FocusPanel tracks which panel has keyboard focus in overview.
type FocusPanel int

const (
	FocusAgents FocusPanel = iota
	FocusEvents
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

type tickMsg time.Time
type eventMsg orchestrator.Event

// Model is the Bubble Tea model for the Symphony TUI.
type Model struct {
	cfg            Config
	state          orchestrator.State
	events         []orchestrator.Event
	eventSub       chan orchestrator.Event
	width          int
	height         int
	quitting       bool
	view           ViewMode
	focus          FocusPanel
	selectedAgent  int      // index into sorted agent list
	agentIDs       []string // sorted keys of Running map
	selectedDetail string   // work item ID of the detailed agent
	detailEvents   []orchestrator.Event
	eventsScroll   int // scroll offset for events panel
}

// New creates a new TUI model.
func New(cfg Config) Model {
	return Model{
		cfg:      cfg,
		eventSub: cfg.EventBus.Subscribe(),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(tickCmd(), waitForEvent(m.eventSub))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tickMsg:
		m.state = m.cfg.StateProvider.GetState()
		m.updateAgentIDs()
		return m, tickCmd()
	case eventMsg:
		e := orchestrator.Event(msg)
		m.events = append(m.events, e)
		// Auto-scroll to bottom if user hasn't scrolled up
		maxScroll := max(0, len(m.events)-m.eventsViewHeight())
		if m.eventsScroll >= maxScroll-1 {
			m.eventsScroll = max(0, len(m.events)-m.eventsViewHeight())
		}
		return m, waitForEvent(m.eventSub)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit

	case "tab":
		if m.view == ViewOverview {
			if m.focus == FocusAgents {
				m.focus = FocusEvents
			} else {
				m.focus = FocusAgents
			}
		}

	case "up", "k":
		if m.view == ViewDetail {
			// Scroll detail events up
			if m.eventsScroll > 0 {
				m.eventsScroll--
			}
		} else if m.focus == FocusAgents {
			if m.selectedAgent > 0 {
				m.selectedAgent--
			}
		} else {
			if m.eventsScroll > 0 {
				m.eventsScroll--
			}
		}

	case "down", "j":
		if m.view == ViewDetail {
			m.eventsScroll++
		} else if m.focus == FocusAgents {
			if m.selectedAgent < len(m.agentIDs)-1 {
				m.selectedAgent++
			}
		} else {
			maxScroll := max(0, len(m.events)-m.eventsViewHeight())
			if m.eventsScroll < maxScroll {
				m.eventsScroll++
			}
		}

	case "enter":
		if m.view == ViewOverview && m.focus == FocusAgents && len(m.agentIDs) > 0 {
			m.view = ViewDetail
			m.selectedDetail = m.agentIDs[m.selectedAgent]
			m.detailEvents = m.filterEventsForAgent(m.selectedDetail)
			m.eventsScroll = max(0, len(m.detailEvents)-m.detailEventsHeight())
		}

	case "esc":
		if m.view == ViewDetail {
			m.view = ViewOverview
			m.eventsScroll = max(0, len(m.events)-m.eventsViewHeight())
		}

	case "pgup":
		if m.focus == FocusEvents || m.view == ViewDetail {
			m.eventsScroll = max(0, m.eventsScroll-10)
		}

	case "pgdown":
		if m.focus == FocusEvents || m.view == ViewDetail {
			m.eventsScroll += 10
		}
	}
	return m, nil
}

func (m Model) View() string {
	if m.quitting {
		return "Shutting down...\n"
	}
	if m.view == ViewDetail {
		return m.viewDetail()
	}
	return m.viewOverview()
}

// --- STYLES ---

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	headerStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	goodStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	selectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("236")).Bold(true)
	focusStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("117"))
)

// --- OVERVIEW ---

func (m Model) viewOverview() string {
	w := max(80, m.width)
	var b strings.Builder

	// Header
	b.WriteString(m.renderHeader(w))
	b.WriteByte('\n')

	// Running agents
	b.WriteString(m.renderAgents(w))
	b.WriteByte('\n')

	// Retry queue
	b.WriteString(m.renderRetries(w))
	b.WriteByte('\n')

	// Events (scrollable, takes remaining vertical space)
	b.WriteString(m.renderEventsPanel(w))
	b.WriteByte('\n')

	// Footer
	focusHint := ""
	if m.focus == FocusAgents {
		focusHint = " (agents)"
	} else {
		focusHint = " (events)"
	}
	b.WriteString(strings.Repeat("─", w) + "\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("[q] Quit  [Tab] Switch focus%s  [↑↓] Navigate  [Enter] Detail  [PgUp/PgDn] Scroll", focusHint)))

	return b.String()
}

func (m Model) renderHeader(w int) string {
	uptime := time.Since(m.cfg.StartedAt).Truncate(time.Second)
	running := len(m.state.Running)
	maxAgents := m.state.MaxConcurrentAgents
	retrying := len(m.state.RetryAttempts)
	handedOff := 0
	if m.state.HandedOff != nil {
		handedOff = len(m.state.HandedOff)
	}

	line1 := titleStyle.Render("🎵 Symphony") + dimStyle.Render(fmt.Sprintf("  Uptime: %s", uptime))
	line2 := fmt.Sprintf("Agents: %s  │  Dispatched: %d  │  Handed Off: %s  │  Retrying: %s  │  Errors: %d",
		colorCount(running, maxAgents), m.state.DispatchTotal,
		goodStyle.Render(fmt.Sprintf("%d", handedOff)),
		retryColor(retrying), m.state.ErrorTotal)

	return line1 + "\n" + line2 + "\n" + strings.Repeat("─", w)
}

func (m Model) renderAgents(w int) string {
	var b strings.Builder
	label := "RUNNING AGENTS"
	if m.focus == FocusAgents {
		label = focusStyle.Render("▶ RUNNING AGENTS")
	} else {
		label = headerStyle.Render(label)
	}
	b.WriteString(label + "\n")

	if len(m.state.Running) == 0 {
		b.WriteString(dimStyle.Render("  (none)") + "\n")
		return b.String()
	}

	// Dynamic column widths
	issueW := max(20, w*35/100)
	phaseW := max(12, w*20/100)
	timeW := 10
	tokW := 10
	sessW := max(8, w-issueW-phaseW-timeW-tokW-6)

	hdr := fmt.Sprintf("  %-*s %-*s %-*s %-*s %s",
		issueW, "Issue", phaseW, "Phase", timeW, "Time", tokW, "Tokens", "Session")
	b.WriteString(hdr + "\n")
	b.WriteString("  " + strings.Repeat("─", w-4) + "\n")

	for i, id := range m.agentIDs {
		entry, ok := m.state.Running[id]
		if !ok {
			continue
		}
		elapsed := time.Since(entry.StartedAt).Truncate(time.Second)
		issue := entry.IssueIdentifier
		if issue == "" {
			issue = id
		}
		phase := string(entry.Phase)
		if phase == "" {
			phase = "running"
		}
		tokens := entry.InputTokens + entry.OutputTokens

		row := fmt.Sprintf("  %-*s %-*s %-*s %-*s %s",
			issueW, trunc(issue, issueW),
			phaseW, trunc(phase, phaseW),
			timeW, elapsed,
			tokW, fmtTokens(tokens),
			trunc(entry.SessionID, sessW))

		if m.focus == FocusAgents && i == m.selectedAgent {
			row = selectedStyle.Render(row)
		}
		b.WriteString(row + "\n")
	}
	return b.String()
}

func (m Model) renderRetries(w int) string {
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
			issue = trunc(entry.WorkItemID, 30)
		}
		b.WriteString(fmt.Sprintf("  %s → %s (attempt %d)\n", issue, status, entry.Attempt))
	}
	return b.String()
}

func (m Model) renderEventsPanel(w int) string {
	var b strings.Builder
	label := "RECENT EVENTS"
	if m.focus == FocusEvents {
		label = focusStyle.Render("▶ RECENT EVENTS") + dimStyle.Render(fmt.Sprintf(" (%d total, scroll: ↑↓ PgUp/PgDn)", len(m.events)))
	} else {
		label = headerStyle.Render(label) + dimStyle.Render(fmt.Sprintf(" (%d)", len(m.events)))
	}
	b.WriteString(label + "\n")

	if len(m.events) == 0 {
		b.WriteString(dimStyle.Render("  (no events yet)") + "\n")
		return b.String()
	}

	viewH := m.eventsViewHeight()
	start := m.eventsScroll
	end := min(start+viewH, len(m.events))
	if start >= len(m.events) {
		start = max(0, len(m.events)-viewH)
		end = len(m.events)
	}

	for _, e := range m.events[start:end] {
		b.WriteString(m.formatEvent(e, w))
	}

	// Scroll indicator
	if start > 0 {
		b.WriteString(dimStyle.Render("  ↑ more above") + "\n")
	}
	if end < len(m.events) {
		b.WriteString(dimStyle.Render("  ↓ more below") + "\n")
	}

	return b.String()
}

// --- DETAIL VIEW ---

func (m Model) viewDetail() string {
	w := max(80, m.width)
	var b strings.Builder

	entry, ok := m.state.Running[m.selectedDetail]
	if !ok {
		b.WriteString(headerStyle.Render("AGENT DETAIL") + "  " + dimStyle.Render("(agent no longer running)") + "\n\n")
		b.WriteString(dimStyle.Render("Press [Esc] to return") + "\n")
		return b.String()
	}

	// Header
	b.WriteString(strings.Repeat("─", w) + "\n")
	b.WriteString(headerStyle.Render("AGENT DETAIL") + "\n")
	b.WriteString(fmt.Sprintf("  Issue:    %s\n", entry.IssueIdentifier))
	b.WriteString(fmt.Sprintf("  Phase:    %s\n", entry.Phase))
	b.WriteString(fmt.Sprintf("  Branch:   %s\n", entry.WorkItem.IssueIdentifier))
	b.WriteString(fmt.Sprintf("  Session:  %s\n", entry.SessionID))
	b.WriteString(fmt.Sprintf("  Running:  %s\n", time.Since(entry.StartedAt).Truncate(time.Second)))
	b.WriteString(fmt.Sprintf("  Tokens:   %s\n", fmtTokens(entry.InputTokens+entry.OutputTokens)))
	b.WriteString(strings.Repeat("─", w) + "\n")

	// Events for this agent
	b.WriteString(headerStyle.Render("AGENT EVENTS") + dimStyle.Render(fmt.Sprintf(" (%d)", len(m.detailEvents))) + "\n")

	viewH := m.detailEventsHeight()
	start := m.eventsScroll
	end := min(start+viewH, len(m.detailEvents))
	if start >= len(m.detailEvents) {
		start = max(0, len(m.detailEvents)-viewH)
		end = len(m.detailEvents)
	}

	if len(m.detailEvents) == 0 {
		b.WriteString(dimStyle.Render("  (no events for this agent)") + "\n")
	} else {
		for _, e := range m.detailEvents[start:end] {
			b.WriteString(m.formatEvent(e, w))
		}
	}

	b.WriteString(strings.Repeat("─", w) + "\n")
	b.WriteString(dimStyle.Render("[Esc] Back  [↑↓] Scroll  [PgUp/PgDn] Page"))

	return b.String()
}

// --- HELPERS ---

func (m Model) formatEvent(e orchestrator.Event, w int) string {
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
	case orchestrator.EventDispatched, orchestrator.EventTurnStarted:
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	}

	issueW := max(15, w*25/100)
	msgW := max(20, w-issueW-12)

	return fmt.Sprintf("  %s  %s  %s\n",
		dimStyle.Render(ts),
		style.Render(trunc(issue, issueW)),
		trunc(e.Message, msgW))
}

func (m Model) eventsViewHeight() int {
	// Reserve: header(3) + agents(~6) + retries(~3) + events_header(1) + footer(2) + scroll_indicators(2)
	used := 17 + len(m.state.Running) + len(m.state.RetryAttempts)
	return max(5, m.height-used)
}

func (m Model) detailEventsHeight() int {
	return max(5, m.height-14) // header(8) + footer(2) + padding
}

func (m *Model) updateAgentIDs() {
	m.agentIDs = m.agentIDs[:0]
	for id := range m.state.Running {
		m.agentIDs = append(m.agentIDs, id)
	}
	if m.selectedAgent >= len(m.agentIDs) {
		m.selectedAgent = max(0, len(m.agentIDs)-1)
	}
}

func (m Model) filterEventsForAgent(workItemID string) []orchestrator.Event {
	var filtered []orchestrator.Event
	for _, e := range m.events {
		if e.WorkItemID == workItemID {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func waitForEvent(ch chan orchestrator.Event) tea.Cmd {
	return func() tea.Msg { return eventMsg(<-ch) }
}

func colorCount(n, max int) string {
	if n > 0 {
		return goodStyle.Render(fmt.Sprintf("%d/%d", n, max))
	}
	return dimStyle.Render(fmt.Sprintf("%d/%d", n, max))
}

func retryColor(n int) string {
	if n > 0 {
		return warnStyle.Render(fmt.Sprintf("%d", n))
	}
	return dimStyle.Render("0")
}

func fmtTokens(n int) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

func trunc(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return s[:n-1] + "…"
}
