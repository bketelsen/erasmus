# GitHub Housekeeping Agent Example

This example is a timer-friendly autonomous agent for one GitHub repository. Each run:

1. clones the target repository into a temporary directory,
2. runs one conservative scan for housekeeping and cleanup opportunities,
3. creates up to three GitHub issues,
4. implements each issue on its own branch, and
5. opens a pull request that closes the matching issue.

The scan prompt intentionally excludes new features, rewrites, style-only changes, and documentation-only work.

## Requirements

- `gh` authenticated with permission to clone, create issues, push branches, and open PRs,
- `git`,
- Erasmus auth/config for a real provider, for example:

```sh
erasmus login openai-codex
erasmus config set provider openai-codex
erasmus config set model gpt-5.5
```

The example defaults to the same XDG config and auth paths as the Erasmus CLI. You can override them with `--config`, `--auth-file`, `ERASMUS_CONFIG_FILE`, or `ERASMUS_AUTH_FILE`.

## Try It

Build the agent:

```sh
go build -o ~/.local/bin/github-housekeeping-agent .
```

Run a dry scan:

```sh
~/.local/bin/github-housekeeping-agent --repo owner/name --dry-run
```

Run the full workflow:

```sh
~/.local/bin/github-housekeeping-agent --repo owner/name
```

Useful flags:

- `--max 3`: cap opportunities; values above 3 are clamped to 3,
- `--max-steps 80`: tool/model turns per scan or implementation run,
- `--state ~/.local/state/erasmus/github-housekeeping-agent`: durable sessions,
- `--keep-work`: keep the temporary clone for inspection,
- `--provider` and `--model`: override Erasmus config for this job.

## systemd User Timer

Install the sample units:

```sh
mkdir -p ~/.config/systemd/user ~/.config/erasmus
cp systemd/github-housekeeping-agent.service ~/.config/systemd/user/
cp systemd/github-housekeeping-agent.timer ~/.config/systemd/user/
```

Create `~/.config/erasmus/github-housekeeping-agent.env`:

```sh
HOUSEKEEPING_REPO=owner/name
```

Enable the timer:

```sh
systemctl --user daemon-reload
systemctl --user enable --now github-housekeeping-agent.timer
```

Run once immediately:

```sh
systemctl --user start github-housekeeping-agent.service
```

Inspect logs:

```sh
journalctl --user -u github-housekeeping-agent.service
```
