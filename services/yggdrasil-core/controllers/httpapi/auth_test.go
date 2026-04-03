package httpapi

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/dakasa-co/yggdrasil-core/model"
	"github.com/google/uuid"
)

func TestMaterializeThirdPartyAuthProviderClientSecretUsesDefaultManagedSecret(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New error: %v", err)
	}
	defer db.Close()

	now := time.Now().UTC()
	mock.ExpectQuery(regexp.QuoteMeta(`
			INSERT INTO public.managed_secrets (
				namespace,
				name,
				status,
				data,
				metadata,
				rotation,
				expires_at
			) VALUES (
				$1,
				$2,
				$3,
				$4::jsonb,
				$5::jsonb,
				$6::jsonb,
				$7
			)
			ON CONFLICT (namespace, name)
			DO UPDATE SET
				status = EXCLUDED.status,
				data = EXCLUDED.data,
				metadata = EXCLUDED.metadata,
				rotation = EXCLUDED.rotation,
				expires_at = EXCLUDED.expires_at,
				version = CASE
					WHEN public.managed_secrets.data IS DISTINCT FROM EXCLUDED.data
						THEN public.managed_secrets.version + 1
					ELSE public.managed_secrets.version
				END,
				last_rotated_at = CASE
					WHEN public.managed_secrets.data IS DISTINCT FROM EXCLUDED.data
						THEN NOW()
					ELSE public.managed_secrets.last_rotated_at
				END,
				updated_at = NOW()
			RETURNING
				id,
				namespace,
				name,
				status,
				version,
				data,
				metadata,
				rotation,
				last_rotated_at,
				expires_at,
				created_at,
				updated_at
		`)).
		WithArgs(
			"global",
			"auth-provider/github-platform/client-secret",
			"active",
			[]byte(`{"value":"top-secret"}`),
			[]byte(`{"auth_provider":{"name":"github-platform"},"source_kind":"auth_provider"}`),
			[]byte(`{"mode":"manual"}`),
			nil,
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"namespace",
			"name",
			"status",
			"version",
			"data",
			"metadata",
			"rotation",
			"last_rotated_at",
			"expires_at",
			"created_at",
			"updated_at",
		}).AddRow(
			uuid.New(),
			"global",
			"auth-provider/github-platform/client-secret",
			"active",
			1,
			[]byte(`{"value":"top-secret"}`),
			[]byte(`{"auth_provider":{"name":"github-platform"},"source_kind":"auth_provider"}`),
			[]byte(`{"mode":"manual"}`),
			now,
			nil,
			now,
			now,
		))

	server := &Server{db: db}
	req := &model.UpsertThirdPartyAuthProviderRequest{
		Name:         "github-platform",
		ClientSecret: "top-secret",
	}

	if err := server.materializeThirdPartyAuthProviderClientSecret(context.Background(), req); err != nil {
		t.Fatalf("materializeThirdPartyAuthProviderClientSecret error: %v", err)
	}

	if req.ClientSecret != "" {
		t.Fatalf("expected raw client secret to be cleared, got %q", req.ClientSecret)
	}

	expectedRef := "secret://global/auth-provider/github-platform/client-secret#value"
	if req.ClientSecretRef != expectedRef {
		t.Fatalf("expected client secret ref %q, got %q", expectedRef, req.ClientSecretRef)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMaterializeThirdPartyAuthProviderClientSecretHonorsExplicitSecretRef(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New error: %v", err)
	}
	defer db.Close()

	now := time.Now().UTC()
	mock.ExpectQuery(regexp.QuoteMeta(`
			INSERT INTO public.managed_secrets (
				namespace,
				name,
				status,
				data,
				metadata,
				rotation,
				expires_at
			) VALUES (
				$1,
				$2,
				$3,
				$4::jsonb,
				$5::jsonb,
				$6::jsonb,
				$7
			)
			ON CONFLICT (namespace, name)
			DO UPDATE SET
				status = EXCLUDED.status,
				data = EXCLUDED.data,
				metadata = EXCLUDED.metadata,
				rotation = EXCLUDED.rotation,
				expires_at = EXCLUDED.expires_at,
				version = CASE
					WHEN public.managed_secrets.data IS DISTINCT FROM EXCLUDED.data
						THEN public.managed_secrets.version + 1
					ELSE public.managed_secrets.version
				END,
				last_rotated_at = CASE
					WHEN public.managed_secrets.data IS DISTINCT FROM EXCLUDED.data
						THEN NOW()
					ELSE public.managed_secrets.last_rotated_at
				END,
				updated_at = NOW()
			RETURNING
				id,
				namespace,
				name,
				status,
				version,
				data,
				metadata,
				rotation,
				last_rotated_at,
				expires_at,
				created_at,
				updated_at
		`)).
		WithArgs(
			"identity",
			"providers/workforce/client-secret",
			"active",
			[]byte(`{"oauth":"top-secret"}`),
			[]byte(`{"auth_provider":{"name":"workforce-oidc"},"source_kind":"auth_provider"}`),
			[]byte(`{"mode":"manual"}`),
			nil,
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"namespace",
			"name",
			"status",
			"version",
			"data",
			"metadata",
			"rotation",
			"last_rotated_at",
			"expires_at",
			"created_at",
			"updated_at",
		}).AddRow(
			uuid.New(),
			"identity",
			"providers/workforce/client-secret",
			"active",
			3,
			[]byte(`{"oauth":"top-secret"}`),
			[]byte(`{"auth_provider":{"name":"workforce-oidc"},"source_kind":"auth_provider"}`),
			[]byte(`{"mode":"manual"}`),
			now,
			nil,
			now,
			now,
		))

	server := &Server{db: db}
	req := &model.UpsertThirdPartyAuthProviderRequest{
		Name:            "workforce-oidc",
		ClientSecret:    "top-secret",
		ClientSecretRef: "secret://identity/providers/workforce/client-secret#oauth",
	}

	if err := server.materializeThirdPartyAuthProviderClientSecret(context.Background(), req); err != nil {
		t.Fatalf("materializeThirdPartyAuthProviderClientSecret error: %v", err)
	}

	expectedRef := "secret://identity/providers/workforce/client-secret#oauth"
	if req.ClientSecretRef != expectedRef {
		t.Fatalf("expected client secret ref %q, got %q", expectedRef, req.ClientSecretRef)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
