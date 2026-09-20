package provider

import (
	"context"
	"testing"
)

func TestResolveRole(t *testing.T) {
	t.Parallel()
	roles := ModelRoles{Primary: "big", Weak: "small", Editor: "edit"}
	tests := []struct {
		name  string
		roles ModelRoles
		role  string
		want  string
	}{
		{"primary", roles, RolePrimary, "big"},
		{"weak", roles, RoleWeak, "small"},
		{"editor", roles, RoleEditor, "edit"},
		{"unknown defaults to primary", roles, "nonsense", "big"},
		{"empty role defaults to primary", roles, "", "big"},
		{"weak falls back to primary when empty", ModelRoles{Primary: "big"}, RoleWeak, "big"},
		{"editor falls back to primary when empty", ModelRoles{Primary: "big"}, RoleEditor, "big"},
		{"all empty returns empty", ModelRoles{}, RoleWeak, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveRole(tt.roles, tt.role); got != tt.want {
				t.Errorf("ResolveRole(%v, %q) = %q, want %q", tt.roles, tt.role, got, tt.want)
			}
		})
	}
}

func TestRoleRouter_RoutesByContextRole(t *testing.T) {
	t.Parallel()
	roles := ModelRoles{Primary: "big", Weak: "small", Editor: "edit"}
	tests := []struct {
		name      string
		ctxRole   string
		optsModel string
		wantModel string
	}{
		{"no role uses primary", "", "", "big"},
		{"weak role", RoleWeak, "", "small"},
		{"editor role", RoleEditor, "", "edit"},
		{"role overrides explicit model", RoleWeak, "explicit", "small"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := NewMockProvider(MockModeFixed)
			mock.Response = "ok"
			rr, err := NewRoleRouter(mock, roles)
			if err != nil {
				t.Fatalf("NewRoleRouter: %v", err)
			}

			ctx := context.Background()
			if tt.ctxRole != "" {
				ctx = WithRole(ctx, tt.ctxRole)
			}
			if _, err := rr.Chat(ctx, userMsg("hi"), ChatOptions{Model: tt.optsModel}); err != nil {
				t.Fatalf("Chat: %v", err)
			}
			last := mock.LastCall()
			if last == nil {
				t.Fatal("expected a recorded call")
			}
			if last.Options.Model != tt.wantModel {
				t.Errorf("model = %q, want %q", last.Options.Model, tt.wantModel)
			}
		})
	}
}

func TestRoleRouter_EmptySlotDoesNotClearModel(t *testing.T) {
	t.Parallel()
	// Primary empty: an unconfigured role must not wipe an explicit model.
	mock := NewMockProvider(MockModeFixed)
	mock.Response = "ok"
	rr, err := NewRoleRouter(mock, ModelRoles{}) // nothing configured
	if err != nil {
		t.Fatalf("NewRoleRouter: %v", err)
	}

	if _, err := rr.Chat(context.Background(), userMsg("hi"), ChatOptions{Model: "explicit"}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got := mock.LastCall().Options.Model; got != "explicit" {
		t.Errorf("model = %q, want %q (explicit model preserved)", got, "explicit")
	}
}

func TestRoleFromContext(t *testing.T) {
	t.Parallel()
	if got := RoleFromContext(context.Background()); got != "" {
		t.Errorf("expected empty role, got %q", got)
	}
	ctx := WithRole(context.Background(), RoleEditor)
	if got := RoleFromContext(ctx); got != RoleEditor {
		t.Errorf("RoleFromContext = %q, want %q", got, RoleEditor)
	}
}
