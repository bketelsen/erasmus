package tui

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/bketelsen/erasmus/packages/harness"
	"github.com/bketelsen/erasmus/packages/message"
	"github.com/bketelsen/erasmus/packages/model"
	"github.com/bketelsen/erasmus/packages/provider"
	"github.com/bketelsen/erasmus/packages/session/memory"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestAppRunPromptStateQuit(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("tui-test"),
		Model:   model.Model{Provider: "fake", ID: "echo"},
		Stream: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
			ch := make(chan provider.Event, 3)
			ch <- provider.MessageStart{MessageID: "m1"}
			ch <- provider.TextDelta{Text: "tui response: " + lastUserText(req)}
			ch <- provider.MessageEnd{StopReason: "end_turn"}
			close(ch)
			return ch, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	app := &App{Harness: h, In: strings.NewReader("hello\n/help\n/state\n/model\n/tree\n/move 1 back\n/branch 1\n/compact\n/quit\n"), Out: &out}
	if err := app.Run(ctx); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"Erasmus TUI MVP", "you: hello", "assistant: tui response: hello", "commands:", "session: tui-test", "messages: 2", "model=fake/echo", "model: fake/echo", "leaf=", "entries=2", "branch session=", "compacted:", "bye"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestModelDialogTabsBetweenProviders(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("tui-model-test"),
		Model:   model.Model{Provider: "fake", ID: "echo"},
		Stream: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
			return streamText("ok"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := newBubbleModel(ctx, &App{Harness: h})
	opened, _ := m.openModelDialog()
	if len(opened.models) == 0 || opened.models[opened.selectedModel].Provider != "fake" {
		t.Fatalf("opened provider = %q, want fake", opened.models[opened.selectedModel].Provider)
	}
	updated, _ := opened.updateModelDialog(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	afterTab := updated.(bubbleModel)
	if len(afterTab.models) == 0 || afterTab.models[afterTab.selectedModel].Provider != "openai" {
		t.Fatalf("tab provider = %q, want openai", afterTab.models[afterTab.selectedModel].Provider)
	}
	updated, _ = afterTab.updateModelDialog(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
	afterShiftTab := updated.(bubbleModel)
	if len(afterShiftTab.models) == 0 || afterShiftTab.models[afterShiftTab.selectedModel].Provider != "fake" {
		t.Fatalf("shift+tab provider = %q, want fake", afterShiftTab.models[afterShiftTab.selectedModel].Provider)
	}
}

func TestModelDialogApplyUsesAppCallback(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("tui-model-apply-test"),
		Model:   model.Model{Provider: "fake", ID: "echo"},
		Stream: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
			return streamText("ok"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var applied model.Model
	var appliedReasoning string
	app := &App{
		Harness: h,
		ApplyModel: func(ctx context.Context, selected model.Model, reasoning string) error {
			applied = selected
			appliedReasoning = reasoning
			return nil
		},
	}
	m := newBubbleModel(ctx, app)
	opened, _ := m.openModelDialog()
	updated, _ := opened.updateModelDialog(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	afterTab := updated.(bubbleModel)
	updated, _ = afterTab.updateModelDialog(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	afterReasoning := updated.(bubbleModel)
	_, cmd := afterReasoning.updateModelDialog(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil {
		t.Fatal("expected apply command")
	}
	if msg := cmd(); msg != (modelAppliedMsg{}) {
		t.Fatalf("apply msg = %#v, want empty modelAppliedMsg", msg)
	}
	if applied.Provider != "openai" || applied.ID != "gpt-4o-mini" {
		t.Fatalf("applied model = %s/%s, want openai/gpt-4o-mini", applied.Provider, applied.ID)
	}
	if appliedReasoning != "low" {
		t.Fatalf("applied reasoning = %q, want low", appliedReasoning)
	}
}

func TestNamedBubbleThemeIncludesAdditionalBuiltins(t *testing.T) {
	tests := []struct {
		name    string
		want    string
		glamour string
	}{
		{name: "light", want: "light", glamour: "light"},
		{name: "high-contrast", want: "high-contrast", glamour: "notty"},
		{name: "contrast", want: "high-contrast", glamour: "notty"},
		{name: "dracula", want: "dracula", glamour: "dracula"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			theme := namedBubbleTheme(tt.name)
			if theme.Name != tt.want || theme.Glamour != tt.glamour {
				t.Fatalf("theme = %q/%q, want %q/%q", theme.Name, theme.Glamour, tt.want, tt.glamour)
			}
		})
	}
}

func TestBubbleModelUsesConfiguredTheme(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("tui-theme-test"),
		Model:   model.Model{Provider: "fake", ID: "echo"},
		Stream: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
			return streamText("ok"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := newBubbleModel(ctx, &App{Harness: h, Theme: "dracula"})
	if m.theme.Name != "dracula" || m.theme.Glamour != "dracula" {
		t.Fatalf("theme = %q/%q, want dracula/dracula", m.theme.Name, m.theme.Glamour)
	}
}

func TestTranscriptSearchNavigatesMatches(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("tui-search-test"),
		Model:   model.Model{Provider: "fake", ID: "echo"},
		Stream: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
			return streamText("ok"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := newBubbleModel(ctx, &App{Harness: h})
	m.renderer = nil
	m.width = 80
	m.height = 12
	for _, line := range []string{"alpha one", "alpha two", "alpha three", "alpha four", "bravo target", "charlie", "delta target", "echo", "foxtrot", "golf", "hotel", "india"} {
		m.appendLine(line)
	}
	m.resize()

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'f', Mod: tea.ModCtrl}))
	searching := updated.(bubbleModel)
	if !searching.searchActive || searching.status != "search" {
		t.Fatalf("search state = active:%v status:%q", searching.searchActive, searching.status)
	}
	for _, r := range "target" {
		updated, _ = searching.Update(tea.KeyPressMsg(tea.Key{Text: string(r), Code: r}))
		searching = updated.(bubbleModel)
	}
	updated, _ = searching.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	found := updated.(bubbleModel)
	if got, want := found.searchQuery, "target"; got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}
	if len(found.searchMatches) != 2 || found.searchIndex != 0 {
		t.Fatalf("matches = %v index=%d", found.searchMatches, found.searchIndex)
	}
	firstOffset := found.viewport.YOffset()
	if firstOffset == 0 {
		t.Fatal("search did not move viewport to first match")
	}

	updated, _ = found.Update(tea.KeyPressMsg(tea.Key{Text: "n", Code: 'n'}))
	next := updated.(bubbleModel)
	if next.searchIndex != 1 || next.viewport.YOffset() <= firstOffset {
		t.Fatalf("next search index=%d offset=%d first=%d", next.searchIndex, next.viewport.YOffset(), firstOffset)
	}
	updated, _ = next.Update(tea.KeyPressMsg(tea.Key{Text: "N", Code: 'n', ShiftedCode: 'N'}))
	prev := updated.(bubbleModel)
	if prev.searchIndex != 0 || prev.viewport.YOffset() != firstOffset {
		t.Fatalf("previous search index=%d offset=%d first=%d", prev.searchIndex, prev.viewport.YOffset(), firstOffset)
	}
}

func TestSlashHelpCommandStillRunsFullScreen(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("tui-slash-command-test"),
		Model:   model.Model{Provider: "fake", ID: "echo"},
		Stream: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
			return streamText("ok"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := newBubbleModel(ctx, &App{Harness: h})
	m.input.SetValue("/help")
	_, cmd := m.submit()
	if cmd == nil {
		t.Fatal("expected /help command")
	}
	msg := cmd()
	done, ok := msg.(commandDoneMsg)
	if !ok {
		t.Fatalf("command msg = %#v", msg)
	}
	if done.err != nil {
		t.Fatal(done.err)
	}
	if !strings.Contains(done.text, "commands:") || !strings.Contains(done.text, "/help") {
		t.Fatalf("help output = %q", done.text)
	}

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: "/", Code: '/'}))
	got := updated.(bubbleModel)
	if got.searchActive {
		t.Fatal("slash opened search")
	}
}

func TestBubbleHeaderOwnsHelpHintAndInputHasNoCommandBars(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("tui-help-hint-test"),
		Model:   model.Model{Provider: "fake", ID: "echo"},
		Stream: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
			return streamText("ok"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := newBubbleModel(ctx, &App{Harness: h}).Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m := updated.(bubbleModel)
	view := m.View().Content
	if !strings.Contains(m.headerView(), "? for help") {
		t.Fatalf("header = %q", m.headerView())
	}
	for _, unwanted := range []string{"ctrl+s submit", "/help commands", "PgUp/PgDn scroll"} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("view still contains command bar text %q:\n%s", unwanted, view)
		}
	}
	for _, unwanted := range []string{"ctrl+o sessions", "ctrl+p model", "ctrl+t tree", "ctrl+w swarm"} {
		if strings.Contains(m.input.View(), unwanted) {
			t.Fatalf("input view still contains shortcut placeholder text %q: %q", unwanted, m.input.View())
		}
	}
}

func TestBubbleDialogRendersAboveInput(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("tui-dialog-order-test"),
		Model:   model.Model{Provider: "fake", ID: "echo"},
		Stream: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
			return streamText("ok"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := newBubbleModel(ctx, &App{Harness: h}).Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m := updated.(bubbleModel)
	m.dialog = dialogHelp
	view := m.View().Content
	helpIndex := strings.Index(view, "Help")
	inputPromptIndex := strings.LastIndex(view, "┃")
	if helpIndex < 0 || inputPromptIndex < 0 {
		t.Fatalf("view missing help or input prompt:\n%s", view)
	}
	if helpIndex > inputPromptIndex {
		t.Fatalf("help rendered below input:\n%s", view)
	}
}

func TestSlashStatusCommandOpensCommandDialog(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("tui-command-dialog-test"),
		Model:   model.Model{Provider: "fake", ID: "echo"},
		Stream: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
			return streamText("ok"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := newBubbleModel(ctx, &App{Harness: h})
	before := m.transcript
	m.input.SetValue("/status")
	submitted, cmd := m.submit()
	if cmd == nil {
		t.Fatal("expected /status command")
	}
	done, ok := cmd().(commandDoneMsg)
	if !ok {
		t.Fatalf("command msg = %#v", done)
	}
	updated, _ := submitted.Update(done)
	got := updated.(bubbleModel)
	if got.dialog != dialogCommand {
		t.Fatalf("dialog = %v, want command", got.dialog)
	}
	if got.commandDialogTitle != "Status" {
		t.Fatalf("command dialog title = %q, want Status", got.commandDialogTitle)
	}
	for _, want := range []string{"session: tui-command-dialog-test", "messages: 0", "model: fake/echo"} {
		if !strings.Contains(got.commandDialogText, want) {
			t.Fatalf("command dialog missing %q:\n%s", want, got.commandDialogText)
		}
	}
	if got.transcript != before {
		t.Fatalf("transcript changed after command dialog:\nbefore=%q\nafter=%q", before, got.transcript)
	}
}

func TestEnterSubmitsPrompt(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("tui-enter-submit-test"),
		Model:   model.Model{Provider: "fake", ID: "echo"},
		Stream: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
			return streamText("ok"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := newBubbleModel(ctx, &App{Harness: h})
	m.input.SetValue("hello")
	updated, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	got := updated.(bubbleModel)
	if cmd == nil {
		t.Fatal("enter did not submit prompt")
	}
	if !got.running || got.status != "running" {
		t.Fatalf("state after enter = running:%v status:%q", got.running, got.status)
	}
	if !strings.Contains(got.transcript, "> hello") {
		t.Fatalf("transcript after enter submit:\n%s", got.transcript)
	}
}

func TestModifiedEnterInsertsMultilineInput(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("tui-multiline-input-test"),
		Model:   model.Model{Provider: "fake", ID: "echo"},
		Stream: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
			return streamText("ok"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		mod  tea.KeyMod
	}{
		{name: "shift", mod: tea.ModShift},
		{name: "ctrl", mod: tea.ModCtrl},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := newBubbleModel(ctx, &App{Harness: h})
			m.input.SetValue("hello")
			updated, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Mod: tt.mod}))
			got := updated.(bubbleModel)
			if cmd != nil {
				t.Fatal("modified enter submitted instead of inserting a newline")
			}
			if got.running {
				t.Fatal("modified enter started runtime")
			}
			if got.input.Value() != "hello\n" {
				t.Fatalf("input after modified enter = %q, want %q", got.input.Value(), "hello\n")
			}
		})
	}
}

func TestEnterRunsExactSlashCommandWithPopupOpen(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("tui-enter-slash-command-test"),
		Model:   model.Model{Provider: "fake", ID: "echo"},
		Stream: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
			return streamText("ok"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := newBubbleModel(ctx, &App{Harness: h})
	for _, r := range "/status" {
		updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: string(r), Code: r}))
		m = updated.(bubbleModel)
	}
	if !m.commandPopup {
		t.Fatal("expected command popup before exact slash command submit")
	}
	updated, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	got := updated.(bubbleModel)
	if cmd == nil {
		t.Fatal("enter did not run exact slash command")
	}
	if !got.running || got.status != "command" {
		t.Fatalf("state after slash enter = running:%v status:%q", got.running, got.status)
	}
	if got.commandPopup {
		t.Fatal("command popup remained open after slash submit")
	}
}

func TestSlashCommandSuggestionsFilterAndInsert(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("tui-command-suggest-test"),
		Model:   model.Model{Provider: "fake", ID: "echo"},
		Stream: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
			return streamText("ok"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := newBubbleModel(ctx, &App{Harness: h})

	for _, r := range "/mo" {
		updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: string(r), Code: r}))
		m = updated.(bubbleModel)
	}
	if !m.commandPopup || len(m.commandSuggestions) != 2 {
		t.Fatalf("popup=%v suggestions=%v", m.commandPopup, m.commandSuggestions)
	}
	if got := m.commandSuggestions[m.selectedCommandSuggestion].Name; got != "/model" {
		t.Fatalf("selected suggestion = %q, want /model", got)
	}
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	m = updated.(bubbleModel)
	if got := m.input.Value(); got != "/model " {
		t.Fatalf("input after completion = %q, want /model ", got)
	}
	if m.commandPopup {
		t.Fatal("popup remained open after completion")
	}
}

func TestSlashCommandSuggestionsNavigate(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("tui-command-navigate-test"),
		Model:   model.Model{Provider: "fake", ID: "echo"},
		Stream: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
			return streamText("ok"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := newBubbleModel(ctx, &App{Harness: h})
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: "/", Code: '/'}))
	m = updated.(bubbleModel)
	if !m.commandPopup || len(m.commandSuggestions) == 0 {
		t.Fatalf("popup=%v suggestions=%v", m.commandPopup, m.commandSuggestions)
	}
	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m = updated.(bubbleModel)
	if m.selectedCommandSuggestion != 1 {
		t.Fatalf("selected suggestion = %d, want 1", m.selectedCommandSuggestion)
	}
	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = updated.(bubbleModel)
	if got, want := m.input.Value(), slashCommands()[1].Name+" "; got != want {
		t.Fatalf("input after enter = %q, want %q", got, want)
	}
}

func TestBubbleLayoutFitsWindowHeight(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("tui-layout-test"),
		Model:   model.Model{Provider: "fake", ID: "echo"},
		Stream: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
			return streamText("ok"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []tea.WindowSizeMsg{
		{Width: 80, Height: 12},
		{Width: 80, Height: 24},
		{Width: 120, Height: 40},
	} {
		updated, _ := newBubbleModel(ctx, &App{Harness: h}).Update(size)
		m := updated.(bubbleModel)
		view := m.View()
		if got, want := lipgloss.Height(view.Content), size.Height; got > want {
			t.Fatalf("%dx%d view height = %d, want <= %d", size.Width, size.Height, got, want)
		}
	}
}

func TestBubbleCommandPopupLayoutFitsWindowHeight(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("tui-command-popup-layout-test"),
		Model:   model.Model{Provider: "fake", ID: "echo"},
		Stream: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
			return streamText("ok"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := newBubbleModel(ctx, &App{Harness: h}).Update(tea.WindowSizeMsg{Width: 80, Height: 18})
	m := updated.(bubbleModel)
	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "/", Code: '/'}))
	m = updated.(bubbleModel)
	view := m.View()
	if got, want := lipgloss.Height(view.Content), 18; got > want {
		t.Fatalf("command popup view height = %d, want <= %d", got, want)
	}
}

func TestBubbleDialogLayoutFitsWindowHeight(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Config{
		Session: memory.New("tui-dialog-layout-test"),
		Model:   model.Model{Provider: "fake", ID: "echo"},
		Stream: func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
			return streamText("ok"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := newBubbleModel(ctx, &App{Harness: h}).Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m := updated.(bubbleModel)
	m.dialog = dialogHelp
	view := m.View()
	if got, want := lipgloss.Height(view.Content), 24; got > want {
		t.Fatalf("help dialog view height = %d, want <= %d", got, want)
	}
}

func streamText(text string) <-chan provider.Event {
	ch := make(chan provider.Event, 3)
	ch <- provider.MessageStart{MessageID: "m1"}
	ch <- provider.TextDelta{Text: text}
	ch <- provider.MessageEnd{StopReason: "end_turn"}
	close(ch)
	return ch
}

func lastUserText(req provider.Request) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role != message.RoleUser {
			continue
		}
		for _, c := range req.Messages[i].Content {
			if text, ok := c.(message.Text); ok {
				return text.Text
			}
		}
	}
	return ""
}
