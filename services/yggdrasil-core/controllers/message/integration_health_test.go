package message

import (
	"testing"
	"time"

	"github.com/dakasa-co/yggdrasil-core/model"
)

func TestIntegrationInstanceOverallStatus(t *testing.T) {
	tests := []struct {
		name         string
		declared     string
		runtimeState *model.IntegrationRuntimeState
		want         string
	}{
		{
			name:     "disabled instance wins",
			declared: "disabled",
			runtimeState: &model.IntegrationRuntimeState{
				Status: model.IntegrationRuntimeStatusHealthy,
			},
			want: "disabled",
		},
		{
			name:     "draft instance wins",
			declared: "draft",
			want:     "draft",
		},
		{
			name:     "active without runtime state is unknown",
			declared: "active",
			want:     model.IntegrationInstanceHealthStatusUnknown,
		},
		{
			name:     "active mirrors runtime state",
			declared: "active",
			runtimeState: &model.IntegrationRuntimeState{
				Status: model.IntegrationRuntimeStatusContractMismatch,
			},
			want: model.IntegrationRuntimeStatusContractMismatch,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := integrationInstanceOverallStatus(tc.declared, tc.runtimeState); got != tc.want {
				t.Fatalf("integrationInstanceOverallStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestShouldFastFailIntegrationRuntimeState(t *testing.T) {
	now := time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		state model.IntegrationRuntimeState
		want  bool
	}{
		{
			name: "healthy does not fast fail",
			state: model.IntegrationRuntimeState{
				Status:        model.IntegrationRuntimeStatusHealthy,
				LastCheckedAt: now.Add(-10 * time.Second),
			},
			want: false,
		},
		{
			name: "recent unreachable fast fails",
			state: model.IntegrationRuntimeState{
				Status:        model.IntegrationRuntimeStatusUnreachable,
				LastCheckedAt: now.Add(-10 * time.Second),
			},
			want: true,
		},
		{
			name: "stale unreachable does not fast fail",
			state: model.IntegrationRuntimeState{
				Status:        model.IntegrationRuntimeStatusUnreachable,
				LastCheckedAt: now.Add(-(integrationRuntimeFastFailWindow + time.Second)),
			},
			want: false,
		},
		{
			name: "recent contract mismatch fast fails",
			state: model.IntegrationRuntimeState{
				Status:        model.IntegrationRuntimeStatusContractMismatch,
				LastCheckedAt: now.Add(-time.Second),
			},
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldFastFailIntegrationRuntimeState(tc.state, now); got != tc.want {
				t.Fatalf("shouldFastFailIntegrationRuntimeState() = %v, want %v", got, tc.want)
			}
		})
	}
}
