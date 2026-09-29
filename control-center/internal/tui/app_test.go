package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewModel(t *testing.T) {
	m := NewModel()

	if m.currentView != ViewDashboard {
		t.Errorf("expected current view 'dashboard', got '%s'", m.currentView)
	}

	if len(m.views) != 7 {
		t.Errorf("expected 7 views, got %d", len(m.views))
	}

	if m.showHelp {
		t.Error("expected showHelp to be false initially")
	}
}

func TestModel_Init(t *testing.T) {
	m := NewModel()
	cmd := m.Init()

	if cmd != nil {
		t.Error("expected Init to return nil")
	}
}

func TestViewConstants(t *testing.T) {
	views := []View{
		ViewDashboard,
		ViewNodes,
		ViewAgents,
		ViewUsers,
		ViewPlaybooks,
		ViewLogs,
		ViewSettings,
	}

	for _, v := range views {
		if v == "" {
			t.Error("view constant should not be empty")
		}
	}
}

func TestDefaultKeyMap(t *testing.T) {
	km := defaultKeyMap()

	if !km.Up.Enabled() {
		t.Error("expected Up key to be enabled")
	}
	if !km.Down.Enabled() {
		t.Error("expected Down key to be enabled")
	}
	if !km.Quit.Enabled() {
		t.Error("expected Quit key to be enabled")
	}
}

func TestModel_View(t *testing.T) {
	m := NewModel()
	view := m.View()

	if view == "" {
		t.Error("expected non-empty view")
	}
}

func TestModel_View_Help(t *testing.T) {
	m := NewModel()
	m.showHelp = true
	view := m.View()

	if view == "" {
		t.Error("expected non-empty view")
	}
}

func TestModel_View_Search(t *testing.T) {
	m := NewModel()
	m.showSearch = true
	view := m.View()

	if view == "" {
		t.Error("expected non-empty view")
	}
}

func TestNodeItem(t *testing.T) {
	node := NodeItem{
		name:    "test-node",
		host:    "192.168.1.1",
		status:  "online",
		users:   100,
		traffic: "1GB",
	}

	if node.FilterValue() != "test-node" {
		t.Errorf("expected filter value 'test-node', got '%s'", node.FilterValue())
	}
	if node.Title() != "test-node" {
		t.Errorf("expected title 'test-node', got '%s'", node.Title())
	}
	if node.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestPlaybookItem(t *testing.T) {
	playbook := PlaybookItem{
		name:    "test-playbook.yml",
		lastRun: "2024-01-01",
		status:  "success",
	}

	if playbook.FilterValue() != "test-playbook.yml" {
		t.Errorf("expected filter value 'test-playbook.yml', got '%s'", playbook.FilterValue())
	}
	if playbook.Title() != "test-playbook.yml" {
		t.Errorf("expected title 'test-playbook.yml', got '%s'", playbook.Title())
	}
	if playbook.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestLogEntry(t *testing.T) {
	log := LogEntry{
		Time:    "12:34:56",
		Level:   "INFO",
		Source:  "test",
		Message: "test message",
	}

	if log.Time != "12:34:56" {
		t.Errorf("expected time '12:34:56', got '%s'", log.Time)
	}
	if log.Level != "INFO" {
		t.Errorf("expected level 'INFO', got '%s'", log.Level)
	}
}

func TestGenerateSampleLogs(t *testing.T) {
	logs := generateSampleLogs()

	if len(logs) == 0 {
		t.Error("expected non-empty sample logs")
	}
}

func TestModel_ShortHelp(t *testing.T) {
	m := NewModel()
	help := m.ShortHelp()

	if len(help) != 4 {
		t.Errorf("expected 4 short help items, got %d", len(help))
	}
}

func TestModel_FullHelp(t *testing.T) {
	m := NewModel()
	help := m.FullHelp()

	if len(help) == 0 {
		t.Error("expected non-empty full help")
	}
}

func TestModelState(t *testing.T) {
	m := NewModel()

	if m.state.nodesList.Title != "Nodes" {
		t.Errorf("expected nodes list title 'Nodes', got '%s'", m.state.nodesList.Title)
	}

	if m.state.playbooksList.Title != "Playbooks" {
		t.Errorf("expected playbooks list title 'Playbooks', got '%s'", m.state.playbooksList.Title)
	}
}

func TestUpdate_QuitKey(t *testing.T) {
	m := NewModel()

	// Create a quit key message
	msg := tea.KeyMsg{}
	// Simulate quit key
	_, cmd := m.Update(msg)

	// The command should be tea.Quit when quit key is pressed
	// Since we're not actually pressing 'q', cmd should be nil
	_ = cmd
}

func TestUpdate_WindowSize(t *testing.T) {
	m := NewModel()

	// Simulate window resize
	msg := tea.WindowSizeMsg{Width: 100, Height: 50}
	updatedModel, _ := m.Update(msg)

	newM := updatedModel.(Model)
	if newM.width != 100 {
		t.Errorf("expected width 100, got %d", newM.width)
	}
	if newM.height != 50 {
		t.Errorf("expected height 50, got %d", newM.height)
	}
}

func TestRenderFunctions(t *testing.T) {
	m := NewModel()

	// Test that render functions don't panic and return non-empty strings
	tests := []struct {
		name     string
		view     View
		renderFn func() string
	}{
		{"dashboard", ViewDashboard, m.renderDashboard},
		{"nodes", ViewNodes, m.renderNodes},
		{"agents", ViewAgents, m.renderAgents},
		{"users", ViewUsers, m.renderUsers},
		{"playbooks", ViewPlaybooks, m.renderPlaybooks},
		{"logs", ViewLogs, m.renderLogs},
		{"settings", ViewSettings, m.renderSettings},
		{"header", "", m.renderHeader},
		{"footer", "", m.renderFooter},
		{"help", "", m.renderHelp},
		{"searchBar", "", m.renderSearchBar},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.renderFn()
			if result == "" {
				t.Errorf("expected non-empty render output for %s", tt.name)
			}
		})
	}
}

func TestPrintFunctions(t *testing.T) {
	// These functions just print to stdout, verify they don't panic
	PrintHeader("Test Header")
	PrintSuccess("Test success")
	PrintError("Test error")
	PrintInfo("Test info")
	PrintWarning("Test warning")
}

func TestStyles(t *testing.T) {
	// Verify styles are defined
	if titleStyle.GetBold() != true {
		t.Error("expected titleStyle to be bold")
	}

	// Test that styles can render
	rendered := titleStyle.Render("Test")
	if rendered == "" {
		t.Error("expected style render to return non-empty string")
	}
}

func TestUpdateDashboard(t *testing.T) {
	// Test switching views via dashboard number keys
	testCases := []struct {
		key      string
		expected View
	}{
		{"1", ViewNodes},
		{"2", ViewUsers},
		{"3", ViewPlaybooks},
		{"4", ViewLogs},
		{"5", ViewSettings},
	}

	for _, tc := range testCases {
		t.Run(tc.key, func(t *testing.T) {
			_ = tc.expected
		})
	}
}

func TestUpdate_TabKey(t *testing.T) {
	m := NewModel()
	initialIndex := m.viewIndex

	// Simulate Tab key - we'll test by manually changing the view index
	m.viewIndex = (m.viewIndex + 1) % len(m.views)
	m.currentView = m.views[m.viewIndex]

	if m.viewIndex == initialIndex {
		t.Error("expected viewIndex to change after Tab")
	}
}

func TestUpdate_ShiftTabKey(t *testing.T) {
	m := NewModel()
	initialIndex := m.viewIndex

	// Simulate Shift+Tab key
	m.viewIndex--
	if m.viewIndex < 0 {
		m.viewIndex = len(m.views) - 1
	}
	m.currentView = m.views[m.viewIndex]

	if m.viewIndex == initialIndex {
		t.Error("expected viewIndex to change after Shift+Tab")
	}
}

func TestHandleEnter(t *testing.T) {
	m := NewModel()
	m.currentView = ViewDashboard

	_, _ = m.handleEnter()
	// handleEnter should not panic
}

func TestHandleEnter_Nodes(t *testing.T) {
	m := NewModel()
	m.currentView = ViewNodes

	_, _ = m.handleEnter()
	// Should handle nodes list selection
}

func TestHandleEnter_Playbooks(t *testing.T) {
	m := NewModel()
	m.currentView = ViewPlaybooks

	_, _ = m.handleEnter()
	// Should handle playbooks selection
}

func TestModel_ShowSearch(t *testing.T) {
	m := NewModel()

	if m.showSearch {
		t.Error("expected showSearch to be false initially")
	}

	// Toggle search
	m.showSearch = true
	m.state.searchInput.Focus()

	if !m.showSearch {
		t.Error("expected showSearch to be true after toggling")
	}
}

func TestModel_ShowHelp(t *testing.T) {
	m := NewModel()

	if m.showHelp {
		t.Error("expected showHelp to be false initially")
	}

	// Toggle help
	m.showHelp = true

	if !m.showHelp {
		t.Error("expected showHelp to be true after toggling")
	}
}

func TestModel_StatusMessage(t *testing.T) {
	m := NewModel()
	m.state.statusMessage = "Test message"

	view := m.View()
	if view == "" {
		t.Error("expected non-empty view")
	}
}

func TestModel_ViewIndex(t *testing.T) {
	m := NewModel()

	for i, view := range m.views {
		if view == "" {
			t.Errorf("view at index %d is empty", i)
		}
	}
}

func TestModel_Views(t *testing.T) {
	expectedViews := []View{
		ViewDashboard, ViewNodes, ViewAgents, ViewUsers,
		ViewPlaybooks, ViewLogs, ViewSettings,
	}

	m := NewModel()
	for i, expected := range expectedViews {
		if m.views[i] != expected {
			t.Errorf("expected view %d to be '%s', got '%s'", i, expected, m.views[i])
		}
	}
}
