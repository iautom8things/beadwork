package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jallum/beadwork/internal/config"
	"github.com/jallum/beadwork/internal/issue"
	"github.com/jallum/beadwork/internal/repo"
	signalcfg "github.com/jallum/beadwork/internal/signal"
)

func init() {
	for i := range commands {
		if commands[i].Name == "signal" {
			commands[i].Description += " Emission always runs the configured enrich, validation, gate, and post-emit pipeline; there is no bypass flag."
			break
		}
	}
}

// SignalArgs holds parsed `bw signal` arguments.
type SignalArgs struct {
	Subcommand string
	TicketID   string
	Type       string
	Fields     map[string]string
	Verbose    bool
	JSON       bool
	RunHooks   bool
}

func parseSignalArgs(raw []string) (SignalArgs, error) {
	if len(raw) == 0 {
		return SignalArgs{}, fmt.Errorf("usage: bw signal emit <ticket-id> <type> [--field key=value...] | bw signal types [--verbose] [--json] | bw signal show <type> [--json] | bw signal validate <type> [--field key=value...] [--run-hooks]")
	}
	switch raw[0] {
	case "emit":
		fields := map[string]string{}
		pos := []string{}
		for i := 1; i < len(raw); i++ {
			if raw[i] == "--field" {
				if i+1 >= len(raw) {
					return SignalArgs{}, fmt.Errorf("--field requires key=value")
				}
				k, v, ok := strings.Cut(raw[i+1], "=")
				if !ok || k == "" {
					return SignalArgs{}, fmt.Errorf("--field requires key=value")
				}
				fields[k] = v
				i++
				continue
			}
			if strings.HasPrefix(raw[i], "--") {
				return SignalArgs{}, fmt.Errorf("unknown flag: %s", raw[i])
			}
			pos = append(pos, raw[i])
		}
		if len(pos) != 2 {
			return SignalArgs{}, fmt.Errorf("usage: bw signal emit <ticket-id> <type> [--field key=value...]")
		}
		return SignalArgs{Subcommand: "emit", TicketID: pos[0], Type: pos[1], Fields: fields}, nil
	case "types":
		a := SignalArgs{Subcommand: "types"}
		for _, arg := range raw[1:] {
			switch arg {
			case "--verbose":
				a.Verbose = true
			case "--json":
				a.JSON = true
			default:
				return SignalArgs{}, fmt.Errorf("unknown flag: %s", arg)
			}
		}
		return a, nil
	case "show":
		a := SignalArgs{Subcommand: "show"}
		pos := []string{}
		for _, arg := range raw[1:] {
			switch arg {
			case "--json":
				a.JSON = true
			default:
				if strings.HasPrefix(arg, "--") {
					return SignalArgs{}, fmt.Errorf("unknown flag: %s", arg)
				}
				pos = append(pos, arg)
			}
		}
		if len(pos) != 1 {
			return SignalArgs{}, fmt.Errorf("usage: bw signal show <type> [--json]")
		}
		a.Type = pos[0]
		return a, nil
	case "validate":
		a := SignalArgs{Subcommand: "validate", Fields: map[string]string{}}
		pos := []string{}
		for i := 1; i < len(raw); i++ {
			switch raw[i] {
			case "--field":
				if i+1 >= len(raw) {
					return SignalArgs{}, fmt.Errorf("--field requires key=value")
				}
				k, v, ok := strings.Cut(raw[i+1], "=")
				if !ok || k == "" {
					return SignalArgs{}, fmt.Errorf("--field requires key=value")
				}
				a.Fields[k] = v
				i++
			case "--run-hooks":
				a.RunHooks = true
			default:
				if strings.HasPrefix(raw[i], "--") {
					return SignalArgs{}, fmt.Errorf("unknown flag: %s", raw[i])
				}
				pos = append(pos, raw[i])
			}
		}
		if len(pos) != 1 {
			return SignalArgs{}, fmt.Errorf("usage: bw signal validate <type> [--field key=value...] [--run-hooks]")
		}
		a.Type = pos[0]
		return a, nil
	default:
		return SignalArgs{}, fmt.Errorf("unknown signal subcommand: %s", raw[0])
	}
}

func cmdSignal(store *issue.Store, args []string, w Writer, _ *config.Config) (*config.Config, error) {
	a, err := parseSignalArgs(args)
	if err != nil {
		return nil, err
	}
	switch a.Subcommand {
	case "emit":
		return cmdSignalEmit(store, a, w)
	case "types":
		return cmdSignalTypes(store, a, w)
	case "show":
		return cmdSignalShow(store, a, w)
	case "validate":
		return cmdSignalValidate(store, a, w)
	default:
		return nil, fmt.Errorf("unknown signal subcommand: %s", a.Subcommand)
	}
}

// cmdSignalEmit runs the unskippable hook pipeline and stores the signal.
func cmdSignalEmit(store *issue.Store, a SignalArgs, w Writer) (*config.Config, error) {
	cfg, err := loadSignalConfig(store)
	if err != nil {
		return nil, err
	}
	if len(cfg.Types) == 0 {
		return nil, signalcfg.ValidationError{Err: fmt.Errorf("no signal types defined")}
	}
	typ, ok := cfg.TypeByName(a.Type)
	if !ok {
		return nil, signalcfg.ValidationError{Err: fmt.Errorf("undefined signal type %s", a.Type)}
	}
	payload := make(map[string]any, len(a.Fields))
	for k, v := range a.Fields {
		payload[k] = v
	}
	var path string
	r := store.Committer.(*repo.Repo)
	pipeline := signalcfg.Pipeline{RepoRoot: r.RepoDir(), CallerCWD: r.CWD, Config: cfg, Type: typ, Ticket: a.TicketID}
	var warnings []error
	_, warnings, err = pipeline.Run(payload,
		func(p map[string]any) (map[string]any, error) { return signalcfg.ValidatePayload(*typ, p) },
		func(p map[string]any) error {
			return commitWithRetry(store, commitMaxRetries, func() (string, error) {
				var serr error
				_, path, serr = store.EmitSignal(a.TicketID, a.Type, p)
				if serr != nil {
					return "", serr
				}
				return fmt.Sprintf("signal %s %s %s", a.TicketID, a.Type, path), nil
			})
		})
	for _, warning := range warnings {
		fmt.Fprintf(w, "WARNING: %v\n", warning)
	}
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(w, "emitted %s for %s at %s\n", w.Style(a.Type, Cyan), w.Style(a.TicketID, Cyan), w.Style(path, Cyan))
	return nil, nil
}

func cmdSignalTypes(store *issue.Store, a SignalArgs, w Writer) (*config.Config, error) {
	cfg, err := loadSignalConfig(store)
	if err != nil {
		if err == signalcfg.ErrNoConfig {
			fmt.Fprintln(w, "no signal types defined")
			return nil, nil
		}
		return nil, err
	}
	names := cfg.TypeNames()
	if len(names) == 0 {
		fmt.Fprintln(w, "no signal types defined")
		return nil, nil
	}
	if a.JSON {
		fprintJSON(w, signalIntrospection(cfg, ""))
		return nil, nil
	}
	if a.Verbose {
		for _, detail := range signalIntrospection(cfg, "").Types {
			printSignalTypeDetail(w, detail)
		}
		return nil, nil
	}
	for _, name := range names {
		fmt.Fprintln(w, name)
	}
	return nil, nil
}

func cmdSignalShow(store *issue.Store, a SignalArgs, w Writer) (*config.Config, error) {
	cfg, err := loadSignalConfig(store)
	if err != nil {
		return nil, err
	}
	typ, ok := cfg.TypeByName(a.Type)
	if !ok {
		return nil, signalcfg.ValidationError{Err: fmt.Errorf("undefined signal type %s", a.Type)}
	}
	detail := buildSignalTypeDetail(cfg, typ)
	if a.JSON {
		fprintJSON(w, detail)
	} else {
		printSignalTypeDetail(w, detail)
	}
	return nil, nil
}

func cmdSignalValidate(store *issue.Store, a SignalArgs, w Writer) (*config.Config, error) {
	cfg, err := loadSignalConfig(store)
	if err != nil {
		return nil, err
	}
	if len(cfg.Types) == 0 {
		return nil, signalcfg.ValidationError{Err: fmt.Errorf("no signal types defined")}
	}
	typ, ok := cfg.TypeByName(a.Type)
	if !ok {
		return nil, signalcfg.ValidationError{Err: fmt.Errorf("undefined signal type %s", a.Type)}
	}
	payload := make(map[string]any, len(a.Fields))
	for k, v := range a.Fields {
		payload[k] = v
	}
	if !a.RunHooks {
		validated, err := signalcfg.ValidatePayload(*typ, payload)
		printSignalValidationReport(w, *typ, validated, err)
		return nil, err
	}
	r := store.Committer.(*repo.Repo)
	result, err := (signalcfg.Pipeline{RepoRoot: r.RepoDir(), CallerCWD: r.CWD, Config: cfg, Type: typ}).DryRun(payload,
		func(p map[string]any) (map[string]any, error) { return signalcfg.ValidatePayload(*typ, p) })
	printSignalDryRunReport(w, result)
	return nil, err
}

type signalIntrospectionResult struct {
	Types []signalTypeDetail `json:"types"`
}

type signalTypeDetail struct {
	Name        string            `json:"name"`
	Fields      []signalFieldInfo `json:"fields"`
	Hooks       signalHookInfo    `json:"hooks"`
	HookTimeout string            `json:"hook_timeout"`
}

type signalFieldInfo struct {
	Name         string              `json:"name"`
	Type         string              `json:"type"`
	Values       []string            `json:"values,omitempty"`
	Required     bool                `json:"required"`
	RequiredWhen *signalRequiredWhen `json:"required_when,omitempty"`
}

type signalRequiredWhen struct {
	Field  string `json:"field"`
	Equals string `json:"equals"`
}

type signalHookInfo struct {
	Enrich    []signalHookCommand `json:"enrich"`
	Gate      []signalHookCommand `json:"gate"`
	OnBlocked []signalHookCommand `json:"on_blocked"`
	PostEmit  []signalHookCommand `json:"post_emit"`
}

type signalHookCommand struct {
	Source  string `json:"source"`
	Command string `json:"command"`
}

func signalIntrospection(cfg *signalcfg.Config, only string) signalIntrospectionResult {
	out := signalIntrospectionResult{}
	for _, name := range cfg.TypeNames() {
		if only != "" && name != only {
			continue
		}
		typ, _ := cfg.TypeByName(name)
		out.Types = append(out.Types, buildSignalTypeDetail(cfg, typ))
	}
	return out
}

func buildSignalTypeDetail(cfg *signalcfg.Config, typ *signalcfg.Type) signalTypeDetail {
	d := signalTypeDetail{Name: typ.Name, HookTimeout: cfg.HookTimeout.String()}
	for _, f := range typ.Fields {
		var requiredWhen *signalRequiredWhen
		if f.RequiredWhen != nil {
			requiredWhen = &signalRequiredWhen{Field: f.RequiredWhen.Field, Equals: f.RequiredWhen.Equals}
		}
		d.Fields = append(d.Fields, signalFieldInfo{
			Name:         f.Name,
			Type:         f.Kind,
			Values:       append([]string{}, f.Values...),
			Required:     f.Required,
			RequiredWhen: requiredWhen,
		})
	}
	d.Hooks.Enrich = append(hookCommands("global", cfg.Hooks.Enrich), hookCommands("type", typ.Hooks.Enrich)...)
	d.Hooks.Gate = append(hookCommands("type", typ.Hooks.Gate), hookCommands("global", cfg.Hooks.Gate)...)
	d.Hooks.OnBlocked = append(hookCommands("type", typ.Hooks.OnBlocked), hookCommands("global", cfg.Hooks.OnBlocked)...)
	d.Hooks.PostEmit = append(hookCommands("type", typ.Hooks.PostEmit), hookCommands("global", cfg.Hooks.PostEmit)...)
	return d
}

func hookCommands(source string, commands []string) []signalHookCommand {
	out := make([]signalHookCommand, 0, len(commands))
	for _, command := range commands {
		out = append(out, signalHookCommand{Source: source, Command: command})
	}
	return out
}

func printSignalTypeDetail(w Writer, d signalTypeDetail) {
	fmt.Fprintf(w, "%s\n", d.Name)
	if len(d.Fields) == 0 {
		fmt.Fprintln(w, "  fields: none")
	} else {
		fmt.Fprintln(w, "  fields:")
		for _, f := range d.Fields {
			line := fmt.Sprintf("    %s: %s", f.Name, f.Type)
			if len(f.Values) > 0 {
				line += " [" + strings.Join(f.Values, ", ") + "]"
			}
			if f.Required {
				line += " required"
			}
			if f.RequiredWhen != nil {
				line += fmt.Sprintf(" required_when %s=%s", f.RequiredWhen.Field, f.RequiredWhen.Equals)
			}
			fmt.Fprintln(w, line)
		}
	}
	fmt.Fprintf(w, "  hook_timeout: %s\n", d.HookTimeout)
	printHookMoment(w, "enrich", d.Hooks.Enrich)
	printHookMoment(w, "gate", d.Hooks.Gate)
	printHookMoment(w, "on-blocked", d.Hooks.OnBlocked)
	printHookMoment(w, "post-emit", d.Hooks.PostEmit)
}

func printHookMoment(w Writer, name string, hooks []signalHookCommand) {
	if len(hooks) == 0 {
		fmt.Fprintf(w, "  %s hooks: none\n", name)
		return
	}
	fmt.Fprintf(w, "  %s hooks:\n", name)
	for _, h := range hooks {
		fmt.Fprintf(w, "    %s: %s\n", h.Source, h.Command)
	}
}

func printSignalValidationReport(w Writer, typ signalcfg.Type, validated map[string]any, err error) {
	fmt.Fprintf(w, "type: %s\n", typ.Name)
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	for _, f := range typ.Fields {
		if errText != "" && strings.Contains(errText, f.Name) {
			fmt.Fprintf(w, "field %s: FAIL (%v)\n", f.Name, err)
			continue
		}
		if val, ok := validated[f.Name]; ok {
			fmt.Fprintf(w, "field %s: PASS (%v)\n", f.Name, val)
		} else {
			fmt.Fprintf(w, "field %s: PASS (absent)\n", f.Name)
		}
	}
	if err != nil {
		fmt.Fprintf(w, "schema: FAIL (%v)\n", err)
		return
	}
	fmt.Fprintln(w, "schema: PASS")
}

func printSignalDryRunReport(w Writer, result signalcfg.DryRunResult) {
	fmt.Fprintf(w, "enriched_payload: %s\n", jsonOneLine(result.Payload))
	if result.ValidationErr != nil {
		fmt.Fprintf(w, "schema: FAIL (%v)\n", result.ValidationErr)
		return
	}
	fmt.Fprintln(w, "schema: PASS")
	switch {
	case result.GateErr == nil:
		fmt.Fprintln(w, "gate: ALLOWED")
	default:
		var hookErr signalcfg.HookError
		if errors.As(result.GateErr, &hookErr) {
			fmt.Fprintf(w, "gate: %s (%s)\n", hookErr.Kind, hookErr.Reason)
		} else {
			fmt.Fprintf(w, "gate: MALFUNCTION (%v)\n", result.GateErr)
		}
	}
}

func jsonOneLine(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func loadSignalConfig(store *issue.Store) (*signalcfg.Config, error) {
	r, ok := store.Committer.(*repo.Repo)
	if !ok {
		return nil, fmt.Errorf("signal config requires a repo-backed store")
	}
	cfg, err := signalcfg.Load(r.RepoDir())
	if err != nil {
		if err == signalcfg.ErrNoConfig {
			return &signalcfg.Config{}, nil
		}
		return nil, err
	}
	return cfg, nil
}
