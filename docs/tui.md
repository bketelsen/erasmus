# Erasmus TUI

The Erasmus TUI has two modes:

- full-screen mode for interactive terminals,
- line-oriented fallback for pipes, scripts, and tests.

Full-screen mode starts only when both stdin and stdout are terminals. If either side is redirected, `erasmus tui` uses the line-oriented prompt so it can still be scripted.

## Launch

```bash
erasmus tui
erasmus tui --memory
erasmus tui --session .erasmus/sessions/work.jsonl
erasmus tui --theme dracula
erasmus --provider openai-codex --model gpt-5.5 tui
```

Root config flags such as `--config`, `--auth-file`, `--provider`, `--model`, `--reasoning`, `--cwd`, `--tool`, and `--no-tools` apply to TUI runs.

TUI-specific flags:

```text
--session <path>  durable JSONL session path
--memory          use an in-memory session
--theme <name>    theme override for this run
```

## Sessions

By default, the TUI stores a durable JSONL session under the XDG state directory:

```text
$XDG_STATE_HOME/erasmus/sessions/<workspace-key>/default.jsonl
```

If `XDG_STATE_HOME` is not set, Erasmus falls back to the platform user state location. Set `ERASMUS_SESSION_DIR` to override the default TUI session directory.

Use `--session <path>` to open a specific durable session. Use `--memory` for throwaway sessions that should not be persisted.

## Global Keys

```text
ctrl+s      submit prompt or send dialog input
ctrl+f      search transcript
ctrl+o      sessions
ctrl+p      model and reasoning picker
ctrl+t      session tree
ctrl+w      swarm dashboard
?           context help
ctrl+c      quit
```

Scrollback:

```text
PageUp/PageDown  page scroll
ctrl+u/ctrl+d    half-page scroll
Home             jump to top
End              jump to bottom and resume follow mode
mouse wheel      scrollback
```

On macOS terminals, use the terminal's Home/End/Page key equivalents, commonly:

```text
Fn+Left   Home
Fn+Right  End
Fn+Up     PageUp
Fn+Down   PageDown
```

## Slash Commands

Type `/` in the prompt to open command suggestions. The popup filters as you type. Use `up`/`down` to select, `tab` or `enter` to insert the selected command, and `esc` to close the popup.

Commands:

```text
/help                  show command help
/status                show runtime status
/state                 show runtime status
/model                 show model and reasoning
/messages [count]      show recent transcript
/transcript [count]    show recent transcript
/sessions [dir]        list durable JSONL sessions
/open <path>           switch to a durable JSONL session
/tree                  show session tree
/move <id> [summary]   move to tree entry
/branch <id>           create branch session at entry
/compact               compact transcript
/quit                  exit
/exit                  exit
```

Slash commands run from the prompt with `ctrl+s`, just like normal prompts.

## Transcript Search

Press `ctrl+f` to enter search mode.

```text
enter  run search
esc    close search prompt
n      next match
N      previous match
```

The status line shows the active match index when matches exist. Scrolling away from the bottom pauses auto-follow; press `End` to resume.

## Sessions Dialog

Open with `ctrl+o`.

```text
up/down or k/j  select session
enter           open selected session
esc or ctrl+o   close
```

The session list shows JSONL sessions from the default TUI session directory unless a directory is supplied through `/sessions [dir]`.

## Model And Reasoning Picker

Open with `ctrl+p`.

```text
tab/shift+tab      switch provider
up/down or k/j     select model
left/right or h/l  select reasoning level
enter              apply model and reasoning
esc or ctrl+p      close
```

Provider switching requires provider credentials when the selected provider needs auth. `ctrl+m` is not used because terminals report it as Enter.

## Session Tree

Open with `ctrl+t`.

```text
up/down or k/j  select tree entry
enter           move session leaf to selected entry
esc or ctrl+t   close
```

The current leaf is marked in the tree. Moving the leaf reloads the in-memory transcript from the selected entry.

## Swarm Dashboard

Open with `ctrl+w`.

```text
up/down or k/j  select server
tab/shift+tab   select agent
a               attach or detach selected agent
n               spawn new agent on selected server
s               send prompt to selected or attached agent
x               stop selected agent
l               load selected agent log tail
enter           refresh selected server or attached agent
r               reload registry
esc             detach if attached, otherwise close
ctrl+w          close dashboard
```

When composing a swarm send/spawn prompt, type in the input box and press `ctrl+s` or `enter` to send. Press `esc` to cancel.

Attach mode follows a selected agent and refreshes status/log tail every two seconds:

```text
s              send prompt to attached agent
enter          refresh status/log now
tab/shift+tab  switch attached agent
a or esc       detach
```

## Themes

Set a persistent theme:

```bash
erasmus config set theme plain
erasmus config set theme dark
erasmus config set theme light
erasmus config set theme high-contrast
erasmus config set theme dracula
```

Override for one run:

```bash
erasmus tui --theme plain
erasmus tui --theme dark
erasmus tui --theme light
erasmus tui --theme high-contrast
erasmus tui --theme dracula
```

Built-in themes:

```text
dark
plain
light
high-contrast
dracula
```

Aliases for `plain`:

```text
mono
monochrome
```

## Troubleshooting

If the full-screen TUI does not start, check whether stdin or stdout is redirected. Full-screen mode requires both to be terminals.

If keys do not behave as expected, use the terminal's alternate bindings for Home, End, PageUp, and PageDown. Some terminals also reserve control-key combinations before applications receive them.

If the input box appears clipped, first check terminal height. The full-screen layout keeps the transcript, dialogs, command popup, and input inside the terminal height, but very small windows leave little transcript space.

If provider/model changes fail, verify credentials:

```bash
erasmus auth status
erasmus login openai
erasmus login openai-codex
erasmus login github-copilot
```

If sessions are not where expected, inspect the active session directory:

```bash
echo "$XDG_STATE_HOME"
echo "$ERASMUS_SESSION_DIR"
erasmus sessions list
```
