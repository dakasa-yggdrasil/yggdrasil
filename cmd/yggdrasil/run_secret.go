package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/term"
	"github.com/dakasa-yggdrasil/yggdrasil/internal/corecli"
)

var (
	errPrivateInputKeys = errors.New("private workflow input: use distinct declared keys only, without values or overlapping --input keys")
	errPrivateTerminal  = errors.New("private workflow input: an interactive terminal is required; pipes and redirected streams are refused")
	errPrivateManifest  = errors.New("private workflow input: could not verify one active workflow and its declared string secret/sensitive inputs")
	errPrivatePrompt    = errors.New("private workflow input: entry canceled or unavailable; no run was dispatched")
	errPrivateValue     = errors.New("private workflow input: empty, surrounding whitespace, control characters or oversized values are refused; no run was dispatched")
	errPrivateDispatch  = errors.New("private workflow dispatch not confirmed; inspect existing runs with a read-only GET before starting another run")
	errPrivateOutput    = errors.New("private workflow dispatch response could not be reported; inspect existing runs with a read-only GET before starting another run")
	privateKeyPattern   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,127}$`)
	privateRunIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

type privateInputIO struct {
	terminal func() bool
	prompt   func(context.Context, string) (string, error)
	out      io.Writer
}

func terminalPrivateInputIO() privateInputIO {
	return privateInputIO{
		terminal: func() bool {
			return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd()) && term.IsTerminal(os.Stderr.Fd())
		},
		prompt: promptPrivateInput,
		out:    os.Stdout,
	}
}

func promptPrivateInput(ctx context.Context, key string) (value string, err error) {
	// Do not allow a terminal-library failure or accessible-mode fallback to
	// expose a value. The caller checks all three streams before this form.
	defer func() {
		if recover() != nil {
			value, err = "", errPrivatePrompt
		}
	}()
	field := huh.NewInput().Title("Private workflow input: " + key).EchoMode(huh.EchoModePassword).Value(&value)
	err = huh.NewForm(huh.NewGroup(field)).WithInput(os.Stdin).WithOutput(os.Stderr).WithAccessible(false).RunWithContext(ctx)
	if err != nil {
		return "", errPrivatePrompt
	}
	return value, nil
}

func validatePrivateInputKeys(keys []string, ordinary map[string]any) ([]string, error) {
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		if !privateKeyPattern.MatchString(key) || seen[key] {
			return nil, errPrivateInputKeys
		}
		if _, conflict := ordinary[key]; conflict {
			return nil, errPrivateInputKeys
		}
		seen[key] = true
	}
	return append([]string(nil), keys...), nil
}

func verifyPrivateInputManifest(items []map[string]any, namespace, name string, keys []string) bool {
	if len(items) != 1 || items[0]["kind"] != "workflow" {
		return false
	}
	id, ok := items[0]["id"].(string)
	if !ok || !privateRunIDPattern.MatchString(id) || id == "00000000-0000-0000-0000-000000000000" {
		return false
	}
	if _, ok := privateWorkflowVersion(items[0]["version"]); !ok {
		return false
	}
	metadata, ok := items[0]["metadata"].(map[string]any)
	if !ok || metadata["name"] != name || metadata["namespace"] != namespace || metadata["active"] != true {
		return false
	}
	spec, ok := items[0]["spec"].(map[string]any)
	if !ok {
		return false
	}
	schema, ok := spec["input_schema"].(map[string]any)
	if !ok {
		return false
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return false
	}
	for _, key := range keys {
		property, ok := properties[key].(map[string]any)
		if !ok || property["type"] != "string" {
			return false
		}
		marked := false
		for _, marker := range []string{"secret", "sensitive"} {
			if raw, present := property[marker]; present {
				value, valid := raw.(bool)
				if !valid {
					return false
				}
				marked = marked || value
			}
		}
		if !marked {
			return false
		}
	}
	return true
}

func privateWorkflowVersion(raw any) (int, bool) {
	version, ok := raw.(float64) // ListManifests decodes the JSON record into a map.
	if !ok || version < 1 || version > math.MaxInt32 || math.Trunc(version) != version {
		return 0, false
	}
	return int(version), true
}

func validPrivateValue(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || len(value) > 64*1024 || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func validPrivateRunResponse(result corecli.WorkflowRunResult, keys []string, inputs map[string]any) bool {
	if !privateRunIDPattern.MatchString(result.RunID) || result.RunID == "00000000-0000-0000-0000-000000000000" {
		return false
	}
	switch result.Status {
	case "pending", "queued", "running", "succeeded", "committed", "failed", "cancelled", "canceled", "aborted":
	default:
		return false
	}
	for _, key := range keys {
		if value, ok := inputs[key].(string); ok && (result.RunID == value || result.Status == value) {
			return false
		}
	}
	return true
}

func runPrivateWorkflow(ctx context.Context, client *corecli.Client, namespace, name string, inputs map[string]any, keys []string, terminal privateInputIO) error {
	if terminal.terminal == nil || !terminal.terminal() || terminal.prompt == nil || terminal.out == nil {
		return errPrivateTerminal
	}
	// Refuse every redirect, including non-session credentials: a private POST
	// body must never be implicitly replayed to a new location.
	privateClient := *client
	httpClient := *client.HTTPClient
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	httpClient.Jar = nil
	privateClient.HTTPClient = &httpClient
	items, err := privateClient.ListManifests(ctx, "workflow", namespace, name, true)
	if err != nil || !verifyPrivateInputManifest(items, namespace, name, keys) {
		return errPrivateManifest
	}
	version, _ := privateWorkflowVersion(items[0]["version"])
	selector := corecli.WorkflowSelector{Namespace: namespace, Name: name, Version: &version}
	defer func() {
		for _, key := range keys {
			delete(inputs, key)
		}
	}()
	for _, key := range keys {
		if err := ctx.Err(); err != nil || !terminal.terminal() {
			return errPrivatePrompt
		}
		value, err := terminal.prompt(ctx, key)
		if err != nil {
			return errPrivatePrompt
		}
		if !validPrivateValue(value) {
			return errPrivateValue
		}
		inputs[key] = value
	}
	// Keep all provider errors opaque once private values enter RAM. Core's
	// ordinary APIError/decode errors may contain response bodies or URLs.
	result, err := privateClient.RunWorkflowSelectorMode(ctx, selector, inputs, true)
	if err != nil || !validPrivateRunResponse(result, keys, inputs) {
		return errPrivateDispatch
	}
	if _, err := fmt.Fprintf(terminal.out, "run %s\nstatus %s\n", result.RunID, result.Status); err != nil {
		return errPrivateOutput
	}
	return nil
}
