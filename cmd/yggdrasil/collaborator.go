package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/dakasa-yggdrasil/yggdrasil/internal/corecli"
)

func runCollaborator(args []string) error {
	if len(args) < 1 {
		printCollaboratorUsage()
		return fmt.Errorf("collaborator subcommand required")
	}
	verb := args[0]
	rest := args[1:]
	switch verb {
	case "create":
		return runCollaboratorCreate(rest)
	case "get":
		return runCollaboratorGet(rest)
	case "list":
		return runCollaboratorList(rest)
	case "update":
		return runCollaboratorUpdate(rest)
	case "offboard":
		return runCollaboratorOffboard(rest)
	case "suspend":
		return runCollaboratorSuspend(rest)
	case "unsuspend":
		return runCollaboratorUnsuspend(rest)
	case "re-onboard":
		return runCollaboratorReOnboard(rest)
	case "role-change":
		return runCollaboratorRoleChange(rest)
	case "team-add":
		return runCollaboratorTeamAdd(rest)
	case "team-remove":
		return runCollaboratorTeamRemove(rest)
	case "attribute-set":
		return runCollaboratorAttributeSet(rest)
	case "manager-change":
		return runCollaboratorManagerChange(rest)
	case "absence-start":
		return runCollaboratorAbsenceStart(rest)
	case "absence-end":
		return runCollaboratorAbsenceEnd(rest)
	case "lifecycle-events":
		return runCollaboratorLifecycleEvents(rest)
	case "provider-state":
		return runCollaboratorProviderState(rest)
	case "help", "-h", "--help":
		printCollaboratorUsage()
		return nil
	default:
		printCollaboratorUsage()
		return fmt.Errorf("unknown collaborator subcommand: %s", verb)
	}
}

func printCollaboratorUsage() {
	fmt.Println(`yggdrasil collaborator <verb> [options]

Lifecycle:
  create           --slug <s> --display-name <n> [--email <e> --status <s> --start-date <YYYY-MM-DD> --role <r> --manager <id> --team <id>]
  get <id|slug>
  list             [--status <s>]
  update <id>      --display-name <n> | --email <e> | --status <s>
  offboard <id>    --reason <voluntary|involuntary|contract-end|deceased> [--end-date <YYYY-MM-DD> --notice-days <n>]
  suspend <id>     [--reason <r>]
  unsuspend <id>   [--reason <r>]
  re-onboard <id>  [--start-date <YYYY-MM-DD> --role <r>]

Field changes:
  role-change <id>      --new-role <r>
  team-add <id>         --team <team-id> [--role-in-team <r>]
  team-remove <id>      --team <team-id>
  attribute-set <id>    --key <k> --value <v> [--type <string|number|bool|json>]
  manager-change <id>   --new-manager <id>

Absence:
  absence-start <id>    --type <vacation|leave-medical|leave-parental|leave-sabbatical> --from <d> --to <d> [--days <n>]
  absence-end <id>      [--absence-event-id <id> --actual-end <d>]

Audit:
  lifecycle-events <id> [--type <event-type> --limit <n>]
  provider-state <id>`)
}

func newCollaboratorClient() (*corecli.Client, error) {
	return corecli.FromContext("", "")
}

func runCollaboratorCreate(args []string) error {
	fs := flag.NewFlagSet("collaborator create", flag.ContinueOnError)
	slug := fs.String("slug", "", "slug")
	display := fs.String("display-name", "", "display name")
	email := fs.String("email", "", "primary email")
	status := fs.String("status", "active", "initial status")
	startDate := fs.String("start-date", "", "employment_data.start_date YYYY-MM-DD")
	role := fs.String("role", "", "employment_data.role")
	manager := fs.String("manager", "", "manager id")
	team := fs.String("team", "", "primary team id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *slug == "" || *display == "" {
		return fmt.Errorf("--slug and --display-name are required")
	}
	client, err := newCollaboratorClient()
	if err != nil {
		return err
	}
	req := corecli.CreateCollaboratorRequest{
		Slug:          *slug,
		DisplayName:   *display,
		PrimaryEmail:  *email,
		Status:        *status,
		ManagerID:     *manager,
		PrimaryTeamID: *team,
	}
	if *startDate != "" || *role != "" {
		emp := map[string]any{}
		if *startDate != "" {
			emp["start_date"] = *startDate
		}
		if *role != "" {
			emp["role"] = *role
		}
		req.EmploymentData = emp
	}
	collab, err := client.CreateCollaborator(context.Background(), req)
	if err != nil {
		return err
	}
	return printCollaboratorJSON(collab)
}

func runCollaboratorGet(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("collaborator id or slug required")
	}
	client, err := newCollaboratorClient()
	if err != nil {
		return err
	}
	collab, err := client.GetCollaborator(context.Background(), args[0])
	if err != nil {
		return err
	}
	return printCollaboratorJSON(collab)
}

func runCollaboratorList(args []string) error {
	fs := flag.NewFlagSet("collaborator list", flag.ContinueOnError)
	status := fs.String("status", "", "filter by status")
	if err := fs.Parse(args); err != nil {
		return err
	}
	client, err := newCollaboratorClient()
	if err != nil {
		return err
	}
	collabs, err := client.ListCollaborators(context.Background(), *status)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"collaborators": collabs})
}

func runCollaboratorUpdate(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("collaborator id required")
	}
	id := args[0]
	fs := flag.NewFlagSet("collaborator update", flag.ContinueOnError)
	display := fs.String("display-name", "", "")
	email := fs.String("email", "", "")
	status := fs.String("status", "", "")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	body := map[string]any{}
	if *display != "" {
		body["display_name"] = *display
	}
	if *email != "" {
		body["primary_email"] = *email
	}
	if *status != "" {
		body["status"] = *status
	}
	if len(body) == 0 {
		return fmt.Errorf("at least one field flag required")
	}
	client, err := newCollaboratorClient()
	if err != nil {
		return err
	}
	collab, err := client.UpdateCollaborator(context.Background(), id, body)
	if err != nil {
		return err
	}
	return printCollaboratorJSON(collab)
}

func runCollaboratorOffboard(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("collaborator id required")
	}
	id := args[0]
	fs := flag.NewFlagSet("collaborator offboard", flag.ContinueOnError)
	reason := fs.String("reason", "", "voluntary|involuntary|contract-end|deceased")
	endDate := fs.String("end-date", "", "")
	noticeDays := fs.Int("notice-days", 0, "")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *reason == "" {
		return fmt.Errorf("--reason is required")
	}
	client, err := newCollaboratorClient()
	if err != nil {
		return err
	}
	collab, err := client.OffboardCollaborator(context.Background(), id, *reason, *endDate, *noticeDays)
	if err != nil {
		return err
	}
	return printCollaboratorJSON(collab)
}

func runCollaboratorSuspend(args []string) error   { return statusVerb(args, "suspend") }
func runCollaboratorUnsuspend(args []string) error { return statusVerb(args, "unsuspend") }

func statusVerb(args []string, verb string) error {
	if len(args) < 1 {
		return fmt.Errorf("collaborator id required")
	}
	id := args[0]
	fs := flag.NewFlagSet("collaborator "+verb, flag.ContinueOnError)
	reason := fs.String("reason", "", "")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	client, err := newCollaboratorClient()
	if err != nil {
		return err
	}
	var collab corecli.Collaborator
	if verb == "suspend" {
		collab, err = client.SuspendCollaborator(context.Background(), id, *reason)
	} else {
		collab, err = client.UnsuspendCollaborator(context.Background(), id, *reason)
	}
	if err != nil {
		return err
	}
	return printCollaboratorJSON(collab)
}

func runCollaboratorReOnboard(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("collaborator id required")
	}
	id := args[0]
	fs := flag.NewFlagSet("collaborator re-onboard", flag.ContinueOnError)
	startDate := fs.String("start-date", "", "")
	role := fs.String("role", "", "")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	client, err := newCollaboratorClient()
	if err != nil {
		return err
	}
	collab, err := client.ReOnboardCollaborator(context.Background(), id, *startDate, *role)
	if err != nil {
		return err
	}
	return printCollaboratorJSON(collab)
}

func runCollaboratorRoleChange(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("collaborator id required")
	}
	id := args[0]
	fs := flag.NewFlagSet("collaborator role-change", flag.ContinueOnError)
	newRole := fs.String("new-role", "", "required")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *newRole == "" {
		return fmt.Errorf("--new-role is required")
	}
	client, err := newCollaboratorClient()
	if err != nil {
		return err
	}
	collab, err := client.ChangeRole(context.Background(), id, *newRole)
	if err != nil {
		return err
	}
	return printCollaboratorJSON(collab)
}

func runCollaboratorTeamAdd(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("collaborator id required")
	}
	id := args[0]
	fs := flag.NewFlagSet("collaborator team-add", flag.ContinueOnError)
	team := fs.String("team", "", "team id")
	roleInTeam := fs.String("role-in-team", "", "")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *team == "" {
		return fmt.Errorf("--team is required")
	}
	client, err := newCollaboratorClient()
	if err != nil {
		return err
	}
	collab, err := client.AddTeam(context.Background(), id, *team, *roleInTeam)
	if err != nil {
		return err
	}
	return printCollaboratorJSON(collab)
}

func runCollaboratorTeamRemove(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("collaborator id required")
	}
	id := args[0]
	fs := flag.NewFlagSet("collaborator team-remove", flag.ContinueOnError)
	team := fs.String("team", "", "team id")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *team == "" {
		return fmt.Errorf("--team is required")
	}
	client, err := newCollaboratorClient()
	if err != nil {
		return err
	}
	collab, err := client.RemoveTeam(context.Background(), id, *team)
	if err != nil {
		return err
	}
	return printCollaboratorJSON(collab)
}

func runCollaboratorAttributeSet(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("collaborator id required")
	}
	id := args[0]
	fs := flag.NewFlagSet("collaborator attribute-set", flag.ContinueOnError)
	key := fs.String("key", "", "attribute key")
	value := fs.String("value", "", "attribute value (string)")
	valueType := fs.String("type", "", "string|number|bool|json")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *key == "" {
		return fmt.Errorf("--key is required")
	}
	var typed any = *value
	switch *valueType {
	case "json":
		var parsed any
		if err := json.Unmarshal([]byte(*value), &parsed); err != nil {
			return fmt.Errorf("decode --value as json: %w", err)
		}
		typed = parsed
	}
	client, err := newCollaboratorClient()
	if err != nil {
		return err
	}
	collab, err := client.SetAttribute(context.Background(), id, *key, typed, *valueType)
	if err != nil {
		return err
	}
	return printCollaboratorJSON(collab)
}

func runCollaboratorManagerChange(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("collaborator id required")
	}
	id := args[0]
	fs := flag.NewFlagSet("collaborator manager-change", flag.ContinueOnError)
	newMgr := fs.String("new-manager", "", "manager id (empty to clear)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	client, err := newCollaboratorClient()
	if err != nil {
		return err
	}
	collab, err := client.ChangeManager(context.Background(), id, *newMgr)
	if err != nil {
		return err
	}
	return printCollaboratorJSON(collab)
}

func runCollaboratorAbsenceStart(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("collaborator id required")
	}
	id := args[0]
	fs := flag.NewFlagSet("collaborator absence-start", flag.ContinueOnError)
	absType := fs.String("type", "", "vacation|leave-medical|leave-parental|leave-sabbatical")
	from := fs.String("from", "", "")
	to := fs.String("to", "", "")
	days := fs.Int("days", 0, "")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *absType == "" {
		return fmt.Errorf("--type is required")
	}
	client, err := newCollaboratorClient()
	if err != nil {
		return err
	}
	collab, err := client.StartAbsence(context.Background(), id, *absType, *from, *to, *days)
	if err != nil {
		return err
	}
	return printCollaboratorJSON(collab)
}

func runCollaboratorAbsenceEnd(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("collaborator id required")
	}
	id := args[0]
	fs := flag.NewFlagSet("collaborator absence-end", flag.ContinueOnError)
	eventID := fs.String("absence-event-id", "", "")
	actualEnd := fs.String("actual-end", "", "")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	client, err := newCollaboratorClient()
	if err != nil {
		return err
	}
	collab, err := client.EndAbsence(context.Background(), id, *eventID, *actualEnd)
	if err != nil {
		return err
	}
	return printCollaboratorJSON(collab)
}

func runCollaboratorLifecycleEvents(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("collaborator id required")
	}
	id := args[0]
	fs := flag.NewFlagSet("collaborator lifecycle-events", flag.ContinueOnError)
	eventType := fs.String("type", "", "")
	limit := fs.Int("limit", 100, "")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	client, err := newCollaboratorClient()
	if err != nil {
		return err
	}
	events, err := client.ListLifecycleEvents(context.Background(), id, *eventType, *limit)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"events": events})
}

func runCollaboratorProviderState(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("collaborator id required")
	}
	id := args[0]
	client, err := newCollaboratorClient()
	if err != nil {
		return err
	}
	state, err := client.GetProviderState(context.Background(), id)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"provider_state": state})
}

func printCollaboratorJSON(c corecli.Collaborator) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(c)
}

// avoid unused import in case some flag parse paths get pruned
var _ = strings.TrimSpace
