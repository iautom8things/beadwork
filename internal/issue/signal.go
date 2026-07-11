package issue

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
)

// ErrSignalNotFound is returned when a requested signal record is absent.
var ErrSignalNotFound = fmt.Errorf("signal not found")

const signalsRoot = "signals"

// SignalRecord is the immutable JSON snapshot stored for every emitted signal.
type SignalRecord struct {
	Seq       int            `json:"seq"`
	Type      string         `json:"type"`
	Ticket    string         `json:"ticket"`
	Payload   map[string]any `json:"payload"`
	EmittedAt string         `json:"emitted_at"`
}

// EmitSignal writes one immutable signal snapshot and returns the record path.
func (s *Store) EmitSignal(ticketID, signalType string, payload map[string]any) (*SignalRecord, string, error) {
	if err := validateSignalTicket(ticketID); err != nil {
		return nil, "", err
	}
	if signalType == "" {
		return nil, "", fmt.Errorf("signal type is empty")
	}
	seq, err := s.nextSignalSeq(ticketID)
	if err != nil {
		return nil, "", err
	}
	rec := &SignalRecord{
		Seq:       seq,
		Type:      signalType,
		Ticket:    ticketID,
		Payload:   copyPayload(payload),
		EmittedAt: s.nowRFC3339(),
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return nil, "", err
	}
	data = append(data, '\n')
	p := SignalPath(ticketID, seq)
	if err := s.FS.WriteFile(p, data); err != nil {
		return nil, "", err
	}
	return rec, p, nil
}

// SignalsForTicket returns stored signals for a ticket in sequence order.
func (s *Store) SignalsForTicket(ticketID string) ([]SignalRecord, error) {
	if err := validateSignalTicket(ticketID); err != nil {
		return nil, err
	}
	entries, err := s.FS.ReadDir(signalsRoot + "/" + ticketID)
	if err != nil {
		return nil, nil
	}
	records := make([]SignalRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := s.FS.ReadFile(signalsRoot + "/" + ticketID + "/" + entry.Name())
		if err != nil {
			return nil, err
		}
		var rec SignalRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return nil, fmt.Errorf("corrupt signal %s/%s: %w", ticketID, entry.Name(), err)
		}
		records = append(records, rec)
	}
	return records, nil
}

// SignalPath returns the tree path for a signal sequence number.
func SignalPath(ticketID string, seq int) string {
	return fmt.Sprintf("%s/%s/%04d.json", signalsRoot, ticketID, seq)
}

// RestageSignal writes an already-serialized signal record at its original path.
func (s *Store) RestageSignal(ticketID, path string, content []byte) error {
	if err := validateSignalTicket(ticketID); err != nil {
		return err
	}
	if !strings.HasPrefix(path, signalsRoot+"/"+ticketID+"/") || !strings.HasSuffix(path, ".json") {
		return fmt.Errorf("invalid signal path %q for ticket %s", path, ticketID)
	}
	if strings.ContainsAny(path, "\n\r") {
		return fmt.Errorf("invalid signal path %q", path)
	}
	return s.FS.WriteFile(path, content)
}

// ReadSignalSource reads a signal blob from the current tree or SourceHash.
func (s *Store) ReadSignalSource(ticketID, path string) ([]byte, error) {
	if err := validateSignalTicket(ticketID); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(path, signalsRoot+"/"+ticketID+"/") {
		return nil, fmt.Errorf("invalid signal path %q for ticket %s", path, ticketID)
	}
	if data, err := s.FS.ReadFile(path); err == nil {
		return data, nil
	}
	if !s.SourceHash.IsZero() {
		data, err := s.FS.ReadFileAt(s.SourceHash, path)
		if err == nil {
			return data, nil
		}
		if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("read signal from source tree: %w", err)
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrSignalNotFound, path)
}

func (s *Store) nextSignalSeq(ticketID string) (int, error) {
	entries, err := s.FS.ReadDir(signalsRoot + "/" + ticketID)
	if err != nil {
		return 1, nil
	}
	maxSeq := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			continue
		}
		if n > maxSeq {
			maxSeq = n
		}
	}
	return maxSeq + 1, nil
}

func validateSignalTicket(ticketID string) error {
	if ticketID == "" {
		return fmt.Errorf("ticket id is empty")
	}
	if strings.ContainsAny(ticketID, " \t\n\r/") {
		return fmt.Errorf("invalid ticket id %q", ticketID)
	}
	return nil
}

func copyPayload(payload map[string]any) map[string]any {
	cp := make(map[string]any, len(payload))
	for k, v := range payload {
		cp[k] = v
	}
	return cp
}
