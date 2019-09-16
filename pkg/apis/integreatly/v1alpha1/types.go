package v1alpha1

// SecretRef Represents a namespace-scoped Secret
type SecretRef struct {
	Name string `json:"name,omitempty"`
}

// ResourceTypeSpec Represents the basic information required to provision a resource type
type ResourceTypeSpec struct {
	Type      string     `json:"type"`
	Tier      string     `json:"tier"`
	SecretRef *SecretRef `json:"secretRef"`
}

// ResourceTypeStatus Represents the basic status information provided by a resource provider
type ResourceTypeStatus struct {
	Strategy  string     `json:"strategy,omitempty"`
	Provider  string     `json:"provider,omitempty"`
	SecretRef *SecretRef `json:"secretRef,omitempty"`
}
