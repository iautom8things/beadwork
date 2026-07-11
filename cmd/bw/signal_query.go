package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/jallum/beadwork/internal/config"
	"github.com/jallum/beadwork/internal/issue"
	"github.com/jallum/beadwork/internal/repo"
)

type SignalQueryResult struct {
	Signals []issue.SignalRecord `json:"signals"`
	Cursor  string               `json:"cursor"`
}

type signalQueryArgs struct {
	TicketID string
	Type     string
	Since    string
	JSON     bool
}

func init() {
	for i := range commands {
		if commands[i].Name != "signal" {
			continue
		}
		commands[i].Description += " Query supports stateless since-cursor polling."
		commands[i].Positionals[0].Name = "emit|types|show|validate|query"
		commands[i].Flags = append(commands[i].Flags,
			Flag{Long: "--ticket", Value: "ID", Help: "Query signals for a ticket"},
			Flag{Long: "--type", Value: "TYPE", Help: "Filter queried signals by type"},
			Flag{Long: "--since", Value: "COMMIT", Help: "Poll after a beadwork commit cursor"},
			Flag{Long: "--json", Help: "Output query result as JSON"},
		)
		commands[i].Examples = append(commands[i].Examples, Example{Cmd: "bw signal query --ticket bw-a3f8 --json"})
		commands[i].Run = cmdSignalWithQuery
		return
	}
}

func cmdSignalWithQuery(store *issue.Store, args []string, w Writer, cfg *config.Config) (*config.Config, error) {
	if len(args) == 0 || args[0] != "query" {
		return cmdSignal(store, args, w, cfg)
	}
	a, err := parseSignalQueryArgs(args[1:])
	if err != nil {
		return nil, err
	}
	return cmdSignalQuery(store, a, w)
}

func parseSignalQueryArgs(raw []string) (signalQueryArgs, error) {
	a := signalQueryArgs{}
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case "--ticket", "--type", "--since":
			if i+1 >= len(raw) {
				return a, fmt.Errorf("%s requires a value", raw[i])
			}
			i++
			switch raw[i-1] {
			case "--ticket":
				a.TicketID = raw[i]
			case "--type":
				a.Type = raw[i]
			case "--since":
				a.Since = raw[i]
			}
		case "--json":
			a.JSON = true
		default:
			return a, fmt.Errorf("unknown flag: %s", raw[i])
		}
	}
	if a.TicketID == "" {
		return a, fmt.Errorf("usage: bw signal query --ticket <id> [--type T] [--since HASH] [--json]")
	}
	return a, nil
}

// cmdSignalQuery polls self-describing signal commits and never persists its cursor.
func cmdSignalQuery(store *issue.Store, a signalQueryArgs, w Writer) (*config.Config, error) {
	r, ok := store.Committer.(*repo.Repo)
	if !ok {
		return nil, fmt.Errorf("signal query requires a repo-backed store")
	}
	commits, err := r.TreeFS().CommitsSince(a.Since)
	if err != nil {
		return nil, fmt.Errorf("query signals: %w", err)
	}
	result := SignalQueryResult{Signals: []issue.SignalRecord{}, Cursor: r.TreeFS().RefHash().String()}
	for i := len(commits) - 1; i >= 0; i-- {
		for _, line := range strings.Split(commits[i].Message, "\n") {
			parts := strings.Fields(line)
			if len(parts) != 4 || parts[0] != "signal" || parts[1] != a.TicketID || (a.Type != "" && parts[2] != a.Type) {
				continue
			}
			data, err := r.TreeFS().ReadFileAt(plumbing.NewHash(commits[i].Hash), parts[3])
			if err != nil {
				return nil, fmt.Errorf("read signal %s: %w", parts[3], err)
			}
			var rec issue.SignalRecord
			if err := json.Unmarshal(data, &rec); err != nil {
				return nil, fmt.Errorf("decode signal %s: %w", parts[3], err)
			}
			result.Signals = append(result.Signals, rec)
		}
	}
	if a.JSON {
		fprintJSON(w, result)
	} else {
		for _, rec := range result.Signals {
			fmt.Fprintf(w, "%s %s %v\n", rec.Ticket, rec.Type, rec.Payload)
		}
		fmt.Fprintf(w, "cursor: %s\n", result.Cursor)
	}
	return nil, nil
}
