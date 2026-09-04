package catalog

import "github.com/GrayCodeAI/graycode-router/catalog/registry"

// PruneUnreferencedDeployments makes the embedded catalog and provider registry
// authoritative for first-class providers, then removes unrelated orphan
// deployments. Referenced deployments for custom providers remain valid.
func PruneUnreferencedDeployments(c *Catalog) {
	if c == nil || len(c.Deployments) == 0 {
		return
	}

	keep := make(map[string]struct{}, len(c.Deployments))
	knownProviderDeployments := map[string]map[string]struct{}{}
	defaults := defaultDeployments()
	for id, deployment := range defaults {
		keep[id] = struct{}{}
		providerID := CanonicalProviderID(deployment.ProviderID)
		if knownProviderDeployments[providerID] == nil {
			knownProviderDeployments[providerID] = map[string]struct{}{}
		}
		knownProviderDeployments[providerID][id] = struct{}{}
	}
	for _, spec := range registry.DefaultRegistry.All() {
		keep[spec.DeploymentID] = struct{}{}
		providerID := CanonicalProviderID(spec.ProviderID)
		if deployment, ok := defaults[spec.DeploymentID]; ok {
			providerID = CanonicalProviderID(deployment.ProviderID)
		}
		if knownProviderDeployments[providerID] == nil {
			knownProviderDeployments[providerID] = map[string]struct{}{}
		}
		knownProviderDeployments[providerID][spec.DeploymentID] = struct{}{}
	}
	for _, offering := range c.Offerings {
		keep[offering.DeploymentID] = struct{}{}
	}
	for _, template := range c.OfferingTemplates {
		keep[template.DeploymentID] = struct{}{}
	}

	remove := map[string]struct{}{}
	for id, deployment := range c.Deployments {
		providerID := CanonicalProviderID(deployment.ProviderID)
		if allowed := knownProviderDeployments[providerID]; len(allowed) > 0 {
			if _, canonical := allowed[id]; !canonical {
				remove[id] = struct{}{}
				continue
			}
		}
		if _, retained := keep[id]; !retained {
			remove[id] = struct{}{}
		}
	}
	for id := range remove {
		delete(c.Deployments, id)
	}
	if len(remove) == 0 {
		return
	}
	offerings := c.Offerings[:0]
	for _, offering := range c.Offerings {
		if _, removed := remove[offering.DeploymentID]; !removed {
			offerings = append(offerings, offering)
		}
	}
	c.Offerings = offerings
	templates := c.OfferingTemplates[:0]
	for _, template := range c.OfferingTemplates {
		if _, removed := remove[template.DeploymentID]; !removed {
			templates = append(templates, template)
		}
	}
	c.OfferingTemplates = templates
}
