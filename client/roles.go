package client

import (
	"context"
	"errors"
)

// Model role slot names. These identify a logical role that a concrete model
// fills, letting callers route a request to the appropriate model without
// hard-coding model ids at the call site.
const (
	// RolePrimary is the default, highest-capability model slot.
	RolePrimary = "primary"
	// RoleWeak is a cheaper/faster model used for auxiliary work such as
	// summarization or classification.
	RoleWeak = "weak"
	// RoleEditor is a model used to revise or refine prior output.
	RoleEditor = "editor"
)

// ModelRoles maps named role slots to concrete model ids. Empty fields fall
// back to Primary via ResolveRole.
type ModelRoles struct {
	Primary string `json:"primary,omitempty"`
	Weak    string `json:"weak,omitempty"`
	Editor  string `json:"editor,omitempty"`
}

// ResolveRole returns the model id configured for the given role, defaulting to
// Primary when the requested role is unknown or its slot is empty. An empty
// Primary returns "" so the caller's existing default-model logic still applies.
func ResolveRole(roles ModelRoles, role string) string {
	switch role {
	case RoleWeak:
		if roles.Weak != "" {
			return roles.Weak
		}
	case RoleEditor:
		if roles.Editor != "" {
			return roles.Editor
		}
	}
	return roles.Primary
}

// roleCtxKey is the context key under which a desired role is carried.
type roleCtxKey struct{}

// WithRole returns a context carrying the named role for a call. The
// RoleRouter reads this to select the model for the request.
func WithRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, roleCtxKey{}, role)
}

// RoleFromContext extracts the role from the context, if present.
func RoleFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(roleCtxKey{}).(string); ok {
		return v
	}
	return ""
}

// RoleRouter wraps a Provider and overrides ChatOptions.Model with the model
// configured for the request's role before delegating. The role is taken from
// the context (see WithRole); when absent, RolePrimary is used. The router only
// sets a model when the resolved slot is non-empty, so it never clears an
// explicit opts.Model with an unconfigured role.
//
// RoleRouter follows the same decorator pattern as BudgetProvider and
// TracingProvider: it is additive and does not change ChatOptions semantics.
type RoleRouter struct {
	inner Provider
	roles ModelRoles
}

// Compile-time check that RoleRouter implements Provider.
var _ Provider = (*RoleRouter)(nil)

// NewRoleRouter wraps inner so that requests are routed to the model configured
// for their role. The inner provider must not be nil; an error is returned
// otherwise.
func NewRoleRouter(inner Provider, roles ModelRoles) (*RoleRouter, error) {
	if inner == nil {
		return nil, errors.New("graycode-router: NewRoleRouter inner provider must not be nil")
	}
	return &RoleRouter{inner: inner, roles: roles}, nil
}

// Name returns the inner provider's name.
func (r *RoleRouter) Name() string { return r.inner.Name() }

// Ping delegates to the inner provider.
func (r *RoleRouter) Ping(ctx context.Context) error { return r.inner.Ping(ctx) }

// Chat resolves the request's role to a model, applies it to opts, then
// delegates to the inner provider.
func (r *RoleRouter) Chat(ctx context.Context, messages []GraycodeRouterMessage, opts ChatOptions) (*GraycodeRouterResponse, error) {
	return r.inner.Chat(ctx, messages, r.applyRole(ctx, opts))
}

// StreamChat resolves the request's role to a model, applies it to opts, then
// delegates to the inner provider.
func (r *RoleRouter) StreamChat(ctx context.Context, messages []GraycodeRouterMessage, opts ChatOptions) (*StreamResult, error) {
	return r.inner.StreamChat(ctx, messages, r.applyRole(ctx, opts))
}

// applyRole returns opts with Model overridden by the role's configured model,
// if one is configured. opts is passed by value so the caller's copy is
// untouched.
func (r *RoleRouter) applyRole(ctx context.Context, opts ChatOptions) ChatOptions {
	role := RoleFromContext(ctx)
	if role == "" {
		role = RolePrimary
	}
	if model := ResolveRole(r.roles, role); model != "" {
		opts.Model = model
	}
	return opts
}
