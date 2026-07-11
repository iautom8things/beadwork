package signal

import (
	"fmt"
	"sort"
)

// Config is the repo-local signal type configuration loaded from
// .beadwork/signals.yml in the working tree.
type Config struct {
	Types []Type
}

// Type describes one repo-defined signal type.
type Type struct {
	Name   string
	Fields []Field
}

// Field describes one payload field in a signal type schema.
type Field struct {
	Name         string
	Kind         string
	Values       []string
	Required     bool
	RequiredWhen *RequiredWhen
}

// RequiredWhen marks a field as required only when another field equals a
// configured value.
type RequiredWhen struct {
	Field  string
	Equals string
}

// TypeByName returns the named type definition.
func (c *Config) TypeByName(name string) (*Type, bool) {
	if c == nil {
		return nil, false
	}
	for i := range c.Types {
		if c.Types[i].Name == name {
			return &c.Types[i], true
		}
	}
	return nil, false
}

// TypeNames returns configured type names in sorted order.
func (c *Config) TypeNames() []string {
	if c == nil {
		return nil
	}
	names := make([]string, 0, len(c.Types))
	for _, typ := range c.Types {
		names = append(names, typ.Name)
	}
	sort.Strings(names)
	return names
}

func (t Type) validateDefinition() error {
	if t.Name == "" {
		return fmt.Errorf("type name is empty")
	}
	seen := make(map[string]bool, len(t.Fields))
	for _, f := range t.Fields {
		if f.Name == "" {
			return fmt.Errorf("type %s has a field with empty name", t.Name)
		}
		if seen[f.Name] {
			return fmt.Errorf("type %s declares duplicate field %s", t.Name, f.Name)
		}
		seen[f.Name] = true
		switch f.Kind {
		case "string", "int", "bool":
			if len(f.Values) > 0 {
				return fmt.Errorf("field %s.%s has values but is not enum", t.Name, f.Name)
			}
		case "enum":
			if len(f.Values) == 0 {
				return fmt.Errorf("field %s.%s enum has no values", t.Name, f.Name)
			}
			valueSeen := make(map[string]bool, len(f.Values))
			for _, v := range f.Values {
				if v == "" {
					return fmt.Errorf("field %s.%s enum contains empty value", t.Name, f.Name)
				}
				if valueSeen[v] {
					return fmt.Errorf("field %s.%s enum repeats value %s", t.Name, f.Name, v)
				}
				valueSeen[v] = true
			}
		default:
			return fmt.Errorf("field %s.%s has invalid type %q", t.Name, f.Name, f.Kind)
		}
		if f.RequiredWhen != nil {
			if f.RequiredWhen.Field == "" {
				return fmt.Errorf("field %s.%s required_when missing field", t.Name, f.Name)
			}
			if f.RequiredWhen.Equals == "" {
				return fmt.Errorf("field %s.%s required_when missing equals", t.Name, f.Name)
			}
		}
	}
	return nil
}
