package quickstartcli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// Lipgloss styles used by the install flow's banners/summaries. Kept simple
// and intentionally a bit minimalist so we don't fight terminal themes.
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("99")).
			Padding(0, 1)

	subtitleStyle = lipgloss.NewStyle().
			Faint(true)

	requirementStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("214")).
				Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Bold(true)

	hintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Italic(true)
)

// PrintBanner shows the manifest's display name + repo_ref + provider list
// before the form starts. Pure cosmetic context for the adopter.
func PrintBanner(doc QuickstartDocument, ref RepoRef) {
	name := doc.Spec.DisplayName
	if name == "" {
		name = doc.Metadata.Name
	}
	fmt.Println(titleStyle.Render(name))
	fmt.Println(subtitleStyle.Render("source: " + ref.String()))
	if doc.Spec.Description != "" {
		fmt.Println()
		fmt.Println(strings.TrimSpace(doc.Spec.Description))
	}
	fmt.Println()
}

// PrintRequirements shows the provider's `requires` list so the adopter
// knows what must already be installed BEFORE they walk through the form
// (the server enforces this for real, but failing here saves a round trip).
func PrintRequirements(provider QuickstartProvider) {
	if len(provider.Requires) == 0 {
		return
	}
	fmt.Println(requirementStyle.Render("Requires:"))
	for _, r := range provider.Requires {
		var line string
		switch r.Kind {
		case "integration_family", "integration_type":
			line = fmt.Sprintf("  • %s: %s", r.Kind, r.Name)
		case "cluster_capability":
			line = fmt.Sprintf("  • cluster_capability: %s", r.Capability)
		default:
			line = fmt.Sprintf("  • %s", r.Kind)
		}
		if r.Reason != "" {
			line += subtitleStyle.Render(" — " + r.Reason)
		}
		fmt.Println(line)
	}
	fmt.Println()
}

// PickProvider returns the only provider when there is exactly one, or
// runs a huh.Select picker when there are multiple. preselected (from
// --provider) wins outright when matched.
func PickProvider(spec QuickstartSpec, preselected string) (QuickstartProvider, error) {
	if preselected != "" {
		return spec.FindProvider(preselected)
	}
	if len(spec.Providers) == 1 {
		return spec.Providers[0], nil
	}
	if len(spec.Providers) == 0 {
		return QuickstartProvider{}, fmt.Errorf("manifest has no providers")
	}

	options := make([]huh.Option[string], 0, len(spec.Providers))
	for _, p := range spec.Providers {
		label := p.DisplayName
		if label == "" {
			label = p.ID
		}
		options = append(options, huh.NewOption(label+"  ("+p.ID+")", p.ID))
	}

	var picked string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Choose a provider").
				Description("This integration ships multiple backends. Pick the one that matches your environment.").
				Options(options...).
				Value(&picked),
		),
	)
	if err := form.Run(); err != nil {
		return QuickstartProvider{}, fmt.Errorf("provider picker: %w", err)
	}
	return spec.FindProvider(picked)
}

// CollectInputs walks the provider's input schema and builds a map of
// answered values via huh forms. seeds (from --input k=v flags) prefill the
// fields so the user only confirms what wasn't already set on the CLI.
//
// We render ONE huh.Group per input so the user can step backwards within
// a single form flow — splitting them keeps state per-question simple.
func CollectInputs(provider QuickstartProvider, seeds map[string]string) (map[string]any, error) {
	out := make(map[string]any, len(provider.Inputs))

	for _, in := range provider.Inputs {
		typ := normalizeType(in.Type)
		seedRaw, hasSeed := seeds[in.ID]

		switch typ {
		case "boolean":
			value := boolDefault(in.Default)
			if hasSeed {
				if b, err := strconv.ParseBool(seedRaw); err == nil {
					value = b
				}
			}
			form := huh.NewForm(huh.NewGroup(
				huh.NewConfirm().
					Title(in.Label).
					Description(in.Description).
					Value(&value),
			))
			if err := form.Run(); err != nil {
				return nil, fmt.Errorf("input %s: %w", in.ID, err)
			}
			out[in.ID] = value

		case "select":
			options := make([]huh.Option[string], 0, len(in.Choices))
			for _, c := range in.Choices {
				lbl := c.Label
				if lbl == "" {
					lbl = c.Value
				}
				options = append(options, huh.NewOption(lbl, c.Value))
			}
			value := stringDefault(in.Default)
			if hasSeed {
				value = seedRaw
			}
			form := huh.NewForm(huh.NewGroup(
				huh.NewSelect[string]().
					Title(in.Label).
					Description(in.Description).
					Options(options...).
					Value(&value),
			))
			if err := form.Run(); err != nil {
				return nil, fmt.Errorf("input %s: %w", in.ID, err)
			}
			out[in.ID] = value

		case "integer":
			value := stringDefault(in.Default)
			if hasSeed {
				value = seedRaw
			}
			input := huh.NewInput().
				Title(in.Label).
				Description(in.Description).
				Value(&value).
				Validate(integerValidator(in))
			if err := huh.NewForm(huh.NewGroup(input)).Run(); err != nil {
				return nil, fmt.Errorf("input %s: %w", in.ID, err)
			}
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return nil, fmt.Errorf("input %s: not an integer (got %q)", in.ID, value)
			}
			out[in.ID] = n

		default:
			value := stringDefault(in.Default)
			if hasSeed {
				value = seedRaw
			}
			input := huh.NewInput().
				Title(in.Label).
				Description(in.Description).
				Value(&value).
				Validate(stringValidator(in))
			if in.Sensitive {
				input = input.EchoMode(huh.EchoModePassword)
			}
			if err := huh.NewForm(huh.NewGroup(input)).Run(); err != nil {
				return nil, fmt.Errorf("input %s: %w", in.ID, err)
			}
			value = strings.TrimSpace(value)
			if value == "" && !in.Required {
				continue // omit optional empties — server treats absent as "use default"
			}
			out[in.ID] = value
		}
	}

	return out, nil
}

// ConfirmInstall renders a summary and asks the user to proceed. dryRun
// flips the wording to make it clear nothing is being applied yet.
func ConfirmInstall(provider QuickstartProvider, inputs map[string]any, dryRun bool) (bool, error) {
	fmt.Println(titleStyle.Render("Review"))
	fmt.Printf("provider: %s\n", provider.ID)
	for _, in := range provider.Inputs {
		v, ok := inputs[in.ID]
		if !ok {
			continue
		}
		display := fmt.Sprintf("%v", v)
		if in.Sensitive {
			display = strings.Repeat("•", len(display))
		}
		fmt.Printf("  %s = %s\n", in.ID, display)
	}
	fmt.Println()

	prompt := "Submit install request to the server?"
	if dryRun {
		prompt = "Dry-run: ask the server to compile and return the workflow without dispatching it?"
	}
	var confirm bool
	form := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Title(prompt).Value(&confirm),
	))
	if err := form.Run(); err != nil {
		return false, err
	}
	return confirm, nil
}

// PrintResult renders the server's response after a successful install.
// Hints are rendered in a low-priority style so they don't compete with
// the success/run-id headline.
func PrintResult(resp InstallResponse, dryRun bool) {
	fmt.Println()
	if dryRun {
		fmt.Println(successStyle.Render("✓ Workflow compiled (no run dispatched)"))
		fmt.Printf("provider: %s\n", resp.ProviderID)
		if resp.CompiledWorkflow != nil {
			fmt.Println(subtitleStyle.Render("compiled workflow returned by the server (truncated to step IDs):"))
			if steps, ok := resp.CompiledWorkflow["steps"].([]any); ok {
				for _, s := range steps {
					if step, ok := s.(map[string]any); ok {
						fmt.Printf("  • %v\n", step["id"])
					}
				}
			}
		}
	} else {
		fmt.Println(successStyle.Render("✓ Install dispatched"))
		fmt.Printf("provider: %s\n", resp.ProviderID)
		if resp.RunID != "" {
			fmt.Printf("run_id:   %s\n", resp.RunID)
		}
		if resp.RunURL != "" {
			fmt.Printf("run_url:  %s\n", resp.RunURL)
		}
	}
	if len(resp.Hints) > 0 {
		fmt.Println()
		fmt.Println(hintStyle.Render("next steps:"))
		for _, h := range resp.Hints {
			fmt.Println(hintStyle.Render("  • " + h))
		}
	}
}

// integerValidator + stringValidator turn a QuickstartInput into a huh
// Validate function. We DON'T trust the server alone — failing fast inside
// the form is much nicer UX than asking the user to retype a 7-field form
// because one regex didn't match.
func integerValidator(in QuickstartInput) func(string) error {
	return func(s string) error {
		s = strings.TrimSpace(s)
		if s == "" {
			if in.Required {
				return fmt.Errorf("required")
			}
			return nil
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("must be an integer")
		}
		if in.Validate == nil {
			return nil
		}
		if in.Validate.Min != nil && n < *in.Validate.Min {
			return fmt.Errorf("must be >= %d", *in.Validate.Min)
		}
		if in.Validate.Max != nil && n > *in.Validate.Max {
			return fmt.Errorf("must be <= %d", *in.Validate.Max)
		}
		return nil
	}
}

func stringValidator(in QuickstartInput) func(string) error {
	var compiled *regexp.Regexp
	if in.Validate != nil && in.Validate.Regex != "" {
		if re, err := regexp.Compile(in.Validate.Regex); err == nil {
			compiled = re
		}
	}
	return func(s string) error {
		s = strings.TrimSpace(s)
		if s == "" {
			if in.Required {
				return fmt.Errorf("required")
			}
			return nil
		}
		if compiled != nil && !compiled.MatchString(s) {
			if in.Validate.Message != "" {
				return fmt.Errorf("%s", in.Validate.Message)
			}
			return fmt.Errorf("does not match required pattern")
		}
		return nil
	}
}

func normalizeType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	if t == "" {
		return "string"
	}
	return t
}

func stringDefault(d any) string {
	switch v := d.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

func boolDefault(d any) bool {
	switch v := d.(type) {
	case bool:
		return v
	case string:
		b, _ := strconv.ParseBool(v)
		return b
	default:
		return false
	}
}
