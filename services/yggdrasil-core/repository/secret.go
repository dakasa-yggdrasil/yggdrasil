package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dakasa-co/yggdrasil-core/model"
)

var ErrManagedSecretNotFound = errors.New("managed secret not found")

// UpsertManagedSecret creates or updates one namespaced secret in the core.
func UpsertManagedSecret(ctx context.Context, db *sql.DB, req model.UpsertManagedSecretRequest) (model.ManagedSecret, error) {
	namespace := strings.ToLower(strings.TrimSpace(req.Namespace))
	if namespace == "" {
		namespace = "global"
	}

	name := strings.ToLower(strings.TrimSpace(req.Name))
	if name == "" {
		return model.ManagedSecret{}, fmt.Errorf("secret name is required")
	}

	status := strings.ToLower(strings.TrimSpace(req.Status))
	if status == "" {
		status = "active"
	}
	switch status {
	case "active", "disabled":
	default:
		return model.ManagedSecret{}, fmt.Errorf("secret status %q is unsupported", req.Status)
	}

	dataRaw, err := marshalStringMap(req.Data)
	if err != nil {
		return model.ManagedSecret{}, err
	}
	metadataRaw, err := marshalJSONObject(req.Metadata)
	if err != nil {
		return model.ManagedSecret{}, err
	}

	row := db.QueryRowContext(
		ctx,
		`
			INSERT INTO public.managed_secrets (
				namespace,
				name,
				status,
				data,
				metadata
			) VALUES (
				$1,
				$2,
				$3,
				$4::jsonb,
				$5::jsonb
			)
			ON CONFLICT (namespace, name)
			DO UPDATE SET
				status = EXCLUDED.status,
				data = EXCLUDED.data,
				metadata = EXCLUDED.metadata,
				updated_at = NOW()
			RETURNING
				id,
				namespace,
				name,
				status,
				data,
				metadata,
				created_at,
				updated_at
		`,
		namespace,
		name,
		status,
		dataRaw,
		metadataRaw,
	)

	return scanManagedSecret(row)
}

// GetManagedSecret fetches one secret by namespace and name.
func GetManagedSecret(ctx context.Context, db *sql.DB, req model.GetManagedSecretRequest) (model.ManagedSecret, error) {
	namespace := strings.ToLower(strings.TrimSpace(req.Namespace))
	if namespace == "" {
		namespace = "global"
	}
	name := strings.ToLower(strings.TrimSpace(req.Name))
	if name == "" {
		return model.ManagedSecret{}, fmt.Errorf("secret name is required")
	}

	row := db.QueryRowContext(
		ctx,
		`
			SELECT
				id,
				namespace,
				name,
				status,
				data,
				metadata,
				created_at,
				updated_at
			FROM public.managed_secrets
			WHERE namespace = $1
				AND name = $2
		`,
		namespace,
		name,
	)

	secret, err := scanManagedSecret(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.ManagedSecret{}, ErrManagedSecretNotFound
		}
		return model.ManagedSecret{}, err
	}
	return secret, nil
}

// ListManagedSecrets returns secrets matching the provided filters.
func ListManagedSecrets(ctx context.Context, db *sql.DB, req model.ListManagedSecretsRequest) ([]model.ManagedSecret, error) {
	query := `
		SELECT
			id,
			namespace,
			name,
			status,
			data,
			metadata,
			created_at,
			updated_at
		FROM public.managed_secrets
	`

	var (
		clauses []string
		args    []any
	)

	if namespace := strings.ToLower(strings.TrimSpace(req.Namespace)); namespace != "" {
		args = append(args, namespace)
		clauses = append(clauses, fmt.Sprintf("namespace = $%d", len(args)))
	}
	if status := strings.ToLower(strings.TrimSpace(req.Status)); status != "" {
		args = append(args, status)
		clauses = append(clauses, fmt.Sprintf("status = $%d", len(args)))
	}
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY namespace, name"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.ManagedSecret, 0)
	for rows.Next() {
		item, err := scanManagedSecret(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// ResolveSecretRef resolves one secret:// reference into its stored value.
func ResolveSecretRef(ctx context.Context, db *sql.DB, ref string) (any, error) {
	namespace, name, key, err := parseSecretRef(ref)
	if err != nil {
		return nil, err
	}

	secret, err := GetManagedSecret(ctx, db, model.GetManagedSecretRequest{
		Namespace: namespace,
		Name:      name,
	})
	if err != nil {
		return nil, err
	}
	if strings.ToLower(strings.TrimSpace(secret.Status)) != "active" {
		return nil, fmt.Errorf("secret %s/%s is not active", secret.Namespace, secret.Name)
	}

	if key != "" {
		value, ok := secret.Data[key]
		if !ok {
			return nil, fmt.Errorf("secret %s/%s does not define key %q", secret.Namespace, secret.Name, key)
		}
		return value, nil
	}
	if len(secret.Data) == 1 {
		for _, value := range secret.Data {
			return value, nil
		}
	}

	output := make(map[string]any, len(secret.Data))
	for name, value := range secret.Data {
		output[name] = value
	}
	return output, nil
}

// ResolveSecretRefs recursively resolves secret:// references from arbitrary input.
func ResolveSecretRefs(ctx context.Context, db *sql.DB, input any) (any, error) {
	switch value := input.(type) {
	case nil:
		return nil, nil
	case string:
		if !strings.HasPrefix(strings.TrimSpace(value), "secret://") {
			return value, nil
		}
		return ResolveSecretRef(ctx, db, value)
	case []any:
		resolved := make([]any, 0, len(value))
		for _, item := range value {
			next, err := ResolveSecretRefs(ctx, db, item)
			if err != nil {
				return nil, err
			}
			resolved = append(resolved, next)
		}
		return resolved, nil
	case map[string]any:
		resolved := make(map[string]any, len(value))
		for key, item := range value {
			next, err := ResolveSecretRefs(ctx, db, item)
			if err != nil {
				return nil, err
			}
			resolved[key] = next
		}
		return resolved, nil
	default:
		return value, nil
	}
}

func parseSecretRef(ref string) (namespace string, name string, key string, err error) {
	ref = strings.TrimSpace(ref)
	if !strings.HasPrefix(ref, "secret://") {
		return "", "", "", fmt.Errorf("secret ref %q must use secret://", ref)
	}

	target := strings.TrimPrefix(ref, "secret://")
	parts := strings.SplitN(target, "#", 2)
	pathPart := strings.Trim(parts[0], "/")
	if len(parts) == 2 {
		key = strings.TrimSpace(parts[1])
	}

	pathSegments := strings.Split(pathPart, "/")
	if len(pathSegments) < 2 {
		return "", "", "", fmt.Errorf("secret ref %q must use secret://<namespace>/<name>[#key]", ref)
	}

	namespace = strings.ToLower(strings.TrimSpace(pathSegments[0]))
	name = strings.ToLower(strings.TrimSpace(strings.Join(pathSegments[1:], "/")))
	if namespace == "" || name == "" {
		return "", "", "", fmt.Errorf("secret ref %q must use secret://<namespace>/<name>[#key]", ref)
	}

	return namespace, name, key, nil
}

func scanManagedSecret(row scanner) (model.ManagedSecret, error) {
	var (
		secret      model.ManagedSecret
		dataRaw     []byte
		metadataRaw []byte
	)

	if err := row.Scan(
		&secret.ID,
		&secret.Namespace,
		&secret.Name,
		&secret.Status,
		&dataRaw,
		&metadataRaw,
		&secret.CreatedAt,
		&secret.UpdatedAt,
	); err != nil {
		return model.ManagedSecret{}, err
	}

	data, err := unmarshalStringMap(dataRaw)
	if err != nil {
		return model.ManagedSecret{}, err
	}
	metadata, err := unmarshalJSONObject(metadataRaw)
	if err != nil {
		return model.ManagedSecret{}, err
	}

	secret.Data = data
	secret.Metadata = metadata
	return secret, nil
}

func marshalStringMap(value map[string]string) ([]byte, error) {
	if value == nil {
		return []byte("{}"), nil
	}
	for key := range value {
		if strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("object keys cannot be empty")
		}
	}
	return jsonMarshal(value)
}

func unmarshalStringMap(raw []byte) (map[string]string, error) {
	if len(raw) == 0 {
		return map[string]string{}, nil
	}
	output := map[string]string{}
	if err := jsonUnmarshal(raw, &output); err != nil {
		return nil, err
	}
	return output, nil
}

func jsonMarshal(value any) ([]byte, error) {
	return json.Marshal(value)
}

func jsonUnmarshal(raw []byte, dst any) error {
	return json.Unmarshal(raw, dst)
}
