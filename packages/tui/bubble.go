package tui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/bketelsen/erasmus/packages/event"
	"github.com/bketelsen/erasmus/packages/harness"
	"github.com/bketelsen/erasmus/packages/model"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
)

type runtimeEventMsg struct{ event event.Event }
type runtimeDoneMsg struct{ err error }
type sessionsLoadedMsg struct {
	sessions []SessionSummary
	err      error
}
type sessionOpenedMsg struct {
	path string
	err  error
}
type modelAppliedMsg struct{ err error }
type treeLoadedMsg struct {
	tree harness.TreeState
	err  error
}
type treeMovedMsg struct{ err error }
type swarmsLoadedMsg struct {
	servers []SwarmServerSummary
	err     error
}
type swarmStatusLoadedMsg struct {
	status SwarmStatusSummary
	err    error
}
type swarmSentMsg struct {
	status SwarmStatusSummary
	err    error
}
type swarmStoppedMsg struct {
	status SwarmStatusSummary
	err    error
}
type swarmSpawnedMsg struct {
	status SwarmStatusSummary
	err    error
}
type swarmAttachTickMsg struct{ id int }
type commandDoneMsg struct {
	title string
	text  string
	err   error
}

type dialogMode int

const (
	dialogNone dialogMode = iota
	dialogSessions
	dialogModel
	dialogTree
	dialogSwarm
	dialogHelp
	dialogCommand
)

type bubbleModel struct {
	app *App
	ctx context.Context

	viewport viewport.Model
	input    textarea.Model
	width    int
	height   int
	renderer *glamour.TermRenderer
	theme    bubbleTheme

	transcript         string
	renderedTranscript string
	status             string
	err                string
	running            bool
	follow             bool
	assistantOpen      bool
	messages           <-chan tea.Msg

	dialog             dialogMode
	helpReturnDialog   dialogMode
	sessions           []SessionSummary
	selectedSession    int
	models             []model.Model
	selectedModel      int
	reasoningLevels    []string
	selectedReasoning  int
	tree               harness.TreeState
	selectedTree       int
	swarmServers       []SwarmServerSummary
	selectedSwarm      int
	swarmStatus        SwarmStatusSummary
	selectedSwarmAgent int
	swarmLog           string
	swarmNotice        string
	swarmPromptMode    string
	swarmAttached      bool
	swarmAttachTickID  int
	commandDialogTitle string
	commandDialogText  string

	searchActive              bool
	searchQuery               string
	searchMatches             []int
	searchIndex               int
	commandPopup              bool
	commandSuggestions        []slashCommand
	selectedCommandSuggestion int
}

type bubbleTheme struct {
	Name        string
	Glamour     string
	Brand       lipgloss.Style
	Muted       lipgloss.Style
	Error       lipgloss.Style
	Help        lipgloss.Style
	ReadyPill   lipgloss.Style
	RunningPill lipgloss.Style
	ErrorPill   lipgloss.Style
	Header      lipgloss.Style
	Viewport    lipgloss.Style
	Input       lipgloss.Style
	Dialog      lipgloss.Style
	Selected    lipgloss.Style
}

func darkBubbleTheme() bubbleTheme {
	return bubbleTheme{
		Name:        "dark",
		Glamour:     "dark",
		Brand:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81")),
		Muted:       lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		Error:       lipgloss.NewStyle().Foreground(lipgloss.Color("203")),
		Help:        lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		ReadyPill:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("114")).Padding(0, 1),
		RunningPill: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("222")).Padding(0, 1),
		ErrorPill:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("203")).Padding(0, 1),
		Header:      lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, true, false).BorderForeground(lipgloss.Color("238")).Padding(0, 1),
		Viewport:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("238")).Padding(0, 1),
		Input:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Padding(0, 1),
		Dialog:      lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(lipgloss.Color("99")).Padding(0, 1),
		Selected:    lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Background(lipgloss.Color("81")).Padding(0, 1),
	}
}

func plainBubbleTheme() bubbleTheme {
	return bubbleTheme{
		Name:        "plain",
		Glamour:     "notty",
		Brand:       lipgloss.NewStyle().Bold(true),
		Muted:       lipgloss.NewStyle(),
		Error:       lipgloss.NewStyle().Bold(true),
		Help:        lipgloss.NewStyle(),
		ReadyPill:   lipgloss.NewStyle().Bold(true).Padding(0, 1),
		RunningPill: lipgloss.NewStyle().Bold(true).Padding(0, 1),
		ErrorPill:   lipgloss.NewStyle().Bold(true).Padding(0, 1),
		Header:      lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, true, false).Padding(0, 1),
		Viewport:    lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1),
		Input:       lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1),
		Dialog:      lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1),
		Selected:    lipgloss.NewStyle().Bold(true).Padding(0, 1),
	}
}

func lightBubbleTheme() bubbleTheme {
	return bubbleTheme{
		Name:        "light",
		Glamour:     "light",
		Brand:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("25")),
		Muted:       lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		Error:       lipgloss.NewStyle().Foreground(lipgloss.Color("124")),
		Help:        lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		ReadyPill:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("29")).Padding(0, 1),
		RunningPill: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("220")).Padding(0, 1),
		ErrorPill:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("124")).Padding(0, 1),
		Header:      lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, true, false).BorderForeground(lipgloss.Color("250")).Padding(0, 1),
		Viewport:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("252")).Padding(0, 1),
		Input:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("32")).Padding(0, 1),
		Dialog:      lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(lipgloss.Color("61")).Padding(0, 1),
		Selected:    lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("25")).Padding(0, 1),
	}
}

func highContrastBubbleTheme() bubbleTheme {
	return bubbleTheme{
		Name:        "high-contrast",
		Glamour:     "notty",
		Brand:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")),
		Muted:       lipgloss.NewStyle().Foreground(lipgloss.Color("15")),
		Error:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9")),
		Help:        lipgloss.NewStyle().Foreground(lipgloss.Color("15")),
		ReadyPill:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("15")).Padding(0, 1),
		RunningPill: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("11")).Padding(0, 1),
		ErrorPill:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("9")).Padding(0, 1),
		Header:      lipgloss.NewStyle().Bold(true).Border(lipgloss.NormalBorder(), false, false, true, false).BorderForeground(lipgloss.Color("15")).Padding(0, 1),
		Viewport:    lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("15")).Padding(0, 1),
		Input:       lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(lipgloss.Color("15")).Padding(0, 1),
		Dialog:      lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(lipgloss.Color("15")).Padding(0, 1),
		Selected:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("15")).Padding(0, 1),
	}
}

func draculaBubbleTheme() bubbleTheme {
	return bubbleTheme{
		Name:        "dracula",
		Glamour:     "dracula",
		Brand:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")),
		Muted:       lipgloss.NewStyle().Foreground(lipgloss.Color("103")),
		Error:       lipgloss.NewStyle().Foreground(lipgloss.Color("203")),
		Help:        lipgloss.NewStyle().Foreground(lipgloss.Color("103")),
		ReadyPill:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("84")).Padding(0, 1),
		RunningPill: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("228")).Padding(0, 1),
		ErrorPill:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("203")).Padding(0, 1),
		Header:      lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, true, false).BorderForeground(lipgloss.Color("60")).Padding(0, 1),
		Viewport:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("60")).Padding(0, 1),
		Input:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("141")).Padding(0, 1),
		Dialog:      lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(lipgloss.Color("212")).Padding(0, 1),
		Selected:    lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Background(lipgloss.Color("212")).Padding(0, 1),
	}
}

func namedBubbleTheme(name string) bubbleTheme {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "plain", "mono", "monochrome":
		return plainBubbleTheme()
	case "light":
		return lightBubbleTheme()
	case "high-contrast", "contrast", "hc":
		return highContrastBubbleTheme()
	case "dracula":
		return draculaBubbleTheme()
	default:
		return darkBubbleTheme()
	}
}

func newBubbleModel(ctx context.Context, app *App) bubbleModel {
	vp := viewport.New()
	ta := textarea.New()
	ta.Placeholder = "Ask Erasmus"
	ta.Prompt = "┃ "
	ta.ShowLineNumbers = false
	ta.SetHeight(4)
	ta.SetWidth(80)
	ta.Focus()
	theme := namedBubbleTheme(app.Theme)
	renderer, _ := glamour.NewTermRenderer(glamour.WithStandardStyle(theme.Glamour), glamour.WithWordWrap(100))
	m := bubbleModel{app: app, ctx: ctx, viewport: vp, input: ta, status: "ready", follow: true, renderer: renderer, theme: theme, reasoningLevels: []string{"", "low", "medium", "high"}}
	m.appendLine("Erasmus TUI — Bubble Tea full-screen shell")
	m.syncTranscript()
	return m
}

func (m bubbleModel) Init() tea.Cmd { return textarea.Blink }

func (m bubbleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch x := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = x.Width
		m.height = x.Height
		m.resize()
		return m, nil
	case tea.MouseWheelMsg:
		return m.handleMouse(x)
	case tea.KeyPressMsg:
		if m.searchActive {
			return m.updateSearch(x)
		}
		if (x.String() == "n" || x.String() == "N") && len(m.searchMatches) > 0 {
			if x.String() == "N" {
				m.previousSearchMatch()
			} else {
				m.nextSearchMatch()
			}
			return m, nil
		}
		if x.String() == "?" && m.dialog != dialogHelp {
			m.helpReturnDialog = m.dialog
			m.dialog = dialogHelp
			m.status = "help"
			return m, nil
		}
		if m.dialog == dialogSessions {
			return m.updateSessionsDialog(x)
		}
		if m.dialog == dialogModel {
			return m.updateModelDialog(x)
		}
		if m.dialog == dialogTree {
			return m.updateTreeDialog(x)
		}
		if m.dialog == dialogSwarm {
			return m.updateSwarmDialog(x)
		}
		if m.dialog == dialogHelp {
			return m.updateHelpDialog(x)
		}
		if m.dialog == dialogCommand {
			return m.updateCommandDialog(x)
		}
		if isMultilineInputKey(x) {
			m.input.InsertString("\n")
			return m, nil
		}
		if updated, cmd, ok := m.handleViewportKey(x); ok {
			return updated, cmd
		}
		if updated, cmd, ok := m.handleCommandPopupKey(x); ok {
			return updated, cmd
		}
		switch x.String() {
		case "ctrl+f":
			m.startSearch()
			return m, nil
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			return m.submit()
		case "ctrl+o":
			return m.openSessionsDialog()
		case "ctrl+p":
			return m.openModelDialog()
		case "ctrl+t":
			return m.openTreeDialog()
		case "ctrl+w":
			return m.openSwarmDialog()
		}
	case runtimeEventMsg:
		m.applyEvent(x.event)
		return m, m.waitRuntime()
	case runtimeDoneMsg:
		m.running = false
		if x.err != nil {
			m.err = x.err.Error()
			m.status = "error"
			m.appendLine("error: " + x.err.Error())
		} else {
			m.err = ""
			m.status = "ready"
		}
		return m, nil
	case sessionsLoadedMsg:
		m.running = false
		if x.err != nil {
			m.err = x.err.Error()
			m.status = "error"
			return m, nil
		}
		m.err = ""
		m.status = "sessions"
		m.dialog = dialogSessions
		m.sessions = x.sessions
		m.selectedSession = 0
		return m, nil
	case sessionOpenedMsg:
		m.running = false
		m.dialog = dialogNone
		if x.err != nil {
			m.err = x.err.Error()
			m.status = "error"
			m.appendLine("error: " + x.err.Error())
			m.syncTranscript()
			return m, nil
		}
		m.err = ""
		m.status = "ready"
		m.follow = true
		m.transcript = ""
		m.appendLine("opened session: " + x.path)
		m.renderSessionTranscript()
		m.syncTranscript()
		return m, nil
	case modelAppliedMsg:
		m.running = false
		m.dialog = dialogNone
		if x.err != nil {
			m.err = x.err.Error()
			m.status = "error"
			m.appendLine("error: " + x.err.Error())
		} else {
			m.err = ""
			m.status = "ready"
			state := m.app.Harness.State(m.ctx)
			m.appendLine(fmt.Sprintf("[model] %s/%s reasoning=%s", state.Agent.Model.Provider, state.Agent.Model.ID, state.Agent.Reasoning))
		}
		m.syncTranscript()
		return m, nil
	case treeLoadedMsg:
		m.running = false
		if x.err != nil {
			m.err = x.err.Error()
			m.status = "error"
			return m, nil
		}
		m.err = ""
		m.status = "tree"
		m.dialog = dialogTree
		m.tree = x.tree
		m.selectedTree = 0
		for i, entry := range x.tree.Entries {
			if entry.ID == x.tree.LeafID {
				m.selectedTree = i
				break
			}
		}
		return m, nil
	case treeMovedMsg:
		m.running = false
		if x.err != nil {
			m.err = x.err.Error()
			m.status = "error"
			m.appendLine("error: " + x.err.Error())
			m.syncTranscript()
			return m, nil
		}
		m.dialog = dialogNone
		m.status = "ready"
		m.err = ""
		m.follow = true
		m.transcript = ""
		m.appendLine("moved session tree")
		m.renderSessionTranscript()
		m.syncTranscript()
		return m, nil
	case swarmsLoadedMsg:
		m.running = false
		if x.err != nil {
			m.err = x.err.Error()
			m.status = "error"
			return m, nil
		}
		m.err = ""
		m.status = "swarm"
		m.dialog = dialogSwarm
		m.swarmServers = x.servers
		m.selectedSwarm = 0
		m.swarmStatus = SwarmStatusSummary{}
		m.selectedSwarmAgent = 0
		m.swarmLog = ""
		m.swarmAttached = false
		m.swarmNotice = fmt.Sprintf("loaded %d swarm server(s)", len(x.servers))
		if len(x.servers) > 0 && x.servers[0].Reachable {
			m.running = true
			return m, m.loadSelectedSwarmStatus()
		}
		return m, nil
	case swarmStatusLoadedMsg:
		m.running = false
		if x.err != nil {
			m.err = x.err.Error()
			m.status = "swarm"
			m.swarmStatus = SwarmStatusSummary{}
			m.swarmNotice = "status error: " + x.err.Error()
			return m, nil
		}
		m.err = ""
		m.status = "swarm"
		m.swarmStatus = x.status
		m.swarmNotice = fmt.Sprintf("status refreshed · agents=%d", len(x.status.Agents))
		if m.selectedSwarmAgent >= len(m.swarmStatus.Agents) {
			m.selectedSwarmAgent = max(0, len(m.swarmStatus.Agents)-1)
		}
		if m.swarmAttached {
			m.loadSelectedAgentLog()
			return m, m.swarmAttachTick()
		}
		return m, nil
	case swarmSentMsg:
		m.running = false
		m.swarmPromptMode = ""
		if x.err != nil {
			m.err = x.err.Error()
			m.status = "swarm"
			m.swarmNotice = "send error: " + x.err.Error()
			return m, nil
		}
		m.err = ""
		m.status = "swarm"
		m.swarmStatus = x.status
		m.swarmNotice = "prompt sent; status refreshed"
		m.loadSelectedAgentLog()
		return m, nil
	case swarmStoppedMsg:
		m.running = false
		if x.err != nil {
			m.err = x.err.Error()
			m.status = "swarm"
			m.swarmNotice = "stop error: " + x.err.Error()
			return m, nil
		}
		m.err = ""
		m.status = "swarm"
		m.swarmStatus = x.status
		m.swarmNotice = "agent stop requested; status refreshed"
		return m, nil
	case swarmSpawnedMsg:
		m.running = false
		m.swarmPromptMode = ""
		if x.err != nil {
			m.err = x.err.Error()
			m.status = "swarm"
			m.swarmNotice = "spawn error: " + x.err.Error()
			return m, nil
		}
		m.err = ""
		m.status = "swarm"
		m.swarmStatus = x.status
		m.swarmNotice = "agent spawned; status refreshed"
		if len(m.swarmStatus.Agents) > 0 {
			m.selectedSwarmAgent = len(m.swarmStatus.Agents) - 1
		}
		return m, nil
	case swarmAttachTickMsg:
		if !m.swarmAttached || x.id != m.swarmAttachTickID || m.dialog != dialogSwarm {
			return m, nil
		}
		if m.running {
			return m, m.swarmAttachTick()
		}
		m.running = true
		m.swarmNotice = "auto-refreshing attached agent"
		return m, m.loadSelectedSwarmStatus()
	case commandDoneMsg:
		m.running = false
		if x.err != nil {
			m.err = x.err.Error()
			m.status = "error"
			m.commandDialogTitle = "Command Error"
			m.commandDialogText = x.err.Error()
			m.dialog = dialogCommand
		} else {
			m.err = ""
			m.status = "ready"
			if x.text != "" {
				m.commandDialogTitle = x.title
				if m.commandDialogTitle == "" {
					m.commandDialogTitle = "Command"
				}
				m.commandDialogText = x.text
				m.dialog = dialogCommand
			}
		}
		m.syncTranscript()
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.updateCommandSuggestions()
	return m, cmd
}

func (m bubbleModel) View() tea.View {
	if m.width == 0 {
		v := tea.NewView("initializing…")
		v.AltScreen = true
		return v
	}
	header := m.headerView()
	searchTitle := m.searchTitleView()
	inputView := m.theme.Input.Width(max(1, m.width-2)).Render(m.input.View())
	commandPopup := m.commandSuggestionView()
	dialog := m.activeDialogView()
	reserved := lipgloss.Height(header) + lipgloss.Height(dialog) + lipgloss.Height(searchTitle) + lipgloss.Height(commandPopup) + lipgloss.Height(inputView) + m.theme.Viewport.GetVerticalFrameSize()
	m.viewport.SetHeight(max(1, m.height-reserved))
	body := lipgloss.JoinVertical(lipgloss.Left,
		header,
		m.theme.Viewport.Width(max(1, m.width-2)).Render(m.viewport.View()),
	)
	if dialog != "" {
		body = lipgloss.JoinVertical(lipgloss.Left, body, dialog)
	}
	if searchTitle != "" {
		body = lipgloss.JoinVertical(lipgloss.Left, body, searchTitle)
	}
	if commandPopup != "" {
		body = lipgloss.JoinVertical(lipgloss.Left, body, commandPopup)
	}
	body = lipgloss.JoinVertical(lipgloss.Left, body, inputView)
	v := tea.NewView(body)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "Erasmus"
	return v
}

func (m bubbleModel) activeDialogView() string {
	switch m.dialog {
	case dialogSessions:
		return m.sessionsDialogView()
	case dialogModel:
		return m.modelDialogView()
	case dialogTree:
		return m.treeDialogView()
	case dialogSwarm:
		return m.swarmDialogView()
	case dialogHelp:
		return m.helpDialogView()
	case dialogCommand:
		return m.commandDialogView()
	default:
		return ""
	}
}

func (m bubbleModel) searchTitleView() string {
	if m.searchActive {
		return m.theme.Help.Render("search: " + m.searchQuery + " · Enter find · Esc close")
	}
	if len(m.searchMatches) == 0 {
		return ""
	}
	return m.theme.Help.Render(fmt.Sprintf("search %d/%d · n/N next/previous", m.searchIndex+1, len(m.searchMatches)))
}

func (m *bubbleModel) resize() {
	inputHeight := 8
	headerHeight := 2
	available := m.height - inputHeight - headerHeight - 1
	if available < 1 {
		available = 1
	}
	m.viewport.SetWidth(max(10, m.width-6))
	m.viewport.SetHeight(available)
	m.input.SetWidth(max(10, m.width-6))
	m.input.SetHeight(4)
	m.syncTranscript()
}

func (m bubbleModel) headerView() string {
	state := m.app.Harness.State(m.ctx)
	pill := m.theme.ReadyPill.Render(m.status)
	if m.running {
		pill = m.theme.RunningPill.Render("running")
	}
	if m.err != "" || m.status == "error" {
		pill = m.theme.ErrorPill.Render("error")
	}
	reasoning := state.Agent.Reasoning
	if reasoning == "" {
		reasoning = "default"
	}
	left := m.theme.Brand.Render("Erasmus") + " " + pill
	right := m.theme.Muted.Render(fmt.Sprintf("%s/%s · reasoning %s · session %s · ? for help", state.Agent.Model.Provider, state.Agent.Model.ID, reasoning, state.Session.ID))
	if !m.follow {
		right += m.theme.Muted.Render(fmt.Sprintf(" · scrollback %.0f%% · End to follow", m.viewport.ScrollPercent()*100))
	}
	line := lipgloss.JoinHorizontal(lipgloss.Center, left, "  ", right)
	if m.err != "" {
		line += "  " + m.theme.Error.Render(m.err)
	}
	return m.theme.Header.Width(max(1, m.width-2)).Render(line)
}

func (m bubbleModel) handleMouse(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseWheelUp:
		m.viewport.ScrollUp(3)
		m.follow = false
		return m, nil
	case tea.MouseWheelDown:
		m.viewport.ScrollDown(3)
		m.follow = m.viewport.AtBottom()
		return m, nil
	}
	return m, nil
}

func (m bubbleModel) handleViewportKey(key tea.KeyPressMsg) (bubbleModel, tea.Cmd, bool) {
	switch key.String() {
	case "pgup":
		m.viewport.PageUp()
		m.follow = false
		return m, nil, true
	case "pgdown":
		m.viewport.PageDown()
		m.follow = m.viewport.AtBottom()
		return m, nil, true
	case "ctrl+u":
		m.viewport.HalfPageUp()
		m.follow = false
		return m, nil, true
	case "ctrl+d":
		m.viewport.HalfPageDown()
		m.follow = m.viewport.AtBottom()
		return m, nil, true
	case "home":
		m.viewport.GotoTop()
		m.follow = false
		return m, nil, true
	case "end":
		m.viewport.GotoBottom()
		m.follow = true
		return m, nil, true
	}
	return m, nil, false
}

func (m *bubbleModel) submit() (bubbleModel, tea.Cmd) {
	if m.running {
		return *m, nil
	}
	text := strings.TrimSpace(m.input.Value())
	if text == "" {
		return *m, nil
	}
	m.input.Reset()
	m.clearCommandSuggestions()
	if strings.HasPrefix(text, "/") {
		m.running = true
		m.status = "command"
		return *m, m.runCommand(text)
	}
	m.running = true
	m.follow = true
	m.assistantOpen = false
	m.status = "running"
	m.err = ""
	m.appendUserMessage(text)
	m.syncTranscript()
	ch := make(chan tea.Msg, 64)
	m.messages = ch
	go func() {
		events, err := m.app.Harness.Prompt(m.ctx, text, harness.PromptOptions{})
		if err != nil {
			ch <- runtimeDoneMsg{err: err}
			close(ch)
			return
		}
		for ev := range events {
			ch <- runtimeEventMsg{event: ev}
		}
		ch <- runtimeDoneMsg{err: m.app.Harness.Wait(m.ctx)}
		close(ch)
	}()
	return *m, m.waitRuntime()
}

func (m bubbleModel) updateHelpDialog(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "?":
		m.dialog = m.helpReturnDialog
		m.status = m.dialogStatus(m.dialog)
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m bubbleModel) updateCommandDialog(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "enter":
		m.dialog = dialogNone
		m.commandDialogTitle = ""
		m.commandDialogText = ""
		m.status = "ready"
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m bubbleModel) dialogStatus(dialog dialogMode) string {
	switch dialog {
	case dialogSessions:
		return "sessions"
	case dialogModel:
		return "model"
	case dialogTree:
		return "tree"
	case dialogSwarm:
		return "swarm"
	case dialogCommand:
		return "command"
	default:
		return "ready"
	}
}

func (m bubbleModel) helpDialogView() string {
	var b strings.Builder
	b.WriteString("Help — ?/esc close")
	if m.helpReturnDialog != dialogNone {
		b.WriteString(" · returns to " + m.dialogName(m.helpReturnDialog))
	}
	b.WriteString("\n\n")
	m.writeCurrentModeHelp(&b)
	b.WriteString("Global\n")
	b.WriteString("  enter         submit prompt / send dialog input\n")
	b.WriteString("  shift+enter   insert newline\n")
	b.WriteString("  ctrl+enter    insert newline when supported\n")
	b.WriteString("  ctrl+o        sessions\n")
	b.WriteString("  ctrl+p        model + reasoning\n")
	b.WriteString("  ctrl+t        session tree\n")
	b.WriteString("  ctrl+w        swarm dashboard\n")
	b.WriteString("  ?             context help\n")
	b.WriteString("  ctrl+c        quit\n\n")
	b.WriteString("Transcript\n")
	b.WriteString("  ctrl+f        search transcript\n")
	b.WriteString("  enter         run search while search prompt is open\n")
	b.WriteString("  n/N           next / previous match\n")
	b.WriteString("  esc           close search prompt\n")
	b.WriteString("  PgUp/PgDn     scroll page\n")
	b.WriteString("  ctrl+u/d      scroll half page\n")
	b.WriteString("  Home/End      top / resume follow at bottom\n")
	b.WriteString("  mouse wheel   scrollback\n")
	return m.renderDialog(b.String())
}

func (m bubbleModel) commandDialogView() string {
	title := strings.TrimSpace(m.commandDialogTitle)
	if title == "" {
		title = "Command"
	}
	text := strings.TrimSpace(m.commandDialogText)
	if text == "" {
		text = "No output."
	}
	var b strings.Builder
	b.WriteString(title + " — enter/esc close\n\n")
	b.WriteString(text)
	if !strings.HasSuffix(text, "\n") {
		b.WriteString("\n")
	}
	return m.renderDialog(b.String())
}

func (m bubbleModel) renderDialog(content string) string {
	style := m.theme.Dialog.Width(max(1, m.width-2))
	if m.height > 0 {
		style = style.MaxHeight(m.maxDialogHeight())
	}
	return style.Render(content)
}

func (m bubbleModel) maxDialogHeight() int {
	header := m.headerView()
	searchTitle := m.searchTitleView()
	inputView := m.theme.Input.Width(max(1, m.width-2)).Render(m.input.View())
	minViewport := 1 + m.theme.Viewport.GetVerticalFrameSize()
	return max(1, m.height-lipgloss.Height(header)-lipgloss.Height(searchTitle)-lipgloss.Height(inputView)-minViewport)
}

func (m bubbleModel) dialogName(dialog dialogMode) string {
	switch dialog {
	case dialogSessions:
		return "sessions"
	case dialogModel:
		return "model"
	case dialogTree:
		return "session tree"
	case dialogSwarm:
		if m.swarmAttached {
			return "swarm attach"
		}
		return "swarm dashboard"
	case dialogCommand:
		return "command"
	default:
		return "chat"
	}
}

func (m bubbleModel) writeCurrentModeHelp(b *strings.Builder) {
	b.WriteString("Current mode: " + m.dialogName(m.helpReturnDialog) + "\n")
	switch m.helpReturnDialog {
	case dialogSessions:
		b.WriteString("  ↑/↓ or k/j    select session\n")
		b.WriteString("  enter         open selected session\n")
		b.WriteString("  esc/ctrl+o    close sessions\n\n")
	case dialogModel:
		b.WriteString("  ↑/↓ or k/j    select model\n")
		b.WriteString("  ←/→ or h/l    select reasoning\n")
		b.WriteString("  enter         apply model/reasoning\n")
		b.WriteString("  esc/ctrl+p    close model picker\n\n")
	case dialogTree:
		b.WriteString("  ↑/↓ or k/j    select tree entry\n")
		b.WriteString("  enter         move session leaf to selected entry\n")
		b.WriteString("  esc/ctrl+t    close tree browser\n\n")
	case dialogSwarm:
		m.writeSwarmHelp(b)
	case dialogCommand:
		b.WriteString("  enter/esc     close command output\n\n")
	default:
		b.WriteString("  type prompt   compose in input box\n")
		b.WriteString("  enter         submit prompt\n")
		b.WriteString("  shift+enter   insert newline\n")
		b.WriteString("  /             show command suggestions\n")
		for _, cmd := range slashCommands() {
			fmt.Fprintf(b, "  %-13s %s\n", cmd.Usage, cmd.Description)
		}
		b.WriteString("\n")
	}
}

func (m bubbleModel) handleCommandPopupKey(key tea.KeyPressMsg) (bubbleModel, tea.Cmd, bool) {
	if !m.commandPopup {
		return m, nil, false
	}
	switch key.String() {
	case "tab":
		m.acceptCommandSuggestion()
		return m, nil, true
	case "enter":
		if exactSlashCommand(m.input.Value()) {
			m.clearCommandSuggestions()
			updated, cmd := m.submit()
			return updated, cmd, true
		}
		m.acceptCommandSuggestion()
		return m, nil, true
	case "down":
		m.selectedCommandSuggestion = (m.selectedCommandSuggestion + 1) % len(m.commandSuggestions)
		return m, nil, true
	case "up":
		m.selectedCommandSuggestion--
		if m.selectedCommandSuggestion < 0 {
			m.selectedCommandSuggestion = len(m.commandSuggestions) - 1
		}
		return m, nil, true
	case "esc":
		m.clearCommandSuggestions()
		return m, nil, true
	}
	return m, nil, false
}

func isMultilineInputKey(msg tea.KeyPressMsg) bool {
	key := msg.Key()
	return key.Code == tea.KeyEnter && (key.Mod&tea.ModShift != 0 || key.Mod&tea.ModCtrl != 0)
}

func exactSlashCommand(value string) bool {
	name := strings.TrimSpace(value)
	if name == "" {
		return false
	}
	for _, cmd := range slashCommands() {
		if name == cmd.Name {
			return true
		}
	}
	return false
}

func (m *bubbleModel) updateCommandSuggestions() {
	value := m.input.Value()
	if !strings.HasPrefix(value, "/") || strings.ContainsAny(value, " \t\r\n") {
		m.clearCommandSuggestions()
		return
	}
	suggestions := make([]slashCommand, 0, len(slashCommands()))
	for _, cmd := range slashCommands() {
		if strings.HasPrefix(cmd.Name, value) {
			suggestions = append(suggestions, cmd)
		}
	}
	if len(suggestions) == 0 {
		m.clearCommandSuggestions()
		return
	}
	m.commandPopup = true
	m.commandSuggestions = suggestions
	if m.selectedCommandSuggestion >= len(suggestions) {
		m.selectedCommandSuggestion = len(suggestions) - 1
	}
	if m.selectedCommandSuggestion < 0 {
		m.selectedCommandSuggestion = 0
	}
}

func (m *bubbleModel) clearCommandSuggestions() {
	m.commandPopup = false
	m.commandSuggestions = nil
	m.selectedCommandSuggestion = 0
}

func (m *bubbleModel) acceptCommandSuggestion() {
	if len(m.commandSuggestions) == 0 {
		m.clearCommandSuggestions()
		return
	}
	selected := m.commandSuggestions[m.selectedCommandSuggestion]
	m.input.SetValue(selected.Name + " ")
	m.input.CursorEnd()
	m.clearCommandSuggestions()
}

func (m bubbleModel) commandSuggestionView() string {
	if !m.commandPopup || len(m.commandSuggestions) == 0 {
		return ""
	}
	limit := min(6, len(m.commandSuggestions))
	lines := make([]string, 0, limit+1)
	lines = append(lines, "Commands")
	maxLineWidth := max(20, m.width-10)
	for i := 0; i < limit; i++ {
		cmd := m.commandSuggestions[i]
		line := fmt.Sprintf("%-22s %s", cmd.Usage, cmd.Description)
		if len(line) > maxLineWidth {
			line = line[:max(0, maxLineWidth-3)] + "..."
		}
		if i == m.selectedCommandSuggestion {
			line = m.theme.Selected.Render(line)
		}
		lines = append(lines, line)
	}
	if len(m.commandSuggestions) > limit {
		lines = append(lines, fmt.Sprintf("+%d more", len(m.commandSuggestions)-limit))
	}
	maxHeight := 5
	if m.height >= 24 {
		maxHeight = 8
	}
	return m.theme.Dialog.Width(max(1, m.width-2)).MaxHeight(maxHeight).Render(strings.Join(lines, "\n"))
}

func (m bubbleModel) writeSwarmHelp(b *strings.Builder) {
	if m.swarmAttached {
		b.WriteString("  attached mode auto-refreshes status/log every 2s\n")
		b.WriteString("  s             send prompt to attached agent\n")
		b.WriteString("  enter         refresh status/log now\n")
		b.WriteString("  tab/shift+tab switch attached agent\n")
		b.WriteString("  esc           detach\n")
		b.WriteString("  ctrl+w        close dashboard\n\n")
		return
	}
	b.WriteString("  ↑/↓ or k/j    select server\n")
	b.WriteString("  tab/shift+tab select agent\n")
	b.WriteString("  a             attach/detach selected agent\n")
	b.WriteString("  n             spawn new agent on selected server\n")
	b.WriteString("  s             send prompt to selected agent\n")
	b.WriteString("  x             stop selected agent\n")
	b.WriteString("  l             load selected agent log tail\n")
	b.WriteString("  enter         refresh selected server\n")
	b.WriteString("  r             reload registry\n")
	b.WriteString("  esc/ctrl+w    close dashboard\n\n")
}

func (m *bubbleModel) startSearch() {
	m.searchActive = true
	m.searchQuery = ""
	m.searchMatches = nil
	m.searchIndex = 0
	m.status = "search"
	m.err = ""
	m.input.Blur()
}

func (m bubbleModel) updateSearch(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.searchActive = false
		m.status = "ready"
		m.input.Focus()
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "enter":
		m.runSearch()
		return m, nil
	case "backspace":
		if m.searchQuery != "" {
			runes := []rune(m.searchQuery)
			m.searchQuery = string(runes[:len(runes)-1])
		}
		return m, nil
	case "n":
		m.nextSearchMatch()
		return m, nil
	case "N":
		m.previousSearchMatch()
		return m, nil
	}
	if key.Text != "" {
		m.searchQuery += key.Text
		return m, nil
	}
	return m, nil
}

func (m *bubbleModel) runSearch() {
	query := strings.TrimSpace(m.searchQuery)
	m.searchMatches = nil
	m.searchIndex = 0
	if query == "" {
		m.status = "search"
		return
	}
	needle := strings.ToLower(query)
	for i, line := range strings.Split(m.transcript, "\n") {
		if strings.Contains(strings.ToLower(line), needle) {
			m.searchMatches = append(m.searchMatches, i)
		}
	}
	if len(m.searchMatches) == 0 {
		m.status = "no matches"
		return
	}
	m.status = fmt.Sprintf("match %d/%d", m.searchIndex+1, len(m.searchMatches))
	m.gotoSearchMatch()
}

func (m *bubbleModel) nextSearchMatch() {
	if len(m.searchMatches) == 0 {
		m.runSearch()
		return
	}
	m.searchIndex = (m.searchIndex + 1) % len(m.searchMatches)
	m.status = fmt.Sprintf("match %d/%d", m.searchIndex+1, len(m.searchMatches))
	m.gotoSearchMatch()
}

func (m *bubbleModel) previousSearchMatch() {
	if len(m.searchMatches) == 0 {
		m.runSearch()
		return
	}
	m.searchIndex = (m.searchIndex - 1 + len(m.searchMatches)) % len(m.searchMatches)
	m.status = fmt.Sprintf("match %d/%d", m.searchIndex+1, len(m.searchMatches))
	m.gotoSearchMatch()
}

func (m *bubbleModel) gotoSearchMatch() {
	if len(m.searchMatches) == 0 {
		return
	}
	m.follow = false
	m.viewport.SetYOffset(m.searchMatches[m.searchIndex])
}

func (m *bubbleModel) openSessionsDialog() (bubbleModel, tea.Cmd) {
	if m.running || m.app.ListSessions == nil {
		if m.app.ListSessions == nil {
			m.err = "session listing is not configured"
			m.status = "error"
		}
		return *m, nil
	}
	m.running = true
	m.status = "loading sessions"
	m.err = ""
	return *m, func() tea.Msg {
		sessions, err := m.app.ListSessions(m.ctx, "")
		return sessionsLoadedMsg{sessions: sessions, err: err}
	}
}

func (m bubbleModel) updateSessionsDialog(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "ctrl+o":
		m.dialog = dialogNone
		m.status = "ready"
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.selectedSession > 0 {
			m.selectedSession--
		}
	case "down", "j":
		if m.selectedSession < len(m.sessions)-1 {
			m.selectedSession++
		}
	case "enter":
		if len(m.sessions) == 0 || m.app.OpenSession == nil {
			return m, nil
		}
		path := m.sessions[m.selectedSession].Path
		m.running = true
		m.status = "opening session"
		return m, func() tea.Msg {
			var buf bytes.Buffer
			err := m.app.openSession(m.ctx, &buf, path)
			return sessionOpenedMsg{path: path, err: err}
		}
	}
	return m, nil
}

func (m bubbleModel) sessionsDialogView() string {
	var b strings.Builder
	b.WriteString("Sessions — ↑/↓ select · enter open · esc close\n")
	if len(m.sessions) == 0 {
		b.WriteString("no sessions found")
		return m.renderDialog(b.String())
	}
	start := 0
	if m.selectedSession > 8 {
		start = m.selectedSession - 8
	}
	end := min(len(m.sessions), start+10)
	for i := start; i < end; i++ {
		entry := m.sessions[i]
		line := fmt.Sprintf("  %s  id=%s  messages=%d  updated=%s", entry.Path, entry.ID, entry.Messages, entry.Updated)
		if i == m.selectedSession {
			line = m.theme.Selected.Render("▸ " + strings.TrimSpace(line))
		}
		b.WriteString(line + "\n")
	}
	return m.renderDialog(b.String())
}

func (m *bubbleModel) renderSessionTranscript() {
	state := m.app.Harness.State(m.ctx)
	for _, msg := range state.Agent.Messages {
		switch msg.Role {
		case "user":
			m.appendUserMessage(messageText(msg))
		case "assistant":
			m.appendAssistantMessage(messageText(msg))
		default:
			m.appendLine(roleLabel(string(msg.Role)) + ": " + messageText(msg))
		}
	}
}

func (m *bubbleModel) openModelDialog() (bubbleModel, tea.Cmd) {
	if m.running {
		return *m, nil
	}
	state := m.app.Harness.State(m.ctx)
	m.setModelDialogProvider(state.Agent.Model.Provider, state.Agent.Model)
	m.selectedReasoning = 0
	for i, level := range m.reasoningLevels {
		if level == state.Agent.Reasoning {
			m.selectedReasoning = i
			break
		}
	}
	m.dialog = dialogModel
	m.status = "model"
	m.err = ""
	return *m, nil
}

func (m bubbleModel) updateModelDialog(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "ctrl+p":
		m.dialog = dialogNone
		m.status = "ready"
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "tab":
		m.cycleModelProvider(1)
	case "shift+tab":
		m.cycleModelProvider(-1)
	case "up", "k":
		if m.selectedModel > 0 {
			m.selectedModel--
		}
	case "down", "j":
		if m.selectedModel < len(m.models)-1 {
			m.selectedModel++
		}
	case "left", "h":
		if m.selectedReasoning > 0 {
			m.selectedReasoning--
		}
	case "right", "l":
		if m.selectedReasoning < len(m.reasoningLevels)-1 {
			m.selectedReasoning++
		}
	case "enter":
		if len(m.models) == 0 {
			return m, nil
		}
		selectedModel := m.models[m.selectedModel]
		reasoning := m.reasoningLevels[m.selectedReasoning]
		m.running = true
		m.status = "applying model"
		return m, func() tea.Msg {
			if m.app.ApplyModel != nil {
				return modelAppliedMsg{err: m.app.ApplyModel(m.ctx, selectedModel, reasoning)}
			}
			if selectedModel.Provider == m.app.Harness.State(m.ctx).Agent.Model.Provider {
				if err := m.app.Harness.SetModel(m.ctx, selectedModel); err != nil {
					return modelAppliedMsg{err: err}
				}
				if err := m.app.Harness.SetReasoning(m.ctx, reasoning); err != nil {
					return modelAppliedMsg{err: err}
				}
				return modelAppliedMsg{}
			}
			return modelAppliedMsg{err: fmt.Errorf("provider switching is not configured")}
		}
	}
	return m, nil
}

func (m *bubbleModel) setModelDialogProvider(providerID string, current model.Model) {
	models := model.DefaultCatalog().ListProvider(providerID)
	if len(models) == 0 && current.Provider == providerID {
		models = []model.Model{current}
	}
	m.models = models
	m.selectedModel = 0
	for i, candidate := range models {
		if candidate.Provider == current.Provider && candidate.ID == current.ID {
			m.selectedModel = i
			break
		}
	}
}

func (m *bubbleModel) cycleModelProvider(delta int) {
	providers := modelDialogProviders()
	if len(providers) == 0 {
		return
	}
	current := ""
	if len(m.models) > 0 {
		current = m.models[m.selectedModel].Provider
	} else {
		current = m.app.Harness.State(m.ctx).Agent.Model.Provider
	}
	idx := 0
	for i, providerID := range providers {
		if providerID == current {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(providers)) % len(providers)
	m.setModelDialogProvider(providers[idx], model.Model{})
}

func modelDialogProviders() []string {
	seen := map[string]bool{}
	var providers []string
	for _, candidate := range model.DefaultCatalog().List() {
		if candidate.Provider == "" || seen[candidate.Provider] {
			continue
		}
		seen[candidate.Provider] = true
		providers = append(providers, candidate.Provider)
	}
	return providers
}

func (m bubbleModel) modelDialogView() string {
	var b strings.Builder
	providerID := ""
	if len(m.models) > 0 {
		providerID = m.models[m.selectedModel].Provider
	}
	b.WriteString("Model — tab provider · ↑/↓ select model · ←/→ reasoning · enter apply · esc close\n")
	if providerID != "" {
		fmt.Fprintf(&b, "Provider: %s\n", providerID)
	}
	for i, candidate := range m.models {
		name := candidate.DisplayName
		if name == "" {
			name = candidate.ID
		}
		line := fmt.Sprintf("  %s/%s  %s", candidate.Provider, candidate.ID, name)
		if i == m.selectedModel {
			line = m.theme.Selected.Render("▸ " + strings.TrimSpace(line))
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\nReasoning: ")
	for i, level := range m.reasoningLevels {
		label := level
		if label == "" {
			label = "default"
		}
		if i == m.selectedReasoning {
			fmt.Fprintf(&b, "[%s] ", label)
		} else {
			fmt.Fprintf(&b, "%s ", label)
		}
	}
	return m.renderDialog(b.String())
}

func (m *bubbleModel) openTreeDialog() (bubbleModel, tea.Cmd) {
	if m.running {
		return *m, nil
	}
	m.running = true
	m.status = "loading tree"
	m.err = ""
	return *m, func() tea.Msg {
		tree, err := m.app.Harness.Tree(m.ctx)
		return treeLoadedMsg{tree: tree, err: err}
	}
}

func (m bubbleModel) updateTreeDialog(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "ctrl+t":
		m.dialog = dialogNone
		m.status = "ready"
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.selectedTree > 0 {
			m.selectedTree--
		}
	case "down", "j":
		if m.selectedTree < len(m.tree.Entries)-1 {
			m.selectedTree++
		}
	case "enter":
		if len(m.tree.Entries) == 0 {
			return m, nil
		}
		id := m.tree.Entries[m.selectedTree].ID
		m.running = true
		m.status = "moving tree"
		return m, func() tea.Msg {
			err := m.app.Harness.MoveTo(m.ctx, id, nil)
			return treeMovedMsg{err: err}
		}
	}
	return m, nil
}

func (m bubbleModel) treeDialogView() string {
	var b strings.Builder
	b.WriteString("Session tree — ↑/↓ select · enter move · esc close\n")
	if len(m.tree.Entries) == 0 {
		b.WriteString("no tree entries")
		return m.renderDialog(b.String())
	}
	start := 0
	if m.selectedTree > 8 {
		start = m.selectedTree - 8
	}
	end := min(len(m.tree.Entries), start+10)
	for i := start; i < end; i++ {
		entry := m.tree.Entries[i]
		leaf := " "
		if entry.ID == m.tree.LeafID {
			leaf = "*"
		}
		when := ""
		if !entry.Time.IsZero() {
			when = entry.Time.Format("15:04:05")
		}
		line := fmt.Sprintf("  %s id=%s parent=%s type=%s time=%s", leaf, entry.ID, entry.Parent, entry.Type, when)
		if i == m.selectedTree {
			line = m.theme.Selected.Render("▸ " + strings.TrimSpace(line))
		}
		b.WriteString(line + "\n")
	}
	return m.renderDialog(b.String())
}

func (m *bubbleModel) openSwarmDialog() (bubbleModel, tea.Cmd) {
	if m.running || m.app.ListSwarms == nil {
		if m.app.ListSwarms == nil {
			m.err = "swarm listing is not configured"
			m.status = "error"
		}
		return *m, nil
	}
	m.running = true
	m.status = "loading swarms"
	m.err = ""
	return *m, func() tea.Msg {
		servers, err := m.app.ListSwarms(m.ctx)
		return swarmsLoadedMsg{servers: servers, err: err}
	}
}

func (m bubbleModel) updateSwarmDialog(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.swarmPromptMode != "" {
		if isMultilineInputKey(key) {
			m.input.InsertString("\n")
			return m, nil
		}
		switch key.String() {
		case "esc":
			m.swarmPromptMode = ""
			m.input.Reset()
			return m, nil
		case "enter":
			return m.submitSwarmPrompt()
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(key)
		return m, cmd
	}
	switch key.String() {
	case "esc":
		if m.swarmAttached {
			m.detachSwarmAgent("detached from swarm agent")
			return m, nil
		}
		m.dialog = dialogNone
		m.status = "ready"
		return m, nil
	case "ctrl+w":
		m.detachSwarmAgent("")
		m.dialog = dialogNone
		m.status = "ready"
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "r":
		m.swarmNotice = "reloading swarm registry"
		return m.openSwarmDialog()
	case "tab":
		if len(m.swarmStatus.Agents) > 0 {
			m.selectedSwarmAgent = (m.selectedSwarmAgent + 1) % len(m.swarmStatus.Agents)
			m.swarmLog = ""
			if m.swarmAttached {
				m.loadSelectedAgentLog()
				m.swarmNotice = "attached to " + m.swarmStatus.Agents[m.selectedSwarmAgent].ID
			}
		}
	case "shift+tab":
		if len(m.swarmStatus.Agents) > 0 {
			m.selectedSwarmAgent--
			if m.selectedSwarmAgent < 0 {
				m.selectedSwarmAgent = len(m.swarmStatus.Agents) - 1
			}
			m.swarmLog = ""
			if m.swarmAttached {
				m.loadSelectedAgentLog()
				m.swarmNotice = "attached to " + m.swarmStatus.Agents[m.selectedSwarmAgent].ID
			}
		}
	case "a":
		return m, m.toggleSwarmAttach()
	case "l":
		m.loadSelectedAgentLog()
	case "x":
		return m.stopSelectedSwarmAgent()
	case "s":
		if len(m.swarmStatus.Agents) > 0 && m.app.SwarmSend != nil {
			m.swarmPromptMode = "send"
			m.input.Reset()
			m.input.SetValue("")
		}
	case "n":
		if len(m.swarmServers) > 0 && m.swarmServers[m.selectedSwarm].Reachable && m.app.SwarmSpawn != nil {
			m.swarmPromptMode = "spawn"
			m.input.Reset()
			m.input.SetValue("")
		}
	case "up", "k":
		if m.selectedSwarm > 0 {
			m.selectedSwarm--
			m.swarmStatus = SwarmStatusSummary{}
			m.selectedSwarmAgent = 0
			m.swarmLog = ""
			m.detachSwarmAgent("")
			m.swarmNotice = "selected server " + m.swarmServers[m.selectedSwarm].Name
			if m.swarmServers[m.selectedSwarm].Reachable {
				m.running = true
				return m, m.loadSelectedSwarmStatus()
			}
		}
	case "down", "j":
		if m.selectedSwarm < len(m.swarmServers)-1 {
			m.selectedSwarm++
			m.swarmStatus = SwarmStatusSummary{}
			m.selectedSwarmAgent = 0
			m.swarmLog = ""
			m.detachSwarmAgent("")
			m.swarmNotice = "selected server " + m.swarmServers[m.selectedSwarm].Name
			if m.swarmServers[m.selectedSwarm].Reachable {
				m.running = true
				return m, m.loadSelectedSwarmStatus()
			}
		}
	case "enter":
		if len(m.swarmServers) > 0 && m.swarmServers[m.selectedSwarm].Reachable {
			m.running = true
			if m.swarmAttached {
				m.swarmNotice = "refreshing attached agent"
			} else {
				m.swarmNotice = "refreshing selected server"
			}
			return m, m.loadSelectedSwarmStatus()
		}
	}
	return m, nil
}

func (m *bubbleModel) toggleSwarmAttach() tea.Cmd {
	if len(m.swarmStatus.Agents) == 0 || m.selectedSwarmAgent >= len(m.swarmStatus.Agents) {
		m.swarmNotice = "no selected swarm agent to attach"
		return nil
	}
	if m.swarmAttached {
		m.detachSwarmAgent("detached from " + m.swarmStatus.Agents[m.selectedSwarmAgent].ID)
		return nil
	}
	m.swarmAttached = true
	m.swarmAttachTickID++
	m.loadSelectedAgentLog()
	m.swarmNotice = "attached to " + m.swarmStatus.Agents[m.selectedSwarmAgent].ID
	return m.swarmAttachTick()
}

func (m *bubbleModel) detachSwarmAgent(notice string) {
	m.swarmAttached = false
	m.swarmAttachTickID++
	if notice != "" {
		m.swarmNotice = notice
	}
}

func (m bubbleModel) swarmAttachTick() tea.Cmd {
	id := m.swarmAttachTickID
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return swarmAttachTickMsg{id: id}
	})
}

func (m bubbleModel) stopSelectedSwarmAgent() (tea.Model, tea.Cmd) {
	if len(m.swarmStatus.Agents) == 0 || m.app.SwarmStop == nil || len(m.swarmServers) == 0 {
		return m, nil
	}
	server := m.swarmServers[m.selectedSwarm]
	agent := m.swarmStatus.Agents[m.selectedSwarmAgent]
	m.running = true
	m.status = "stopping swarm agent"
	m.err = ""
	return m, func() tea.Msg {
		status, err := m.app.SwarmStop(m.ctx, server, agent.ID)
		return swarmStoppedMsg{status: status, err: err}
	}
}

func (m *bubbleModel) submitSwarmPrompt() (bubbleModel, tea.Cmd) {
	text := strings.TrimSpace(m.input.Value())
	if text == "" || len(m.swarmServers) == 0 {
		return *m, nil
	}
	server := m.swarmServers[m.selectedSwarm]
	mode := m.swarmPromptMode
	m.input.Reset()
	m.swarmPromptMode = ""
	m.running = true
	m.err = ""
	if mode == "spawn" {
		if m.app.SwarmSpawn == nil {
			m.running = false
			return *m, nil
		}
		m.status = "spawning swarm agent"
		return *m, func() tea.Msg {
			status, err := m.app.SwarmSpawn(m.ctx, server, text)
			return swarmSpawnedMsg{status: status, err: err}
		}
	}
	if len(m.swarmStatus.Agents) == 0 || m.app.SwarmSend == nil {
		m.running = false
		return *m, nil
	}
	agent := m.swarmStatus.Agents[m.selectedSwarmAgent]
	m.status = "sending swarm prompt"
	return *m, func() tea.Msg {
		status, err := m.app.SwarmSend(m.ctx, server, agent.ID, text)
		return swarmSentMsg{status: status, err: err}
	}
}

func (m bubbleModel) loadSelectedSwarmStatus() tea.Cmd {
	return func() tea.Msg {
		if m.app.SwarmStatus == nil || len(m.swarmServers) == 0 {
			return swarmStatusLoadedMsg{}
		}
		status, err := m.app.SwarmStatus(m.ctx, m.swarmServers[m.selectedSwarm])
		return swarmStatusLoadedMsg{status: status, err: err}
	}
}

func (m *bubbleModel) loadSelectedAgentLog() {
	if len(m.swarmStatus.Agents) == 0 || m.selectedSwarmAgent >= len(m.swarmStatus.Agents) {
		m.swarmNotice = "no selected swarm agent"
		return
	}
	agent := m.swarmStatus.Agents[m.selectedSwarmAgent]
	path := agent.EventLog
	if path == "" {
		m.swarmLog = "no event log path for selected agent"
		m.swarmNotice = "no event log for " + agent.ID
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		m.swarmLog = "log error: " + err.Error()
		m.swarmNotice = "log error for " + agent.ID
		return
	}
	m.swarmLog = tailLines(string(data), 12)
	m.swarmNotice = fmt.Sprintf("loaded log tail for %s", agent.ID)
}

func (m bubbleModel) swarmDialogView() string {
	var b strings.Builder
	switch m.swarmPromptMode {
	case "send":
		b.WriteString("Swarm dashboard — type prompt for selected agent · enter send · shift+enter newline · esc cancel\n")
	case "spawn":
		b.WriteString("Swarm dashboard — type task for new agent · enter spawn · shift+enter newline · esc cancel\n")
	default:
		b.WriteString("Swarm dashboard — ↑/↓ server · tab agent · a attach · n spawn · s send · x stop · l logs · enter refresh · r reload · esc close\n")
	}
	if m.swarmNotice != "" {
		b.WriteString("Status: " + m.swarmNotice + "\n")
	}
	if len(m.swarmServers) == 0 {
		b.WriteString("no registered swarm servers")
		return m.renderDialog(b.String())
	}
	for i, server := range m.swarmServers {
		reachable := "stale"
		if server.Reachable {
			reachable = "reachable"
		}
		line := fmt.Sprintf("  %s  %s  %s  agents=%d  %s", server.Name, server.Status, reachable, len(m.swarmStatus.Agents), server.Socket)
		if i != m.selectedSwarm {
			line = fmt.Sprintf("  %s  %s  %s  %s", server.Name, server.Status, reachable, server.Socket)
		}
		if i == m.selectedSwarm {
			line = m.theme.Selected.Render("▸ " + strings.TrimSpace(line))
		}
		b.WriteString(line + "\n")
		if i == m.selectedSwarm && server.Error != "" {
			fmt.Fprintf(&b, "    error: %s\n", server.Error)
		}
	}
	if len(m.swarmStatus.Agents) > 0 || m.swarmStatus.Uptime != "" {
		fmt.Fprintf(&b, "\nSelected: pid=%d uptime=%s cwd=%s model=%s/%s\n", m.swarmStatus.PID, m.swarmStatus.Uptime, m.swarmStatus.CWD, m.swarmStatus.Provider, m.swarmStatus.Model)
		b.WriteString("Agents:\n")
		for i, agent := range m.swarmStatus.Agents {
			state := agent.State
			if state == "" {
				state = "idle"
				if agent.Running {
					state = "running"
				}
			}
			modelName := agent.Model
			if agent.Provider != "" && agent.Model != "" {
				modelName = agent.Provider + "/" + agent.Model
			}
			lastEvent := agent.LastEventType
			if !agent.LastEventAt.IsZero() {
				lastEvent = fmt.Sprintf("%s@%s", lastEvent, agent.LastEventAt.Format("15:04:05"))
			}
			line := fmt.Sprintf("  %s  %s  msgs=%d tools=%d events=%d last=%s session=%s model=%s  %s", agent.ID, state, agent.Messages, agent.PendingTools, agent.Events, lastEvent, agent.SessionID, modelName, agent.Task)
			if i == m.selectedSwarmAgent {
				line = m.theme.Selected.Render("▸ " + strings.TrimSpace(line))
			}
			b.WriteString(line + "\n")
		}
	}
	if m.swarmAttached && len(m.swarmStatus.Agents) > 0 {
		fmt.Fprintf(&b, "\nAttached to %s — auto-refresh 2s · s send · enter refresh now · tab switch agent · esc detach.\n", m.swarmStatus.Agents[m.selectedSwarmAgent].ID)
	}
	if m.swarmPromptMode == "send" && len(m.swarmStatus.Agents) > 0 {
		fmt.Fprintf(&b, "\nPrompting agent %s; compose in input box below.\n", m.swarmStatus.Agents[m.selectedSwarmAgent].ID)
	}
	if m.swarmPromptMode == "spawn" {
		b.WriteString("\nComposing task for a new swarm agent in the input box below.\n")
	}
	if m.swarmLog != "" {
		b.WriteString("\nLog tail:\n")
		b.WriteString(m.swarmLog)
		if !strings.HasSuffix(m.swarmLog, "\n") {
			b.WriteString("\n")
		}
	}
	return m.renderDialog(b.String())
}

func (m *bubbleModel) runCommand(line string) tea.Cmd {
	return func() tea.Msg {
		if line == "/quit" || line == "/exit" {
			return tea.Quit()
		}
		var buf bytes.Buffer
		handled, err := m.app.handleCommand(m.ctx, &buf, line)
		if !handled {
			err = fmt.Errorf("unknown command %q", line)
		}
		return commandDoneMsg{title: commandDialogTitle(line), text: strings.TrimSpace(buf.String()), err: err}
	}
}

func commandDialogTitle(line string) string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "Command"
	}
	switch fields[0] {
	case "/status", "/state":
		return "Status"
	case "/model":
		return "Model"
	case "/messages", "/transcript":
		return "Messages"
	case "/tree":
		return "Session Tree"
	case "/sessions":
		return "Sessions"
	case "/compact":
		return "Compaction"
	case "/open":
		return "Session"
	case "/move":
		return "Session Tree"
	case "/branch":
		return "Branch"
	case "/help":
		return "Commands"
	default:
		return "Command"
	}
}

func (m *bubbleModel) waitRuntime() tea.Cmd {
	ch := m.messages
	return func() tea.Msg {
		if ch == nil {
			return runtimeDoneMsg{}
		}
		msg, ok := <-ch
		if !ok {
			return runtimeDoneMsg{}
		}
		return msg
	}
}

func (m *bubbleModel) applyEvent(ev event.Event) {
	switch e := ev.(type) {
	case event.MessageStart:
		if e.Message.Role == "assistant" && !m.assistantOpen {
			m.appendAssistantStart()
		}
	case event.MessageDelta:
		if !m.assistantOpen {
			m.appendAssistantStart()
		}
		m.transcript += e.Text
	case event.ToolExecutionStart:
		m.appendLine(fmt.Sprintf("\n`tool start` **%s** %s", e.Name, strings.TrimSpace(string(e.Args))))
	case event.ToolExecutionProgress:
		if strings.TrimSpace(e.Text) != "" {
			m.appendLine(fmt.Sprintf("`tool progress` %s %s", e.ID, e.Text))
		}
	case event.ToolExecutionEnd:
		status := "done"
		if e.IsError {
			status = "error"
		}
		m.appendLine(fmt.Sprintf("`tool end` **%s** %s", e.Name, status))
	case event.AgentEnd:
		m.assistantOpen = false
		m.appendLine("")
	case event.ModelUpdate:
		m.appendLine(fmt.Sprintf("[model] %s/%s", e.Model.Provider, e.Model.ID))
	case event.ReasoningUpdate:
		m.appendLine("[reasoning] " + e.Reasoning)
	case event.SessionCompact:
		m.appendLine("[compact] " + e.Summary)
	case event.ResourcesUpdate:
		m.appendLine(fmt.Sprintf("[resources] %d skills", len(e.Skills)))
	}
	m.syncTranscript()
}

func roleLabel(role string) string {
	return "**" + role + "**"
}

func (m *bubbleModel) appendUserMessage(text string) {
	m.ensureBlockBreak()
	m.transcript += "> " + strings.ReplaceAll(strings.TrimSpace(text), "\n", "\n> ") + "\n\n"
}

func (m *bubbleModel) appendAssistantStart() {
	m.ensureBlockBreak()
	m.assistantOpen = true
}

func (m *bubbleModel) appendAssistantMessage(text string) {
	m.ensureBlockBreak()
	m.transcript += strings.TrimSpace(text) + "\n\n"
}

func (m *bubbleModel) ensureBlockBreak() {
	m.transcript = strings.TrimRight(m.transcript, "\n")
	if m.transcript != "" {
		m.transcript += "\n\n"
	}
}

func (m *bubbleModel) appendLine(s string) {
	if m.transcript != "" && !strings.HasSuffix(m.transcript, "\n") {
		m.transcript += "\n"
	}
	m.transcript += s
	if !strings.HasSuffix(s, "\n") {
		m.transcript += "\n"
	}
}

func (m *bubbleModel) syncTranscript() {
	m.renderedTranscript = m.renderTranscript()
	m.viewport.SetContent(m.renderedTranscript)
	if m.follow {
		m.viewport.GotoBottom()
	}
}

func (m *bubbleModel) renderTranscript() string {
	if m.renderer == nil {
		return m.transcript
	}
	rendered, err := m.renderer.Render(m.transcript)
	if err != nil {
		return m.transcript
	}
	return strings.TrimRight(rendered, "\n")
}

func (a *App) runBubble(ctx context.Context, in io.Reader, out io.Writer) error {
	p := tea.NewProgram(newBubbleModel(ctx, a), tea.WithInput(in), tea.WithOutput(out))
	_, err := p.Run()
	return err
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func tailLines(text string, n int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
