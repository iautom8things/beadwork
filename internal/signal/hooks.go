package signal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// HookError is a deliberate policy refusal or a fail-closed hook malfunction.
type HookError struct{ Kind, Hook, Reason string }

func (e HookError) Error() string { return fmt.Sprintf("%s: hook %s: %s", e.Kind, e.Hook, e.Reason) }

// Pipeline executes configured signal hooks once around validation and storage.
type Pipeline struct {
	RepoRoot string
	Config   *Config
	Type     *Type
	Ticket   string
}

type hookRecord struct {
	Type    string         `json:"type"`
	Ticket  string         `json:"ticket"`
	Payload map[string]any `json:"payload"`
}

// Run executes enrich, validation, gate, store, and post-emit in order. Post-emit
// failures are returned as warnings while a successful store remains successful.
func (p Pipeline) Run(payload map[string]any, validate func(map[string]any) (map[string]any, error), store func(map[string]any) error) (map[string]any, []error, error) {
	if p.Config == nil || p.Type == nil {
		return nil, nil, fmt.Errorf("pipeline requires config and type")
	}
	var err error
	for _, h := range appendCopy(p.Config.Hooks.Enrich, p.Type.Hooks.Enrich...) {
		payload, err = p.enrich(h, payload)
		if err != nil {
			return nil, nil, err
		}
	}
	payload, err = validate(payload)
	if err != nil {
		return nil, nil, err
	}
	for _, h := range appendCopy(p.Type.Hooks.Gate, p.Config.Hooks.Gate...) {
		res, runErr := p.run(h, "gate", payload)
		if runErr == nil {
			continue
		}
		var ee *exec.ExitError
		if errors.As(runErr, &ee) && ee.ExitCode() == 1 {
			reason := strings.TrimSpace(res.stdout.String() + res.stderr.String())
			if reason == "" {
				reason = "refused"
			}
			blocked := HookError{Kind: "BLOCKED", Hook: h, Reason: reason}
			warnings := p.runIsolated(appendCopy(p.Type.Hooks.OnBlocked, p.Config.Hooks.OnBlocked...), "on-blocked", payload)
			return nil, warnings, blocked
		}
		return nil, nil, malfunction(h, runErr)
	}
	if err = store(payload); err != nil {
		return nil, nil, err
	}
	warnings := p.runIsolated(appendCopy(p.Type.Hooks.PostEmit, p.Config.Hooks.PostEmit...), "post-emit", payload)
	return payload, warnings, nil
}

type hookResult struct{ stdout, stderr bytes.Buffer }

func (p Pipeline) run(h, moment string, payload map[string]any) (hookResult, error) {
	var result hookResult
	b, err := json.Marshal(hookRecord{Type: p.Type.Name, Ticket: p.Ticket, Payload: payload})
	if err != nil {
		return result, err
	}
	timeout := p.Config.HookTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	cmd := exec.Command(h)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Dir = p.RepoRoot
	cmd.Env = append(os.Environ(), "BW_SIGNAL_TYPE="+p.Type.Name, "BW_SIGNAL_TICKET="+p.Ticket, "BW_SIGNAL_MOMENT="+moment)
	cmd.Stdin = bytes.NewReader(append(b, '\n'))
	cmd.Stdout, cmd.Stderr = &result.stdout, &result.stderr
	if err := cmd.Start(); err != nil {
		return result, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return result, err
	case <-time.After(timeout):
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		}
		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
			if cmd.Process != nil {
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
			<-done
		}
		return result, fmt.Errorf("timed out after %s", timeout)
	}
}

func (p Pipeline) enrich(h string, payload map[string]any) (map[string]any, error) {
	res, err := p.run(h, "enrich", payload)
	if err != nil {
		return nil, malfunction(h, err)
	}
	if len(bytes.TrimSpace(res.stdout.Bytes())) == 0 {
		return payload, nil
	}
	var out map[string]any
	if err := json.Unmarshal(res.stdout.Bytes(), &out); err != nil || out == nil {
		return nil, HookError{Kind: "MALFUNCTION", Hook: h, Reason: "enrich stdout is not a JSON object"}
	}
	return out, nil
}
func (p Pipeline) runIsolated(hooks []string, moment string, payload map[string]any) []error {
	var out []error
	for _, h := range hooks {
		if _, err := p.run(h, moment, payload); err != nil {
			out = append(out, fmt.Errorf("hook %s: %w", h, err))
		}
	}
	return out
}
func appendCopy(a []string, b ...string) []string {
	out := append([]string{}, a...)
	return append(out, b...)
}
func malfunction(h string, err error) error {
	return HookError{Kind: "MALFUNCTION", Hook: h, Reason: err.Error()}
}
