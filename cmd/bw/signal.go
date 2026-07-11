package main

import (
	"fmt"
	"strings"

	"github.com/jallum/beadwork/internal/config"
	"github.com/jallum/beadwork/internal/issue"
	"github.com/jallum/beadwork/internal/repo"
	signalcfg "github.com/jallum/beadwork/internal/signal"
)

// SignalArgs holds parsed `bw signal` arguments.
type SignalArgs struct {
	Subcommand string
	TicketID   string
	Type       string
	Fields     map[string]string
}

func parseSignalArgs(raw []string) (SignalArgs, error) {
	if len(raw) == 0 {
		return SignalArgs{}, fmt.Errorf("usage: bw signal emit <ticket-id> <type> [--field key=value...] | bw signal types")
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
		if len(raw) != 1 {
			return SignalArgs{}, fmt.Errorf("usage: bw signal types")
		}
		return SignalArgs{Subcommand: "types"}, nil
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
		return cmdSignalTypes(store, w)
	default:
		return nil, fmt.Errorf("unknown signal subcommand: %s", a.Subcommand)
	}
}

// cmdSignalEmit validates against working-tree config and stores the signal.
// Hooks are intentionally absent in this stage.
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
	payload, err = signalcfg.ValidatePayload(*typ, payload)
	if err != nil {
		return nil, err
	}

	var path string
	err = commitWithRetry(store, commitMaxRetries, func() (string, error) {
		var serr error
		_, path, serr = store.EmitSignal(a.TicketID, a.Type, payload)
		if serr != nil {
			return "", serr
		}
		return fmt.Sprintf("signal %s %s %s", a.TicketID, a.Type, path), nil
	})
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(w, "emitted %s for %s at %s\n", w.Style(a.Type, Cyan), w.Style(a.TicketID, Cyan), w.Style(path, Cyan))
	return nil, nil
}

func cmdSignalTypes(store *issue.Store, w Writer) (*config.Config, error) {
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
	for _, name := range names {
		fmt.Fprintln(w, name)
	}
	return nil, nil
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
