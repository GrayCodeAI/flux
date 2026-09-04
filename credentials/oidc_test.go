package credentials

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDetectGitHubActions(t *testing.T) {
	tests := []struct {
		name string
		val  string
		set  bool
		want bool
	}{
		{name: "true", val: "true", set: true, want: true},
		{name: "false", val: "false", set: true, want: false},
		{name: "unset", set: false, want: false},
		{name: "garbage", val: "1", set: true, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv("GITHUB_ACTIONS", tt.val)
			} else {
				t.Setenv("GITHUB_ACTIONS", "")
			}
			if got := DetectGitHubActions(); got != tt.want {
				t.Fatalf("DetectGitHubActions() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFetchGitHubOIDCToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer req-token" {
			t.Errorf("Authorization = %q, want Bearer req-token", got)
		}
		if got := r.URL.Query().Get("audience"); got != "sts.amazonaws.com" {
			t.Errorf("audience = %q, want sts.amazonaws.com", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":"oidc-jwt-token"}`))
	}))
	defer srv.Close()

	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", srv.URL)
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "req-token")

	tok, err := FetchGitHubOIDCToken(context.Background(), AWSAudience)
	if err != nil {
		t.Fatalf("FetchGitHubOIDCToken() error = %v", err)
	}
	if tok != "oidc-jwt-token" {
		t.Fatalf("token = %q, want oidc-jwt-token", tok)
	}
}

func TestFetchGitHubOIDCToken_MissingEnv(t *testing.T) {
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")
	if _, err := FetchGitHubOIDCToken(context.Background(), AWSAudience); err == nil {
		t.Fatal("expected error when OIDC env vars are missing")
	}
}

func TestFetchGitHubOIDCToken_EmptyValue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"value":""}`))
	}))
	defer srv.Close()
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", srv.URL)
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "req-token")
	if _, err := FetchGitHubOIDCToken(context.Background(), AWSAudience); err == nil {
		t.Fatal("expected error for empty token value")
	}
}

func TestExchangeForAWS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if got := r.Form.Get("Action"); got != "AssumeRoleWithWebIdentity" {
			t.Errorf("Action = %q", got)
		}
		if got := r.Form.Get("RoleArn"); got != "arn:aws:iam::123:role/graycode-router" {
			t.Errorf("RoleArn = %q", got)
		}
		if got := r.Form.Get("WebIdentityToken"); got != "oidc-jwt" {
			t.Errorf("WebIdentityToken = %q", got)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<AssumeRoleWithWebIdentityResponse>
  <AssumeRoleWithWebIdentityResult>
    <Credentials>
      <AccessKeyId>AKIATEST</AccessKeyId>
      <SecretAccessKey>secret123</SecretAccessKey>
      <SessionToken>session456</SessionToken>
      <Expiration>2026-06-06T00:00:00Z</Expiration>
    </Credentials>
  </AssumeRoleWithWebIdentityResult>
</AssumeRoleWithWebIdentityResponse>`))
	}))
	defer srv.Close()

	ak, sk, st, err := ExchangeForAWSWith(context.Background(),
		"arn:aws:iam::123:role/graycode-router", "us-east-1", "oidc-jwt",
		AWSEndpoints{STSURL: srv.URL})
	if err != nil {
		t.Fatalf("ExchangeForAWSWith() error = %v", err)
	}
	if ak != "AKIATEST" || sk != "secret123" || st != "session456" {
		t.Fatalf("creds = %q/%q/%q, want AKIATEST/secret123/session456", ak, sk, st)
	}
}

func TestExchangeForAWS_Validation(t *testing.T) {
	tests := []struct {
		name      string
		roleARN   string
		oidcToken string
	}{
		{name: "missing role", roleARN: "", oidcToken: "tok"},
		{name: "missing token", roleARN: "arn", oidcToken: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, _, err := ExchangeForAWS(context.Background(), tt.roleARN, "us-east-1", tt.oidcToken); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestExchangeForAWS_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("<Error>denied</Error>"))
	}))
	defer srv.Close()
	if _, _, _, err := ExchangeForAWSWith(context.Background(), "arn", "us-east-1", "tok",
		AWSEndpoints{STSURL: srv.URL}); err == nil {
		t.Fatal("expected error on non-200 status")
	}
}

func TestExchangeForGCP(t *testing.T) {
	var stsHit, iamHit bool
	iam := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		iamHit = true
		if got := r.Header.Get("Authorization"); got != "Bearer federated-token" {
			t.Errorf("impersonation Authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"accessToken":"sa-access-token"}`))
	}))
	defer iam.Close()
	sts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stsHit = true
		_, _ = w.Write([]byte(`{"access_token":"federated-token"}`))
	}))
	defer sts.Close()

	tok, err := ExchangeForGCPWith(context.Background(),
		"//iam.googleapis.com/projects/1/pool/provider",
		"sa@project.iam.gserviceaccount.com", "oidc-jwt",
		GCPEndpoints{STSURL: sts.URL, IAMCredentialsHost: iam.URL})
	if err != nil {
		t.Fatalf("ExchangeForGCPWith() error = %v", err)
	}
	if !stsHit || !iamHit {
		t.Fatalf("expected both STS and IAM hit: sts=%v iam=%v", stsHit, iamHit)
	}
	if tok != "sa-access-token" {
		t.Fatalf("token = %q, want sa-access-token", tok)
	}
}

func TestExchangeForGCP_NoImpersonation(t *testing.T) {
	sts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"federated-token"}`))
	}))
	defer sts.Close()

	tok, err := ExchangeForGCPWith(context.Background(),
		"//iam.googleapis.com/projects/1/pool/provider", "", "oidc-jwt",
		GCPEndpoints{STSURL: sts.URL})
	if err != nil {
		t.Fatalf("ExchangeForGCPWith() error = %v", err)
	}
	if tok != "federated-token" {
		t.Fatalf("token = %q, want federated-token (federated, no impersonation)", tok)
	}
}

func TestExchangeForGCP_Validation(t *testing.T) {
	if _, err := ExchangeForGCP(context.Background(), "", "sa@x", "tok"); err == nil {
		t.Fatal("expected error for missing audience")
	}
	if _, err := ExchangeForGCP(context.Background(), "aud", "sa@x", ""); err == nil {
		t.Fatal("expected error for missing token")
	}
}

func TestBedrockCredentialsFromOIDC_NotInActions(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "false")
	if _, err := BedrockCredentialsFromOIDC(context.Background(), "arn", "us-east-1"); !errors.Is(err, ErrNoOIDC) {
		t.Fatalf("error = %v, want ErrNoOIDC", err)
	}
}

func TestVertexTokenFromOIDC_NotInActions(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "false")
	if _, err := VertexTokenFromOIDC(context.Background(), "aud", "sa@x"); !errors.Is(err, ErrNoOIDC) {
		t.Fatalf("error = %v, want ErrNoOIDC", err)
	}
}
