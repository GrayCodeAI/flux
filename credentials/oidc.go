package credentials

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// OIDC keyless authentication for cloud Anthropic deployments.
//
// In GitHub Actions a workflow with `permissions: id-token: write` can mint a
// short-lived OIDC token and exchange it for cloud credentials without storing
// any long-lived secrets:
//
//   - AWS Bedrock: STS AssumeRoleWithWebIdentity -> temporary AWS credentials.
//   - GCP Vertex: STS token exchange (Workload Identity Federation) then
//     iamcredentials generateAccessToken to impersonate a service account.
//
// Everything is implemented with net/http + stdlib XML/JSON — no cloud SDKs.
// Network endpoints are injectable via the *Endpoints structs so tests can
// point them at httptest servers.

// audience constants for the OIDC token request.
const (
	// AWSAudience is the OIDC audience AWS STS expects for web-identity federation.
	AWSAudience = "sts.amazonaws.com"
)

// Default cloud endpoints. Overridable for testing.
const (
	defaultSTSAWSURL            = "https://sts.amazonaws.com/"
	defaultSTSGCPURL            = "https://sts.googleapis.com/v1/token"
	defaultIAMCredentialsHost   = "https://iamcredentials.googleapis.com" // #nosec G101 -- public STS endpoint URL, not a secret value
	awsAssumeRoleWebIdentityVer = "2011-06-15"
)

// DetectGitHubActions reports whether the process is running inside a GitHub
// Actions runner.
func DetectGitHubActions() bool {
	return os.Getenv("GITHUB_ACTIONS") == "true"
}

// httpClientOrDefault returns the supplied client or http.DefaultClient.
func httpClientOrDefault(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return http.DefaultClient
}

// FetchGitHubOIDCToken requests an OIDC ID token from the GitHub Actions token
// service for the given audience. It reads ACTIONS_ID_TOKEN_REQUEST_URL and
// ACTIONS_ID_TOKEN_REQUEST_TOKEN from the environment (set by the runner when
// the workflow has `id-token: write` permission).
func FetchGitHubOIDCToken(ctx context.Context, audience string) (string, error) {
	return fetchGitHubOIDCToken(ctx, audience, nil)
}

// fetchGitHubOIDCToken is the injectable core of FetchGitHubOIDCToken.
func fetchGitHubOIDCToken(ctx context.Context, audience string, hc *http.Client) (string, error) {
	requestURL := strings.TrimSpace(os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL"))
	requestToken := strings.TrimSpace(os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN"))
	if requestURL == "" || requestToken == "" {
		return "", fmt.Errorf("credentials: GitHub OIDC unavailable (missing ACTIONS_ID_TOKEN_REQUEST_URL/TOKEN; set workflow permissions: id-token: write)")
	}

	u, err := url.Parse(requestURL)
	if err != nil {
		return "", fmt.Errorf("credentials: invalid OIDC request url: %w", err)
	}
	if audience != "" {
		q := u.Query()
		q.Set("audience", audience)
		u.RawQuery = q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("credentials: OIDC request creation failed: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+requestToken)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClientOrDefault(hc).Do(req)
	if err != nil {
		return "", fmt.Errorf("credentials: OIDC token request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("credentials: OIDC token request error (status %d): %s", resp.StatusCode, readErrBody(resp.Body))
	}

	var parsed struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("credentials: OIDC token decode failed: %w", err)
	}
	if strings.TrimSpace(parsed.Value) == "" {
		return "", fmt.Errorf("credentials: OIDC token response had empty value")
	}
	return parsed.Value, nil
}

// AWSEndpoints holds the network configuration for AWS STS exchange.
// The zero value uses production AWS STS.
type AWSEndpoints struct {
	// STSURL overrides the STS endpoint (default https://sts.amazonaws.com/).
	STSURL string
	// HTTPClient overrides the HTTP client used for the request.
	HTTPClient *http.Client
}

// stsAssumeRoleResponse mirrors the XML returned by AssumeRoleWithWebIdentity.
type stsAssumeRoleResponse struct {
	XMLName xml.Name `xml:"AssumeRoleWithWebIdentityResponse"`
	Result  struct {
		Credentials struct {
			AccessKeyID     string `xml:"AccessKeyId"`
			SecretAccessKey string `xml:"SecretAccessKey"`
			SessionToken    string `xml:"SessionToken"`
			Expiration      string `xml:"Expiration"`
		} `xml:"Credentials"`
	} `xml:"AssumeRoleWithWebIdentityResult"`
}

// ExchangeForAWS exchanges a GitHub OIDC token for temporary AWS credentials
// via STS AssumeRoleWithWebIdentity. The returned credentials are short-lived
// and suitable for NewBedrockClient.
func ExchangeForAWS(ctx context.Context, roleARN, region, oidcToken string) (accessKeyID, secretKey, sessionToken string, err error) {
	return ExchangeForAWSWith(ctx, roleARN, region, oidcToken, AWSEndpoints{})
}

// ExchangeForAWSWith is ExchangeForAWS with injectable endpoints for testing.
func ExchangeForAWSWith(ctx context.Context, roleARN, region, oidcToken string, ep AWSEndpoints) (accessKeyID, secretKey, sessionToken string, err error) {
	roleARN = strings.TrimSpace(roleARN)
	oidcToken = strings.TrimSpace(oidcToken)
	if roleARN == "" {
		return "", "", "", fmt.Errorf("credentials: role ARN required for AWS OIDC exchange")
	}
	if oidcToken == "" {
		return "", "", "", fmt.Errorf("credentials: OIDC token required for AWS exchange")
	}

	endpoint := ep.STSURL
	if endpoint == "" {
		endpoint = defaultSTSAWSURL
	}

	form := url.Values{}
	form.Set("Action", "AssumeRoleWithWebIdentity")
	form.Set("Version", awsAssumeRoleWebIdentityVer)
	form.Set("RoleArn", roleARN)
	form.Set("RoleSessionName", "graycode-router-github-oidc")
	form.Set("WebIdentityToken", oidcToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", "", fmt.Errorf("credentials: STS request creation failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/xml")
	if region != "" {
		// STS is global but AWS accepts a regional hint header on some setups.
		req.Header.Set("X-Amz-Region", region)
	}

	resp, err := httpClientOrDefault(ep.HTTPClient).Do(req)
	if err != nil {
		return "", "", "", fmt.Errorf("credentials: STS exchange failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("credentials: STS exchange error (status %d): %s", resp.StatusCode, readErrBody(resp.Body))
	}

	var parsed stsAssumeRoleResponse
	if err := xml.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", "", "", fmt.Errorf("credentials: STS response decode failed: %w", err)
	}
	creds := parsed.Result.Credentials
	if creds.AccessKeyID == "" || creds.SecretAccessKey == "" {
		return "", "", "", fmt.Errorf("credentials: STS response missing credentials")
	}
	return creds.AccessKeyID, creds.SecretAccessKey, creds.SessionToken, nil
}

// GCPEndpoints holds the network configuration for GCP STS + impersonation.
// The zero value uses production GCP endpoints.
type GCPEndpoints struct {
	// STSURL overrides the STS token-exchange endpoint.
	STSURL string
	// IAMCredentialsHost overrides the iamcredentials host (no trailing slash).
	IAMCredentialsHost string
	// HTTPClient overrides the HTTP client used for the requests.
	HTTPClient *http.Client
}

// ExchangeForGCP exchanges a GitHub OIDC token for a Google access token via
// Workload Identity Federation. It first exchanges the OIDC token for a
// federated access token at the STS endpoint, then impersonates the named
// service account through iamcredentials generateAccessToken. The returned
// token is suitable for NewVertexClient.
//
// audience is the full WIF audience, e.g.
// "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/POOL/providers/PROVIDER".
func ExchangeForGCP(ctx context.Context, audience, serviceAccountEmail, oidcToken string) (accessToken string, err error) {
	return ExchangeForGCPWith(ctx, audience, serviceAccountEmail, oidcToken, GCPEndpoints{})
}

// ExchangeForGCPWith is ExchangeForGCP with injectable endpoints for testing.
func ExchangeForGCPWith(ctx context.Context, audience, serviceAccountEmail, oidcToken string, ep GCPEndpoints) (accessToken string, err error) {
	audience = strings.TrimSpace(audience)
	serviceAccountEmail = strings.TrimSpace(serviceAccountEmail)
	oidcToken = strings.TrimSpace(oidcToken)
	if audience == "" {
		return "", fmt.Errorf("credentials: audience required for GCP OIDC exchange")
	}
	if oidcToken == "" {
		return "", fmt.Errorf("credentials: OIDC token required for GCP exchange")
	}

	federated, err := gcpSTSExchange(ctx, audience, oidcToken, ep)
	if err != nil {
		return "", err
	}
	if serviceAccountEmail == "" {
		// No impersonation requested — the federated token is the result.
		return federated, nil
	}
	return gcpImpersonate(ctx, serviceAccountEmail, federated, ep)
}

// gcpSTSExchange performs the WIF token exchange and returns a federated token.
func gcpSTSExchange(ctx context.Context, audience, oidcToken string, ep GCPEndpoints) (string, error) {
	endpoint := ep.STSURL
	if endpoint == "" {
		endpoint = defaultSTSGCPURL
	}

	reqBody := map[string]string{
		"grantType":          "urn:ietf:params:oauth:grant-type:token-exchange",
		"audience":           audience,
		"scope":              "https://www.googleapis.com/auth/cloud-platform",
		"requestedTokenType": "urn:ietf:params:oauth:token-type:access_token",
		"subjectTokenType":   "urn:ietf:params:oauth:token-type:jwt",
		"subjectToken":       oidcToken,
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("credentials: GCP STS request encode failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return "", fmt.Errorf("credentials: GCP STS request creation failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClientOrDefault(ep.HTTPClient).Do(req)
	if err != nil {
		return "", fmt.Errorf("credentials: GCP STS exchange failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("credentials: GCP STS exchange error (status %d): %s", resp.StatusCode, readErrBody(resp.Body))
	}

	var parsed struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("credentials: GCP STS response decode failed: %w", err)
	}
	if strings.TrimSpace(parsed.AccessToken) == "" {
		return "", fmt.Errorf("credentials: GCP STS response missing access_token")
	}
	return parsed.AccessToken, nil
}

// gcpImpersonate calls iamcredentials generateAccessToken to impersonate the
// service account using the federated token.
func gcpImpersonate(ctx context.Context, serviceAccountEmail, federatedToken string, ep GCPEndpoints) (string, error) {
	host := ep.IAMCredentialsHost
	if host == "" {
		host = defaultIAMCredentialsHost
	}
	endpoint := fmt.Sprintf("%s/v1/projects/-/serviceAccounts/%s:generateAccessToken",
		strings.TrimRight(host, "/"), url.PathEscape(serviceAccountEmail))

	reqBody := map[string]any{
		"scope": []string{"https://www.googleapis.com/auth/cloud-platform"},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("credentials: GCP impersonation request encode failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return "", fmt.Errorf("credentials: GCP impersonation request creation failed: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+federatedToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClientOrDefault(ep.HTTPClient).Do(req)
	if err != nil {
		return "", fmt.Errorf("credentials: GCP impersonation failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("credentials: GCP impersonation error (status %d): %s", resp.StatusCode, readErrBody(resp.Body))
	}

	var parsed struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("credentials: GCP impersonation response decode failed: %w", err)
	}
	if strings.TrimSpace(parsed.AccessToken) == "" {
		return "", fmt.Errorf("credentials: GCP impersonation response missing accessToken")
	}
	return parsed.AccessToken, nil
}

// AWSCredentials holds temporary AWS credentials from an OIDC exchange.
type AWSCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Region          string
}

// BedrockCredentialsFromOIDC is a convenience helper for setup/deployment.go:
// it detects GitHub Actions, fetches an OIDC token for the AWS audience, and
// exchanges it for temporary Bedrock credentials. It returns ErrNoOIDC when not
// running inside GitHub Actions so callers can fall back to their happy path.
func BedrockCredentialsFromOIDC(ctx context.Context, roleARN, region string) (AWSCredentials, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !DetectGitHubActions() {
		return AWSCredentials{}, ErrNoOIDC
	}
	token, err := FetchGitHubOIDCToken(ctx, AWSAudience)
	if err != nil {
		return AWSCredentials{}, err
	}
	ak, sk, st, err := ExchangeForAWS(ctx, roleARN, region, token)
	if err != nil {
		return AWSCredentials{}, err
	}
	return AWSCredentials{
		AccessKeyID:     ak,
		SecretAccessKey: sk,
		SessionToken:    st,
		Region:          region,
	}, nil
}

// VertexTokenFromOIDC is a convenience helper for setup/deployment.go: it
// detects GitHub Actions, fetches an OIDC token for the given WIF audience, and
// exchanges it for a Google access token usable by NewVertexClient. It returns
// ErrNoOIDC when not running inside GitHub Actions.
func VertexTokenFromOIDC(ctx context.Context, audience, serviceAccountEmail string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !DetectGitHubActions() {
		return "", ErrNoOIDC
	}
	token, err := FetchGitHubOIDCToken(ctx, audience)
	if err != nil {
		return "", err
	}
	return ExchangeForGCP(ctx, audience, serviceAccountEmail, token)
}

// readErrBody reads a capped error body for diagnostic messages.
func readErrBody(r io.Reader) string {
	const maxErr = 2048
	b, _ := io.ReadAll(io.LimitReader(r, maxErr))
	return strings.TrimSpace(string(b))
}
