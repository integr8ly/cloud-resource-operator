package apis

import (
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	configv1 "github.com/openshift/api/config/v1"
	creds "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
)

func init() {
	// Register the types with the Scheme so the components can map objects to GroupVersionKinds and back
	AddToSchemes = append(
		AddToSchemes,
		v1alpha1.SchemeBuilder.AddToScheme,
		configv1.Install,
		creds.AddToScheme)
}
