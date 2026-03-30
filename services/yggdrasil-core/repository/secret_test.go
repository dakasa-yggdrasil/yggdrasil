package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestResolveSecretRefReturnsSingleValue(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New error: %v", err)
	}
	defer db.Close()

	now := time.Now().UTC()
	rows := sqlmock.NewRows([]string{
		"id",
		"namespace",
		"name",
		"status",
		"data",
		"metadata",
		"created_at",
		"updated_at",
	}).AddRow(
		uuid.New(),
		"github",
		"private-key",
		"active",
		[]byte(`{"value":"pem-data"}`),
		[]byte(`{}`),
		now,
		now,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`
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
		`)).
		WithArgs("github", "private-key").
		WillReturnRows(rows)

	value, err := ResolveSecretRef(context.Background(), db, "secret://github/private-key")
	if err != nil {
		t.Fatalf("ResolveSecretRef error: %v", err)
	}

	if value != "pem-data" {
		t.Fatalf("expected single secret value, got %#v", value)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestResolveSecretRefsResolvesNestedReferences(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New error: %v", err)
	}
	defer db.Close()

	now := time.Now().UTC()
	query := regexp.QuoteMeta(`
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
		`)

	mock.ExpectQuery(query).
		WithArgs("github", "platform").
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"namespace",
			"name",
			"status",
			"data",
			"metadata",
			"created_at",
			"updated_at",
		}).AddRow(
			uuid.New(),
			"github",
			"platform",
			"active",
			[]byte(`{"token":"ghp_123","app_id":"42"}`),
			[]byte(`{}`),
			now,
			now,
		))

	mock.ExpectQuery(query).
		WithArgs("github", "private-key").
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"namespace",
			"name",
			"status",
			"data",
			"metadata",
			"created_at",
			"updated_at",
		}).AddRow(
			uuid.New(),
			"github",
			"private-key",
			"active",
			[]byte(`{"value":"pem-data"}`),
			[]byte(`{}`),
			now,
			now,
		))

	resolved, err := ResolveSecretRefs(context.Background(), db, map[string]any{
		"credentials": "secret://github/platform",
		"nested": map[string]any{
			"private_key": "secret://github/private-key",
		},
	})
	if err != nil {
		t.Fatalf("ResolveSecretRefs error: %v", err)
	}

	payload, ok := resolved.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %#v", resolved)
	}

	credentials, ok := payload["credentials"].(map[string]any)
	if !ok {
		t.Fatalf("expected credentials object, got %#v", payload["credentials"])
	}
	if credentials["token"] != "ghp_123" || credentials["app_id"] != "42" {
		t.Fatalf("unexpected resolved credentials %#v", credentials)
	}

	nested, ok := payload["nested"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested map, got %#v", payload["nested"])
	}
	if nested["private_key"] != "pem-data" {
		t.Fatalf("unexpected nested private key %#v", nested["private_key"])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
