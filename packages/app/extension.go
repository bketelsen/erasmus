package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"erasmus/packages/config"
	"erasmus/packages/event"
	"erasmus/packages/extension"
	"erasmus/packages/tool"
)

// ExtensionListProcess starts an extension process and prints registered tools/commands.
func ExtensionListProcess(ctx context.Context, out io.Writer, command string, args ...string) error {
	if strings.TrimSpace(command) == "" {
		return fmt.Errorf("extension command is required")
	}
	if out == nil {
		out = io.Discard
	}
	proc, err := extension.StartProcessWithOptions(ctx, command, extension.ProcessOptions{LogPath: defaultExtensionLogPath(command)}, args...)
	if err != nil {
		return err
	}
	defer proc.Close()
	for _, t := range proc.Manager().Registry().List() {
		fmt.Fprintf(out, "tool\t%s\t%s\n", t.Name(), t.Description())
	}
	for _, c := range proc.Manager().Commands() {
		fmt.Fprintf(out, "command\t%s\t%s\n", c.Name(), c.Description())
	}
	printExtensionDiagnostics(out, proc.Diagnostics())
	return nil
}

// ExtensionExecProcess starts an extension process and executes one registered command.
func ExtensionExecProcess(ctx context.Context, out io.Writer, processCommand string, processArgs []string, commandName string, input string) error {
	if strings.TrimSpace(processCommand) == "" {
		return fmt.Errorf("extension command is required")
	}
	if strings.TrimSpace(commandName) == "" {
		return fmt.Errorf("registered command name is required")
	}
	if out == nil {
		out = io.Discard
	}
	proc, err := extension.StartProcessWithOptions(ctx, processCommand, extension.ProcessOptions{LogPath: defaultExtensionLogPath(processCommand)}, processArgs...)
	if err != nil {
		return err
	}
	defer proc.Close()
	cmd, ok := proc.Manager().Command(commandName)
	if !ok {
		return withExtensionLogPath(fmt.Errorf("extension command %q is not registered", commandName), proc.LogPath())
	}
	res, err := cmd.Execute(ctx, commandInput(input))
	if err != nil {
		return withExtensionLogPath(err, proc.LogPath())
	}
	actions := append([]extension.HostAction(nil), res.Actions...)
	actions = append(actions, proc.Manager().DrainHostActions()...)
	if len(actions) == 0 {
		fmt.Fprintln(out, "ok")
		printExtensionDiagnostics(out, proc.Diagnostics())
		return nil
	}
	for _, action := range actions {
		data := string(action.Data)
		if data == "" {
			data = "{}"
		}
		fmt.Fprintf(out, "action\t%s\t%s\n", action.Type, data)
	}
	printExtensionDiagnostics(out, proc.Diagnostics())
	return nil
}

func printExtensionDiagnostics(out io.Writer, diagnostics []string) {
	for _, line := range diagnostics {
		fmt.Fprintf(out, "diagnostic\t%s\n", line)
	}
}

func withExtensionLogPath(err error, path string) error {
	if err == nil || path == "" {
		return err
	}
	if strings.Contains(err.Error(), "extension log: "+path) {
		return err
	}
	return fmt.Errorf("%w\nextension log: %s", err, path)
}

func commandInput(input string) json.RawMessage {
	input = strings.TrimSpace(input)
	if input == "" {
		return json.RawMessage(`{}`)
	}
	if json.Valid([]byte(input)) {
		return json.RawMessage(input)
	}
	data, _ := json.Marshal(map[string]string{"text": input})
	return data
}

// ConfiguredExtensions is a running set of configured extension subprocesses.
type ConfiguredExtensions struct {
	procs []*extension.Process
	tools tool.Registry
}

// Tools returns all registered subprocess tools.
func (e *ConfiguredExtensions) Tools() tool.Registry {
	if e == nil {
		return nil
	}
	return e.tools
}

// Close terminates all subprocesses.
func (e *ConfiguredExtensions) Close() {
	if e == nil {
		return
	}
	for i := len(e.procs) - 1; i >= 0; i-- {
		_ = e.procs[i].Close()
	}
}

// DrainHostActions drains queued host actions from all subprocesses.
func (e *ConfiguredExtensions) DrainHostActions() []extension.HostAction {
	if e == nil {
		return nil
	}
	var out []extension.HostAction
	for _, proc := range e.procs {
		out = append(out, proc.Manager().DrainHostActions()...)
	}
	return out
}

// Commands returns all registered subprocess commands.
func (e *ConfiguredExtensions) Commands() []extension.Command {
	if e == nil {
		return nil
	}
	var out []extension.Command
	for _, proc := range e.procs {
		out = append(out, proc.Manager().Commands()...)
	}
	return out
}

// Diagnostics returns recent diagnostics from all subprocesses.
func (e *ConfiguredExtensions) Diagnostics() []string {
	if e == nil {
		return nil
	}
	var out []string
	for _, proc := range e.procs {
		out = append(out, proc.Diagnostics()...)
		if path := proc.LogPath(); path != "" {
			out = append(out, "extension log: "+path)
		}
	}
	return out
}

// LogPaths returns persistent diagnostic log paths from all subprocesses.
func (e *ConfiguredExtensions) LogPaths() []string {
	if e == nil {
		return nil
	}
	var out []string
	for _, proc := range e.procs {
		if path := proc.LogPath(); path != "" {
			out = append(out, path)
		}
	}
	return out
}

// Command returns the first registered subprocess command with name.
func (e *ConfiguredExtensions) Command(name string) (extension.Command, bool) {
	if e == nil {
		return nil, false
	}
	for _, proc := range e.procs {
		if cmd, ok := proc.Manager().Command(name); ok {
			return cmd, true
		}
	}
	return nil, false
}

func (e *ConfiguredExtensions) FirstLogPath() string {
	if e == nil {
		return ""
	}
	for _, path := range e.LogPaths() {
		if path != "" {
			return path
		}
	}
	return ""
}

// PublishEvent forwards a runtime event to all configured extension subprocesses.
func (e *ConfiguredExtensions) PublishEvent(ctx context.Context, ev event.Event) error {
	if e == nil || ev == nil {
		return nil
	}
	for _, proc := range e.procs {
		if err := proc.PublishEvent(ctx, ev); err != nil {
			return err
		}
	}
	return nil
}

// StartConfiguredExtensionSet starts configured extension subprocesses.
func StartConfiguredExtensionSet(ctx context.Context, cfg config.Config) (*ConfiguredExtensions, error) {
	if len(cfg.Extensions) == 0 {
		return nil, nil
	}
	set := &ConfiguredExtensions{}
	var tools []tool.Tool
	for _, ext := range cfg.Extensions {
		if strings.TrimSpace(ext.Command) == "" {
			set.Close()
			return nil, fmt.Errorf("extension command is required")
		}
		proc, err := extension.StartProcessWithOptions(ctx, ext.Command, extension.ProcessOptions{LogPath: defaultExtensionLogPath(ext.Command)}, ext.Args...)
		if err != nil {
			set.Close()
			return nil, err
		}
		set.procs = append(set.procs, proc)
		tools = append(tools, proc.Manager().Registry().List()...)
	}
	set.tools = tool.NewRegistry(tools...)
	return set, nil
}

func defaultExtensionLogPath(command string) string {
	return filepath.Join(xdgStateHome(), "erasmus", "extensions", "logs", extensionLogName(command, time.Now()))
}

var extensionLogNameUnsafe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func extensionLogName(command string, t time.Time) string {
	name := filepath.Base(strings.TrimSpace(command))
	name = extensionLogNameUnsafe.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-.")
	if name == "" {
		name = "extension"
	}
	return t.UTC().Format("20060102T150405.000000000Z") + "-" + name + ".jsonl"
}

// StartConfiguredExtensions starts configured extension subprocesses and returns their tools.
func StartConfiguredExtensions(ctx context.Context, cfg config.Config) (tool.Registry, func(), error) {
	set, err := StartConfiguredExtensionSet(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	if set == nil {
		return nil, func() {}, nil
	}
	return set.Tools(), set.Close, nil
}
