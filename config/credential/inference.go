package credential

// CredentialInference is save metadata for a gateway chosen in setup UI (no secret).
type CredentialInference struct {
	ProviderID   string `json:"provider_id"`
	DeploymentID string `json:"deployment_id"`
	EnvVar       string `json:"env_var"`
	DisplayName  string `json:"display_name"`
}
