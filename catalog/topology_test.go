package catalog

import "testing"

func TestPruneUnreferencedDeployments(t *testing.T) {
	t.Parallel()

	c := SeedCatalog()
	c.Deployments["stale-orphan"] = Deployment{
		ID:                  "stale-orphan",
		Name:                "Stale orphan",
		ProviderID:          "concentrate",
		APIProtocolID:       "openai-chat-completions",
		AdapterConstructor:  "concentrate",
		NativeModelIDSource: NativeModelIDDiscovered,
	}
	c.Deployments["external-referenced"] = Deployment{
		ID:                  "external-referenced",
		Name:                "External referenced",
		ProviderID:          "custom-concentrate",
		APIProtocolID:       "openai-chat-completions",
		AdapterConstructor:  "concentrate",
		NativeModelIDSource: NativeModelIDDiscovered,
	}
	c.Providers["custom-concentrate"] = Provider{ID: "custom-concentrate", Name: "Custom Concentrate"}
	c.Offerings = append(c.Offerings, ModelOffering{
		ID:               "external-referenced:native-model",
		CanonicalModelID: "custom-concentrate/native-model",
		DeploymentID:     "external-referenced",
		NativeModelID:    "native-model",
	})
	c.Models["custom-concentrate/native-model"] = Model{
		ID:         "custom-concentrate/native-model",
		ProviderID: "custom-concentrate",
		Name:       "Native model",
	}
	c.Deployments["obsolete-first-class"] = Deployment{
		ID:                  "obsolete-first-class",
		Name:                "Obsolete first-class deployment",
		ProviderID:          "concentrate",
		APIProtocolID:       "openai-chat-completions",
		AdapterConstructor:  "concentrate",
		NativeModelIDSource: NativeModelIDDiscovered,
	}
	c.Offerings = append(c.Offerings, ModelOffering{
		ID:               "obsolete-first-class:old-model",
		CanonicalModelID: "concentrate/old-model",
		DeploymentID:     "obsolete-first-class",
		NativeModelID:    "old-model",
	})
	c.Models["concentrate/old-model"] = Model{
		ID:         "concentrate/old-model",
		ProviderID: "concentrate",
		Name:       "Old model",
	}

	PruneUnreferencedDeployments(&c)

	if _, exists := c.Deployments["stale-orphan"]; exists {
		t.Fatal("unregistered orphan deployment was not pruned")
	}
	if _, exists := c.Deployments["external-referenced"]; !exists {
		t.Fatal("referenced external deployment was pruned")
	}
	if _, exists := c.Deployments["concentrate-payg"]; !exists {
		t.Fatal("registered deployment was pruned")
	}
	if _, exists := c.Deployments["obsolete-first-class"]; exists {
		t.Fatal("obsolete first-class deployment was not pruned")
	}
	for _, offering := range c.Offerings {
		if offering.DeploymentID == "obsolete-first-class" {
			t.Fatal("offering for obsolete first-class deployment was not pruned")
		}
	}
}
