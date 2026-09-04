package runtime

import (
	"context"

	"github.com/GrayCodeAI/graycode-router/setup"
)

// DeploymentStatus returns deployment-routing diagnostics for host UIs.
func DeploymentStatus(ctx context.Context, activeModel string) (setup.StatusReport, error) {
	return setup.DeploymentStatus(ctx, activeModel)
}

// FormatDeploymentStatus renders a deployment-routing diagnostics report for host UIs.
func FormatDeploymentStatus(report setup.StatusReport) string {
	return setup.FormatStatus(report)
}

// RoutingPreview returns the effective routing preview JSON for a model ID.
func RoutingPreview(ctx context.Context, model string) (string, error) {
	return setup.RoutingPreview(ctx, model)
}
