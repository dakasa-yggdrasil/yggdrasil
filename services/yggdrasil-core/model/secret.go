package model

import (
	"time"

	"github.com/google/uuid"
)

// ManagedSecret stores one namespaced secret owned by the core.
type ManagedSecret struct {
	ID        uuid.UUID         `json:"id"`
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	Status    string            `json:"status"`
	Data      map[string]string `json:"data,omitempty"`
	Metadata  map[string]any    `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// UpsertManagedSecretRequest creates or updates one secret record.
type UpsertManagedSecretRequest struct {
	Namespace string            `json:"namespace,omitempty"`
	Name      string            `json:"name"`
	Status    string            `json:"status,omitempty"`
	Data      map[string]string `json:"data"`
	Metadata  map[string]any    `json:"metadata,omitempty"`
}

// GetManagedSecretRequest resolves one secret by namespace and name.
type GetManagedSecretRequest struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}

// ListManagedSecretsRequest filters secret listing.
type ListManagedSecretsRequest struct {
	Namespace string `json:"namespace,omitempty"`
	Status    string `json:"status,omitempty"`
}
