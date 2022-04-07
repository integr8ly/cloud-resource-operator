/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	// configv1 "github.com/integr8ly/cloud-resource-operator/apis/config/v1"
	// integreatlyv1alpha1 "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	// . "github.com/onsi/ginkgo"
	// . "github.com/onsi/gomega"
	// "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	// "sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	// logf "sigs.k8s.io/controller-runtime/pkg/log"
	// "sigs.k8s.io/controller-runtime/pkg/log/zap"
	// "testing"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var err error

// func TestAPIs(t *testing.T) {
// 	RegisterFailHandler(Fail)

// 	// start test env
// 	By("bootstrapping test environment")

// 	useCluster := true
// 	testEnv = &envtest.Environment{
// 		UseExistingCluster:       &useCluster,
// 		AttachControlPlaneOutput: true,
// 	}

// 	cfg, err = testEnv.Start()
// 	if err != nil {
// 		t.Fatalf("could not get start test environment %s", err)
// 	}

// 	cfg.Impersonate = rest.ImpersonationConfig{
// 		UserName: "system:admin",
// 		Groups:   []string{"system:authenticated"},
// 	}

// 	RunSpecsWithDefaultAndCustomReporters(t,
// 		"E2E Test Suite",
// 		[]Reporter{printer.NewlineReporter{}})
// }

// var _ = BeforeSuite(func(done Done) {

// 	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))

// 	err = integreatlyv1alpha1.AddToScheme(scheme.Scheme)
// 	Expect(err).NotTo(HaveOccurred())

// 	err = configv1.AddToScheme(scheme.Scheme)
// 	Expect(err).NotTo(HaveOccurred())

// 	// +kubebuilder:scaffold:scheme
// 	close(done)
// }, 60)

// var _ = AfterSuite(func() {
// 	By("tearing down the test environment")

// 	err := testEnv.Stop()
// 	Expect(err).ToNot(HaveOccurred())
// })
