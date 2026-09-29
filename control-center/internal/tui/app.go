package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// keyMap defines keybindings
type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	Left     key.Binding
	Right    key.Binding
	Tab      key.Binding
	ShiftTab key.Binding
	Enter    key.Binding
	Back     key.Binding
	Quit     key.Binding
	Help     key.Binding
	Search   key.Binding
	Refresh  key.Binding
	New      key.Binding
	Edit     key.Binding
	Delete   key.Binding
	Filter   key.Binding
}

// defaultKeyMap returns default keybindings
func defaultKeyMap() keyMap {
	return keyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Left: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("←/h", "left"),
		),
		Right: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("→/l", "right"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next"),
		),
		ShiftTab: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("Shift+Tab", "previous"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r", "ctrl+r"),
			key.WithHelp("r", "refresh"),
		),
		New: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "new"),
		),
		Edit: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "edit"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "delete"),
		),
		Filter: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "filter"),
		),
	}
}

// View represents a TUI view
type View string

const (
	ViewDashboard View = "dashboard"
	ViewNodes     View = "nodes"
	ViewAgents    View = "agents"
	ViewUsers     View = "users"
	ViewPlaybooks View = "playbooks"
	ViewLogs      View = "logs"
	ViewSettings  View = "settings"
)

// ModelState holds the state for each view
type ModelState struct {
	nodesList     list.Model
	agentsList    list.Model
	usersTable    table.Model
	playbooksList list.Model
	logs          []LogEntry
	searchInput   textinput.Model
	showModal     bool
	modalContent  string
	loading       bool
	statusMessage string
}

// LogEntry represents a log entry
type LogEntry struct {
	Time    string
	Level   string
	Source  string
	Message string
}

// Model represents the TUI model
type Model struct {
	keys        keyMap
	help        help.Model
	currentView View
	viewIndex   int
	views       []View
	width       int
	height      int
	showHelp    bool
	showSearch  bool
	state       ModelState
}

// NewModel creates a new TUI model
func NewModel() Model {
	// Initialize search input
	searchInput := textinput.New()
	searchInput.Placeholder = "Search..."
	searchInput.CharLimit = 100

	// Initialize nodes list
	nodesList := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	nodesList.Title = "Nodes"
	nodesList.SetShowStatusBar(false)
	nodesList.SetFilteringEnabled(true)

	// Initialize users table
	usersTable := table.New(
		table.WithColumns([]table.Column{
			{Title: "ID", Width: 5},
			{Title: "Email", Width: 30},
			{Title: "Plan", Width: 15},
			{Title: "Status", Width: 10},
			{Title: "Used", Width: 10},
		}),
		table.WithRows([]table.Row{}),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	// Initialize playbooks list
	playbooksList := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	playbooksList.Title = "Playbooks"
	playbooksList.SetShowStatusBar(false)

	return Model{
		keys:        defaultKeyMap(),
		help:        help.New(),
		currentView: ViewDashboard,
		viewIndex:   0,
		views: []View{
			ViewDashboard,
			ViewNodes,
			ViewAgents,
			ViewUsers,
			ViewPlaybooks,
			ViewLogs,
			ViewSettings,
		},
		showHelp: false,
		state: ModelState{
			nodesList:     nodesList,
			playbooksList: playbooksList,
			usersTable:    usersTable,
			searchInput:   searchInput,
			logs:          generateSampleLogs(),
		},
	}
}

// Init initializes the TUI
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles updates
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle quit
		if key.Matches(msg, m.keys.Quit) {
			return m, tea.Quit
		}

		// Handle help toggle
		if key.Matches(msg, m.keys.Help) {
			m.showHelp = !m.showHelp
			return m, nil
		}

		// Handle search toggle
		if key.Matches(msg, m.keys.Search) {
			m.showSearch = !m.showSearch
			if m.showSearch {
				m.state.searchInput.Focus()
			}
			return m, nil
		}

		// Handle view switching
		if key.Matches(msg, m.keys.Tab) {
			m.viewIndex = (m.viewIndex + 1) % len(m.views)
			m.currentView = m.views[m.viewIndex]
			return m, nil
		}

		if key.Matches(msg, m.keys.ShiftTab) {
			m.viewIndex--
			if m.viewIndex < 0 {
				m.viewIndex = len(m.views) - 1
			}
			m.currentView = m.views[m.viewIndex]
			return m, nil
		}

		// Handle search input
		if m.showSearch {
			var cmd tea.Cmd
			m.state.searchInput, cmd = m.state.searchInput.Update(msg)
			return m, cmd
		}

		// Handle navigation based on current view
		switch m.currentView {
		case ViewDashboard:
			return m.updateDashboard(msg)
		case ViewNodes:
			m.state.nodesList, _ = m.state.nodesList.Update(msg)
		case ViewPlaybooks:
			m.state.playbooksList, _ = m.state.playbooksList.Update(msg)
		}

		// Handle enter key for actions
		if key.Matches(msg, m.keys.Enter) {
			return m.handleEnter()
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width

		// Update list dimensions
		m.state.nodesList.SetSize(m.width-4, m.height-8)
		m.state.playbooksList.SetSize(m.width-4, m.height-8)

		return m, nil
	}

	return m, tea.Batch(cmds...)
}

// handleEnter handles the enter key press
func (m *Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.currentView {
	case ViewNodes:
		if item, ok := m.state.nodesList.SelectedItem().(NodeItem); ok {
			m.state.statusMessage = fmt.Sprintf("Selected node: %s", item.name)
		}
	case ViewPlaybooks:
		if item, ok := m.state.playbooksList.SelectedItem().(PlaybookItem); ok {
			m.state.statusMessage = fmt.Sprintf("Running playbook: %s", item.name)
		}
	}
	return m, nil
}

// View renders the TUI
func (m Model) View() string {
	if m.showHelp {
		return m.renderHelp()
	}

	// Build layout
	var content strings.Builder

	// Header
	content.WriteString(m.renderHeader())
	content.WriteString("\n")

	// Main content based on view
	switch m.currentView {
	case ViewDashboard:
		content.WriteString(m.renderDashboard())
	case ViewNodes:
		content.WriteString(m.renderNodes())
	case ViewAgents:
		content.WriteString(m.renderAgents())
	case ViewUsers:
		content.WriteString(m.renderUsers())
	case ViewPlaybooks:
		content.WriteString(m.renderPlaybooks())
	case ViewLogs:
		content.WriteString(m.renderLogs())
	case ViewSettings:
		content.WriteString(m.renderSettings())
	}

	// Search bar if active
	if m.showSearch {
		content.WriteString("\n")
		content.WriteString(m.renderSearchBar())
	}

	// Status message
	if m.state.statusMessage != "" {
		content.WriteString("\n")
		content.WriteString(statusStyle.Render(m.state.statusMessage))
	}

	// Footer
	content.WriteString("\n")
	content.WriteString(m.renderFooter())

	return content.String()
}

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86"))

	viewStyle = lipgloss.NewStyle().
			Padding(1, 2)

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("208"))

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86"))

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("57")).
			Padding(0, 1)

	searchStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)

	viewTabStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Padding(0, 1)

	activeTabStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			Bold(true).
			Padding(0, 1).
			Underline(true)
)

func (m Model) renderHeader() string {
	title := titleStyle.Render(" AnixOps Control Center v1.0 ")
	status := headerStyle.Render("● Running")
	plugins := headerStyle.Render("Plugins: 4/4")
	nodes := headerStyle.Render("Nodes: 12")
	alerts := warningStyle.Render("Alerts: 3")

	topRow := lipgloss.JoinHorizontal(lipgloss.Top, title, "  ", status, " │ ", plugins, " │ ", nodes, " │ ", alerts)

	// View tabs
	tabs := []string{}
	for i, v := range m.views {
		if i == m.viewIndex {
			tabs = append(tabs, activeTabStyle.Render(string(v)))
		} else {
			tabs = append(tabs, viewTabStyle.Render(string(v)))
		}
	}
	tabsRow := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)

	return lipgloss.JoinVertical(lipgloss.Left, topRow, tabsRow)
}

func (m Model) renderFooter() string {
	keys := "[Tab] Next │ [Shift+Tab] Prev │ [Enter] Select │ [Esc] Back │ [/] Search │ [?] Help │ [q] Quit"
	return footerStyle.Render(keys)
}

func (m Model) renderHelp() string {
	helpText := `
AnixOps Control Center - Help
=============================

Navigation:
  ↑/k, ↓/j    - Move up/down
  ←/h, →/l    - Move left/right
  Tab         - Next view
  Shift+Tab   - Previous view
  Enter       - Select/Execute
  Esc         - Back/Cancel

Actions:
  n           - New item
  e           - Edit item
  d           - Delete item
  r           - Refresh
  f           - Filter
  /           - Search
  ?           - Toggle help

General:
  q/Ctrl+C    - Quit

Press any key to close this help.
`
	return boxStyle.Render(helpText)
}

func (m Model) renderSearchBar() string {
	return searchStyle.Render("Search: " + m.state.searchInput.View())
}

func (m Model) renderDashboard() string {
	// Left panel - Infrastructure
	leftBox := boxStyle.Render(`
   Infrastructure

   [●] Panel        Running
   [●] Nodes (8)    156 users
   [●] Agents (4)   All healthy
   [○] Monitoring   Disabled
	`)

	// Right panel - Recent Activity
	rightBox := boxStyle.Render(`
   Recent Activity

   12:34:56  Node tokyo-01 deployed
   12:33:21  User admin logged in
   12:30:00  Backup completed
   12:28:45  Certificate renewed
	`)

	// Quick actions
	actionsBox := boxStyle.Render(`
   Quick Actions

   [1] Deploy Node
   [2] Manage Users
   [3] Run Playbook
   [4] View Logs
   [5] System Settings
	`)

	// Metrics
	metricsBox := boxStyle.Render(`
   System Metrics

   CPU: ████████░░ 67%
   MEM: ██████░░░░ 54%
   DISK: ████░░░░░░ 42%
   NET: ↑ 1.2GB ↓ 3.4GB

   Requests: 1,234/min
   Latency:  45ms avg
	`)

	// Layout
	topRow := lipgloss.JoinHorizontal(lipgloss.Top, leftBox, "  ", rightBox)
	bottomRow := lipgloss.JoinHorizontal(lipgloss.Top, actionsBox, "  ", metricsBox)

	return lipgloss.JoinVertical(lipgloss.Left, topRow, "\n", bottomRow)
}

// NodeItem represents a node list item
type NodeItem struct {
	name    string
	host    string
	status  string
	users   int
	traffic string
}

func (n NodeItem) FilterValue() string { return n.name }
func (n NodeItem) Title() string       { return n.name }
func (n NodeItem) Description() string {
	return fmt.Sprintf("%s │ %s │ %d users │ %s", n.host, n.status, n.users, n.traffic)
}

// PlaybookItem represents a playbook list item
type PlaybookItem struct {
	name    string
	lastRun string
	status  string
}

func (p PlaybookItem) FilterValue() string { return p.name }
func (p PlaybookItem) Title() string       { return p.name }
func (p PlaybookItem) Description() string {
	return fmt.Sprintf("Last run: %s │ %s", p.lastRun, p.status)
}

func (m Model) renderNodes() string {
	// Sample data
	items := []list.Item{
		NodeItem{name: "tokyo-01", host: "192.168.1.101", status: "● Online", users: 156, traffic: "1.2TB"},
		NodeItem{name: "singapore-01", host: "192.168.1.102", status: "● Online", users: 89, traffic: "890GB"},
		NodeItem{name: "la-01", host: "192.168.1.103", status: "○ Offline", users: 0, traffic: "0B"},
		NodeItem{name: "frankfurt-01", host: "192.168.1.104", status: "● Online", users: 45, traffic: "234GB"},
		NodeItem{name: "london-01", host: "192.168.1.105", status: "● Online", users: 67, traffic: "456GB"},
	}

	m.state.nodesList.SetItems(items)

	actions := boxStyle.Render(`
   Actions: [n] New │ [e] Edit │ [d] Delete │ [r] Refresh
	`)

	return m.state.nodesList.View() + "\n" + actions
}

func (m Model) renderAgents() string {
	return boxStyle.Render(`
   Agents View
   ===========

   ID   Name          Host            Status    Uptime     Version
   ────────────────────────────────────────────────────────────────
   1    web-01        10.0.0.101      ●         2d 4h      1.0.0
   2    db-01         10.0.0.102      ●         5d 12h     1.0.0
   3    cache-01      10.0.0.103      ○         -          -

   Actions: [c] Connect │ [e] Execute │ [r] Refresh
	`)
}

func (m Model) renderUsers() string {
	rows := []table.Row{
		{"1", "admin@example.com", "Pro", "● Active", "12.4GB"},
		{"2", "user1@example.com", "Basic", "● Active", "5.2GB"},
		{"3", "user2@example.com", "Pro", "○ Banned", "0B"},
		{"4", "user3@example.com", "Enterprise", "● Active", "45.6GB"},
		{"5", "user4@example.com", "Basic", "● Active", "1.8GB"},
	}

	m.state.usersTable.SetRows(rows)

	actions := boxStyle.Render(`
   Actions: [n] New │ [e] Edit │ [b] Ban │ [u] Unban │ [r] Refresh
	`)

	return m.state.usersTable.View() + "\n" + actions
}

func (m Model) renderPlaybooks() string {
	items := []list.Item{
		PlaybookItem{name: "deploy_node.yml", lastRun: "2024-03-15 12:34", status: "✓ Success"},
		PlaybookItem{name: "update_certificates.yml", lastRun: "2024-03-14 08:00", status: "✓ Success"},
		PlaybookItem{name: "backup_database.yml", lastRun: "2024-03-15 00:00", status: "○ Running..."},
		PlaybookItem{name: "cleanup_logs.yml", lastRun: "2024-03-13 06:00", status: "✗ Failed"},
		PlaybookItem{name: "deploy_agent.yml", lastRun: "2024-03-12 18:30", status: "✓ Success"},
	}

	m.state.playbooksList.SetItems(items)

	actions := boxStyle.Render(`
   Actions: [Enter] Run │ [v] View │ [e] Edit │ [r] Refresh
	`)

	return m.state.playbooksList.View() + "\n" + actions
}

func (m Model) renderLogs() string {
	var logs strings.Builder
	logs.WriteString("   Logs View\n")
	logs.WriteString("   ==========\n\n")

	for _, log := range m.state.logs {
		levelStyle := infoStyle
		switch log.Level {
		case "ERROR":
			levelStyle = errorStyle
		case "WARN":
			levelStyle = warningStyle
		case "INFO":
			levelStyle = infoStyle
		}

		logs.WriteString(fmt.Sprintf("   %s [%s] %s %s\n",
			log.Time,
			levelStyle.Render(log.Level),
			log.Source,
			log.Message,
		))
	}

	actions := boxStyle.Render(`
   Actions: [f] Filter │ [t] Tail │ [c] Clear │ [r] Refresh
	`)

	return boxStyle.Render(logs.String()) + "\n" + actions
}

func (m Model) renderSettings() string {
	return boxStyle.Render(`
   Settings View
   =============

   Server Configuration
   ├─ Host: 0.0.0.0
   ├─ Port: 8080
   └─ Mode: production

   Plugins
   ├─ ansible: ● Running
   ├─ v2board: ● Running
   ├─ v2bx:    ● Running
   └─ agent:   ● Running

   Authentication
   └─ Provider: local

   Actions: [e] Edit │ [s] Save │ [r] Restart
	`)
}

// updateDashboard handles dashboard updates
func (m Model) updateDashboard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "1":
		m.currentView = ViewNodes
		m.viewIndex = 1
		m.state.statusMessage = "Switching to Nodes view..."
	case "2":
		m.currentView = ViewUsers
		m.viewIndex = 3
		m.state.statusMessage = "Switching to Users view..."
	case "3":
		m.currentView = ViewPlaybooks
		m.viewIndex = 4
		m.state.statusMessage = "Switching to Playbooks view..."
	case "4":
		m.currentView = ViewLogs
		m.viewIndex = 5
		m.state.statusMessage = "Switching to Logs view..."
	case "5":
		m.currentView = ViewSettings
		m.viewIndex = 6
		m.state.statusMessage = "Switching to Settings view..."
	}
	return m, nil
}

// ShortHelp implements help.KeyMap interface
func (m Model) ShortHelp() []key.Binding {
	return []key.Binding{m.keys.Up, m.keys.Down, m.keys.Enter, m.keys.Quit}
}

// FullHelp implements help.KeyMap interface
func (m Model) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{m.keys.Up, m.keys.Down},
		{m.keys.Left, m.keys.Right},
		{m.keys.Tab, m.keys.Enter},
		{m.keys.Help, m.keys.Quit},
		{m.keys.New, m.keys.Edit},
		{m.keys.Delete, m.keys.Refresh},
		{m.keys.Search, m.keys.Filter},
	}
}

// Run starts the TUI application
func Run() error {
	p := tea.NewProgram(NewModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// PrintHeader prints a styled header
func PrintHeader(title string) {
	fmt.Println(titleStyle.Render(" " + title + " "))
}

// PrintSuccess prints a success message
func PrintSuccess(msg string) {
	fmt.Println(successStyle.Render("✓ " + msg))
}

// PrintError prints an error message
func PrintError(msg string) {
	fmt.Println(errorStyle.Render("✗ " + msg))
}

// PrintInfo prints an info message
func PrintInfo(msg string) {
	fmt.Println(infoStyle.Render("ℹ " + msg))
}

// PrintWarning prints a warning message
func PrintWarning(msg string) {
	fmt.Println(warningStyle.Render("⚠ " + msg))
}

// generateSampleLogs generates sample log entries
func generateSampleLogs() []LogEntry {
	return []LogEntry{
		{"12:34:56", "INFO", "node", "Node tokyo-01 deployed successfully"},
		{"12:34:55", "INFO", "ansible", "Running playbook: deploy_node.yml"},
		{"12:33:21", "INFO", "auth", "User admin logged in from 192.168.1.1"},
		{"12:30:00", "INFO", "backup", "Database backup completed"},
		{"12:28:45", "WARN", "cert", "Certificate for node-03 expires in 7 days"},
		{"12:20:00", "ERROR", "agent", "Failed to connect to agent cache-01: connection refused"},
		{"12:15:30", "INFO", "traffic", "Traffic report: 1.2TB uploaded, 3.4TB downloaded"},
		{"12:10:00", "INFO", "scheduler", "Running scheduled task: cleanup_logs"},
		{"12:05:00", "WARN", "memory", "Memory usage above 80% on node tokyo-01"},
		{"12:00:00", "INFO", "system", "AnixOps Control Center started"},
	}
}
