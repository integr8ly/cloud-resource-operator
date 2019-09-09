package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SecretRef This represents a namespace-scoped Secret
type SecretRef struct {
	Name string `json:"name,omitempty"`
}

// BlobStorageSpec defines the desired state of BlobStorage
// +k8s:openapi-gen=true
type BlobStorageSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	Type      string    `json:"type"`
	Tier      string    `json:"tier"`
	SecretRef SecretRef `json:"secretRef"`
}

// BlobStorageStatus defines the observed state of BlobStorage
// +k8s:openapi-gen=true
type BlobStorageStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	Strategy  string    `json:"strategy,omitempty"`
	Provider  string    `json:"provider,omitempty"`
	SecretRef SecretRef `json:"secretRef,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BlobStorage is the Schema for the blobstorages API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type BlobStorage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BlobStorageSpec   `json:"spec,omitempty"`
	Status BlobStorageStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BlobStorageList contains a list of BlobStorage
type BlobStorageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BlobStorage `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BlobStorage{}, &BlobStorageList{})
}
