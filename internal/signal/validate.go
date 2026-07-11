package signal

import (
	"fmt"
	"strconv"
	"strings"
)

// ValidationError distinguishes payload/type validation from config errors.
type ValidationError struct {
	Err error
}

func (e ValidationError) Error() string {
	return "VALIDATION: " + e.Err.Error()
}

func (e ValidationError) Unwrap() error { return e.Err }

// ValidatePayload validates and coerces a final signal payload against typ.
func ValidatePayload(typ Type, payload map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(payload))
	fields := make(map[string]Field, len(typ.Fields))
	for _, f := range typ.Fields {
		fields[f.Name] = f
	}

	for key, val := range payload {
		f, ok := fields[key]
		if !ok {
			return nil, ValidationError{Err: fmt.Errorf("unknown field %s for type %s", key, typ.Name)}
		}
		coerced, err := coerceField(f, val)
		if err != nil {
			return nil, ValidationError{Err: err}
		}
		out[key] = coerced
	}

	for _, f := range typ.Fields {
		required := f.Required
		if f.RequiredWhen != nil && requiredWhenMatches(out, *f.RequiredWhen) {
			required = true
		}
		if required {
			if _, ok := out[f.Name]; !ok {
				return nil, ValidationError{Err: fmt.Errorf("missing required field %s", f.Name)}
			}
		}
	}
	return out, nil
}

func requiredWhenMatches(payload map[string]any, cond RequiredWhen) bool {
	val, ok := payload[cond.Field]
	if !ok {
		return false
	}
	return fmt.Sprint(val) == cond.Equals
}

func coerceField(f Field, val any) (any, error) {
	switch f.Kind {
	case "string":
		s, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("field %s must be string", f.Name)
		}
		return s, nil
	case "int":
		switch v := val.(type) {
		case int:
			return v, nil
		case int64:
			return int(v), nil
		case float64:
			if v != float64(int(v)) {
				return nil, fmt.Errorf("field %s must be int", f.Name)
			}
			return int(v), nil
		case string:
			i, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("field %s must be int", f.Name)
			}
			return i, nil
		default:
			return nil, fmt.Errorf("field %s must be int", f.Name)
		}
	case "bool":
		switch v := val.(type) {
		case bool:
			return v, nil
		case string:
			b, err := strconv.ParseBool(v)
			if err != nil {
				return nil, fmt.Errorf("field %s must be bool", f.Name)
			}
			return b, nil
		default:
			return nil, fmt.Errorf("field %s must be bool", f.Name)
		}
	case "enum":
		s, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("field %s must be enum string", f.Name)
		}
		for _, allowed := range f.Values {
			if s == allowed {
				return s, nil
			}
		}
		return nil, fmt.Errorf("field %s value %q not in [%s]", f.Name, s, strings.Join(f.Values, ", "))
	default:
		return nil, fmt.Errorf("field %s has invalid type %q", f.Name, f.Kind)
	}
}
