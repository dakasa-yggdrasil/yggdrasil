package quickstartcli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// CollectInputsHeadless resolves the provider's inputs from the seeds map
// alone — no TUI, no prompts. Used by --non-interactive mode (CI / scripts).
//
// For each declared input we apply, in order:
//   1. seeds[input.id] when provided (after type coercion)
//   2. input.Default when present
//   3. drop the field when the input is optional
//
// Missing required inputs and constraint violations fail fast with a
// composite error so the operator sees ALL the gaps in one pass instead of
// one-at-a-time.
func CollectInputsHeadless(provider QuickstartProvider, seeds map[string]string) (map[string]any, error) {
	out := make(map[string]any, len(provider.Inputs))
	var errs []string

	for _, in := range provider.Inputs {
		typ := normalizeType(in.Type)
		raw, has := seeds[in.ID]

		if !has {
			if in.Default != nil {
				out[in.ID] = in.Default
				continue
			}
			if in.Required {
				errs = append(errs, fmt.Sprintf("input %q is required", in.ID))
			}
			continue
		}

		value, err := coerceSeed(in.ID, typ, raw)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		if err := enforceSeedConstraints(in, typ, value); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		out[in.ID] = value
	}

	for k := range seeds {
		if !providerHasInput(provider, k) {
			errs = append(errs, fmt.Sprintf("unknown input %q (not declared by provider)", k))
		}
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("input validation failed: %s", strings.Join(errs, "; "))
	}
	return out, nil
}

func providerHasInput(p QuickstartProvider, id string) bool {
	for _, in := range p.Inputs {
		if in.ID == id {
			return true
		}
	}
	return false
}

func coerceSeed(id, typ, raw string) (any, error) {
	switch typ {
	case "integer":
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("input %q must be an integer (got %q)", id, raw)
		}
		return n, nil
	case "boolean":
		b, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("input %q must be a boolean (got %q)", id, raw)
		}
		return b, nil
	default:
		return raw, nil
	}
}

func enforceSeedConstraints(in QuickstartInput, typ string, value any) error {
	if typ == "select" {
		s := value.(string)
		for _, c := range in.Choices {
			if c.Value == s {
				return nil
			}
		}
		return fmt.Errorf("input %q value %q is not one of the allowed choices", in.ID, s)
	}
	if in.Validate == nil {
		return nil
	}
	if in.Validate.Regex != "" {
		s, ok := value.(string)
		if !ok {
			return nil
		}
		re, err := regexp.Compile(in.Validate.Regex)
		if err != nil {
			return fmt.Errorf("input %q validate.regex is invalid: %w", in.ID, err)
		}
		if !re.MatchString(s) {
			if in.Validate.Message != "" {
				return fmt.Errorf("input %q: %s", in.ID, in.Validate.Message)
			}
			return fmt.Errorf("input %q does not match required pattern", in.ID)
		}
	}
	if typ == "integer" {
		n := value.(int)
		if in.Validate.Min != nil && n < *in.Validate.Min {
			return fmt.Errorf("input %q must be >= %d", in.ID, *in.Validate.Min)
		}
		if in.Validate.Max != nil && n > *in.Validate.Max {
			return fmt.Errorf("input %q must be <= %d", in.ID, *in.Validate.Max)
		}
	}
	return nil
}
