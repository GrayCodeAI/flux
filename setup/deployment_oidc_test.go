package setup

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/credentials"
)

// swapOIDCSeams overrides the injectable OIDC helpers for the duration of a
// test and restores them on cleanup.
func swapOIDCSeams(t *testing.T,
	bedrock func(context.Context, string, string) (credentials.AWSCredentials, error),
	vertex func(context.Context, string, string) (string, error),
) {
	t.Helper()
	origBedrock, origVertex := oidcBedrockCreds, oidcVertexToken
	if bedrock != nil {
		oidcBedrockCreds = bedrock
	}
	if vertex != nil {
		oidcVertexToken = vertex
	}
	t.Cleanup(func() {
		oidcBedrockCreds = origBedrock
		oidcVertexToken = origVertex
	})
}

func TestProviderForDeploymentBedrockOIDCTakenWhenEnabled(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("EYRIE_OIDC", "1")

	called := false
	swapOIDCSeams(t, func(_ context.Context, roleARN, region string) (credentials.AWSCredentials, error) {
		called = true
		if roleARN != "arn:aws:iam::123:role/ci" {
			t.Fatalf("roleARN = %q, want the configured role", roleARN)
		}
		return credentials.AWSCredentials{
			AccessKeyID:     "ASIAOIDC",
			SecretAccessKey: "oidc-secret",
			SessionToken:    "oidc-session",
			Region:          region,
		}, nil
	}, nil)

	p, ok := ProviderForDeployment("anthropic-bedrock", config.DeploymentConfig{
		Region:  "us-east-1",
		RoleARN: "arn:aws:iam::123:role/ci",
		// Note: no stored access/secret keys — only the OIDC branch can satisfy this.
	})
	if !ok {
		t.Fatal("expected bedrock deployment via OIDC to be configured")
	}
	if !called {
		t.Fatal("expected OIDC bedrock helper to be invoked")
	}
	if p.Name() != "anthropic-bedrock" {
		t.Fatalf("provider name = %q, want anthropic-bedrock", p.Name())
	}
}

func TestProviderForDeploymentBedrockOIDCSkippedWhenDisabled(t *testing.T) {
	// In Actions but neither EYRIE_OIDC nor roleARN/audience set: OIDC must be skipped.
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("EYRIE_OIDC", "")

	swapOIDCSeams(t, func(_ context.Context, _, _ string) (credentials.AWSCredentials, error) {
		t.Fatal("OIDC bedrock helper must not be called when OIDC is not enabled")
		return credentials.AWSCredentials{}, nil
	}, nil)

	p, ok := ProviderForDeployment("anthropic-bedrock", config.DeploymentConfig{
		Region:          "us-east-1",
		AccessKeyID:     "AKIATEST",
		SecretAccessKey: "secret",
	})
	if !ok {
		t.Fatal("expected bedrock deployment via stored creds")
	}
	if p.Name() != "anthropic-bedrock" {
		t.Fatalf("provider name = %q, want anthropic-bedrock", p.Name())
	}
}

func TestProviderForDeploymentBedrockOIDCSkippedOutsideActions(t *testing.T) {
	// EYRIE_OIDC=1 but not in GitHub Actions: OIDC must be skipped, fall back to stored.
	t.Setenv("GITHUB_ACTIONS", "false")
	t.Setenv("EYRIE_OIDC", "1")

	swapOIDCSeams(t, func(_ context.Context, _, _ string) (credentials.AWSCredentials, error) {
		t.Fatal("OIDC bedrock helper must not be called outside GitHub Actions")
		return credentials.AWSCredentials{}, nil
	}, nil)

	_, ok := ProviderForDeployment("anthropic-bedrock", config.DeploymentConfig{
		Region:          "us-east-1",
		AccessKeyID:     "AKIATEST",
		SecretAccessKey: "secret",
	})
	if !ok {
		t.Fatal("expected bedrock deployment via stored creds fallback")
	}
}

func TestProviderForDeploymentBedrockOIDCFallsBackOnError(t *testing.T) {
	// OIDC enabled and in Actions, but the exchange fails: must fall back to stored creds.
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("EYRIE_OIDC", "1")

	swapOIDCSeams(t, func(_ context.Context, _, _ string) (credentials.AWSCredentials, error) {
		return credentials.AWSCredentials{}, credentials.ErrNoOIDC
	}, nil)

	p, ok := ProviderForDeployment("anthropic-bedrock", config.DeploymentConfig{
		Region:          "us-east-1",
		RoleARN:         "arn:aws:iam::123:role/ci",
		AccessKeyID:     "AKIATEST",
		SecretAccessKey: "secret",
	})
	if !ok {
		t.Fatal("expected fallback to stored creds when OIDC exchange fails")
	}
	if p.Name() != "anthropic-bedrock" {
		t.Fatalf("provider name = %q, want anthropic-bedrock", p.Name())
	}
}

func TestProviderForDeploymentVertexOIDCTakenWhenAudienceSet(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("EYRIE_OIDC", "")
	t.Setenv("VERTEX_PROJECT_ID", "proj")
	t.Setenv("VERTEX_REGION", "us-central1")

	called := false
	swapOIDCSeams(t, nil, func(_ context.Context, audience, sa string) (string, error) {
		called = true
		if audience != "//iam.googleapis.com/projects/1/pool/provider" {
			t.Fatalf("audience = %q, want the configured WIF audience", audience)
		}
		if sa != "ci@proj.iam.gserviceaccount.com" {
			t.Fatalf("service account = %q, want the configured email", sa)
		}
		return "oidc-vertex-token", nil
	})

	p, ok := ProviderForDeployment("anthropic-vertex", config.DeploymentConfig{
		WIFAudience:         "//iam.googleapis.com/projects/1/pool/provider",
		ServiceAccountEmail: "ci@proj.iam.gserviceaccount.com",
		// No stored token: only the OIDC branch can satisfy this.
	})
	if !ok {
		t.Fatal("expected vertex deployment via OIDC to be configured")
	}
	if !called {
		t.Fatal("expected OIDC vertex helper to be invoked")
	}
	if p.Name() != "anthropic-vertex" {
		t.Fatalf("provider name = %q, want anthropic-vertex", p.Name())
	}
}

func TestProviderForDeploymentVertexOIDCSkippedWhenDisabled(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("EYRIE_OIDC", "")
	t.Setenv("VERTEX_PROJECT_ID", "proj")
	t.Setenv("VERTEX_REGION", "us-central1")

	swapOIDCSeams(t, nil, func(_ context.Context, _, _ string) (string, error) {
		t.Fatal("OIDC vertex helper must not be called when OIDC is not enabled")
		return "", nil
	})

	p, ok := ProviderForDeployment("anthropic-vertex", config.DeploymentConfig{
		Token: "stored-token",
	})
	if !ok {
		t.Fatal("expected vertex deployment via stored token")
	}
	if p.Name() != "anthropic-vertex" {
		t.Fatalf("provider name = %q, want anthropic-vertex", p.Name())
	}
}
