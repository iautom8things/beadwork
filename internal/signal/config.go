package signal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// ErrNoConfig is returned when .beadwork/signals.yml does not exist.
var ErrNoConfig = errors.New("no signal config")

// ConfigError distinguishes fail-closed config errors from validation errors.
type ConfigError struct {
	Err error
}

func (e ConfigError) Error() string {
	return "CONFIG ERROR: " + e.Err.Error()
}

func (e ConfigError) Unwrap() error { return e.Err }

// Load reads .beadwork/signals.yml from repoRoot in the working tree.
func Load(repoRoot string) (*Config, error) {
	data, err := os.ReadFile(filepath.Join(repoRoot, ".beadwork", "signals.yml"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNoConfig
		}
		return nil, ConfigError{Err: err}
	}
	cfg, err := Parse(data)
	if err != nil {
		return nil, ConfigError{Err: err}
	}
	return cfg, nil
}

// Parse parses signal configuration YAML.
func Parse(data []byte) (*Config, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("signals.yml must be a mapping")
	}
	top := root.Content[0]
	var typesNode *yaml.Node
	var hooksNode *yaml.Node
	var timeoutNode *yaml.Node
	seenTop := map[string]bool{}
	for i := 0; i < len(top.Content); i += 2 {
		key := top.Content[i].Value
		if seenTop[key] {
			return nil, fmt.Errorf("duplicate top-level key %s", key)
		}
		seenTop[key] = true
		if key == "types" {
			typesNode = top.Content[i+1]
		} else if key == "hooks" {
			hooksNode = top.Content[i+1]
		} else if key == "hook_timeout" {
			timeoutNode = top.Content[i+1]
		}
	}
	cfg := &Config{HookTimeout: 30 * time.Second}
	if hooksNode != nil {
		var err error
		cfg.Hooks, err = parseHooks(hooksNode)
		if err != nil {
			return nil, fmt.Errorf("hooks: %w", err)
		}
	}
	if timeoutNode != nil {
		d, err := time.ParseDuration(timeoutNode.Value)
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("hook_timeout must be a positive duration")
		}
		cfg.HookTimeout = d
	}
	if typesNode == nil {
		return cfg, nil
	}

	types, err := parseTypes(typesNode)
	if err != nil {
		return nil, err
	}
	cfg.Types = types
	seenTypes := make(map[string]bool, len(types))
	for _, typ := range cfg.Types {
		if seenTypes[typ.Name] {
			return nil, fmt.Errorf("duplicate signal type %s", typ.Name)
		}
		seenTypes[typ.Name] = true
		if err := typ.validateDefinition(); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

func parseTypes(n *yaml.Node) ([]Type, error) {
	switch n.Kind {
	case yaml.SequenceNode:
		types := make([]Type, 0, len(n.Content))
		for _, item := range n.Content {
			var rt rawType
			if err := item.Decode(&rt); err != nil {
				return nil, err
			}
			typ, err := rt.toType()
			if err != nil {
				return nil, err
			}
			types = append(types, typ)
		}
		return types, nil
	case yaml.MappingNode:
		types := make([]Type, 0, len(n.Content)/2)
		seen := map[string]bool{}
		for i := 0; i < len(n.Content); i += 2 {
			name := n.Content[i].Value
			if seen[name] {
				return nil, fmt.Errorf("duplicate signal type %s", name)
			}
			seen[name] = true
			var rt rawType
			if err := n.Content[i+1].Decode(&rt); err != nil {
				return nil, err
			}
			rt.Name = name
			typ, err := rt.toType()
			if err != nil {
				return nil, err
			}
			types = append(types, typ)
		}
		return types, nil
	default:
		return nil, fmt.Errorf("types must be a mapping or sequence")
	}
}

type rawType struct {
	Name   string    `yaml:"name"`
	Fields yaml.Node `yaml:"fields"`
	Hooks  yaml.Node `yaml:"hooks"`
}

type rawField struct {
	Name         string           `yaml:"name"`
	Kind         string           `yaml:"type"`
	Values       []string         `yaml:"values"`
	Required     bool             `yaml:"required"`
	RequiredWhen *rawRequiredWhen `yaml:"required_when"`
}

type rawRequiredWhen struct {
	Field  string `yaml:"field"`
	Equals any    `yaml:"equals"`
}

func (rt rawType) toType() (Type, error) {
	typ := Type{Name: rt.Name}
	if rt.Hooks.Kind != 0 {
		var err error
		typ.Hooks, err = parseHooks(&rt.Hooks)
		if err != nil {
			return Type{}, fmt.Errorf("type %s hooks: %w", rt.Name, err)
		}
	}
	if rt.Fields.Kind == 0 {
		return typ, nil
	}
	fields, err := parseFields(&rt.Fields)
	if err != nil {
		return Type{}, fmt.Errorf("type %s fields: %w", rt.Name, err)
	}
	typ.Fields = fields
	return typ, nil
}

func parseHooks(n *yaml.Node) (Hooks, error) {
	if n.Kind != yaml.MappingNode {
		return Hooks{}, fmt.Errorf("must be a mapping")
	}
	var h Hooks
	seen := map[string]bool{}
	for i := 0; i < len(n.Content); i += 2 {
		name := n.Content[i].Value
		if seen[name] {
			return Hooks{}, fmt.Errorf("duplicate moment %s", name)
		}
		seen[name] = true
		commands, err := parseCommands(n.Content[i+1])
		if err != nil {
			return Hooks{}, fmt.Errorf("%s: %w", name, err)
		}
		switch name {
		case "enrich":
			h.Enrich = commands
		case "gate":
			h.Gate = commands
		case "on-blocked", "on_blocked":
			h.OnBlocked = commands
		case "post-emit", "post_emit":
			h.PostEmit = commands
		default:
			return Hooks{}, fmt.Errorf("unknown moment %s", name)
		}
	}
	return h, nil
}

func parseCommands(n *yaml.Node) ([]string, error) {
	switch n.Kind {
	case yaml.ScalarNode:
		if n.Value == "" {
			return nil, fmt.Errorf("command is empty")
		}
		return []string{n.Value}, nil
	case yaml.SequenceNode:
		out := make([]string, 0, len(n.Content))
		for _, item := range n.Content {
			if item.Kind != yaml.ScalarNode || item.Value == "" {
				return nil, fmt.Errorf("commands must be non-empty strings")
			}
			out = append(out, item.Value)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("must be a command or command list")
	}
}

func parseFields(n *yaml.Node) ([]Field, error) {
	switch n.Kind {
	case yaml.MappingNode:
		fields := make([]Field, 0, len(n.Content)/2)
		seen := map[string]bool{}
		for i := 0; i < len(n.Content); i += 2 {
			name := n.Content[i].Value
			if seen[name] {
				return nil, fmt.Errorf("duplicate field %s", name)
			}
			seen[name] = true
			var rf rawField
			if err := n.Content[i+1].Decode(&rf); err != nil {
				return nil, err
			}
			rf.Name = name
			fields = append(fields, rf.toField())
		}
		return fields, nil
	case yaml.SequenceNode:
		fields := make([]Field, 0, len(n.Content))
		for _, item := range n.Content {
			var rf rawField
			if err := item.Decode(&rf); err != nil {
				return nil, err
			}
			fields = append(fields, rf.toField())
		}
		return fields, nil
	default:
		return nil, fmt.Errorf("fields must be a mapping or sequence")
	}
}

func (rf rawField) toField() Field {
	f := Field{
		Name:     rf.Name,
		Kind:     rf.Kind,
		Values:   rf.Values,
		Required: rf.Required,
	}
	if rf.RequiredWhen != nil {
		f.RequiredWhen = &RequiredWhen{
			Field:  rf.RequiredWhen.Field,
			Equals: fmt.Sprint(rf.RequiredWhen.Equals),
		}
	}
	return f
}
