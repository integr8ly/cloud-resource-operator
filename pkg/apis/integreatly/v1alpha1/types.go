package v1alpha1

// SecretRef This represents a namespace-scoped Secret
type SecretRef struct {
	Name string `json:"name,omitempty"`
}

type ResourceTypeSpec struct {
	Type      string    `json:"type"`
	Tier      string    `json:"tier"`
	SecretRef SecretRef `json:"secretRef"`
}

type ResourceTypeStatus struct {
	Strategy  string    `json:"strategy,omitempty"`
	Provider  string    `json:"provider,omitempty"`
	SecretRef SecretRef `json:"secretRef"`
}
