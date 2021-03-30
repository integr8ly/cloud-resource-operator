package apis

import (
	v1 "github.com/integr8ly/cloud-resource-operator/apis/config/v1"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	creds "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
)

func init() {
	// Register the types with the Scheme so the components can map objects to GroupVersionKinds and back
	AddToSchemes = append(
		AddToSchemes,
		v1alpha1.SchemeBuilder.AddToScheme,
		v1.SchemeBuilder.AddToScheme,
		creds.AddToScheme)
}
