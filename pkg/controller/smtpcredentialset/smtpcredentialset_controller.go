package smtpcredentialset

import (
	"context"
	"time"

	v1 "k8s.io/api/core/v1"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers/aws"
	"github.com/sirupsen/logrus"

	integreatlyv1alpha1 "github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	errorUtil "github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_smtpcredentialset")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new SMTPCredentials Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	logger := logrus.WithFields(logrus.Fields{"controller": "controller_blobstorage"})
	return &ReconcileSMTPCredentialSet{client: mgr.GetClient(), scheme: mgr.GetScheme(), logger: logger}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("smtpcredentialset-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource SMTPCredentials
	err = c.Watch(&source.Kind{Type: &integreatlyv1alpha1.SMTPCredentialSet{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileSMTPCredentials implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileSMTPCredentialSet{}

// ReconcileSMTPCredentials reconciles a SMTPCredentials object
type ReconcileSMTPCredentialSet struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
	logger *logrus.Entry
}

// Reconcile reads that state of the cluster for a SMTPCredentials object and makes changes based on the state read
// and what is in the SMTPCredentials.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileSMTPCredentialSet) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	ctx := context.TODO()
	providerList := []providers.SMTPCredentialsProvider{aws.NewAWSSMTPCredentialProvider(r.client, r.logger)}
	cfgMgr := providers.NewConfigManager(providers.DefaultProviderConfigMapName, providers.DefaultConfigNamespace, r.client)

	return r.reconcile(ctx, request, providerList, cfgMgr)
}

func (r *ReconcileSMTPCredentialSet) reconcile(ctx context.Context, request reconcile.Request, providerList []providers.SMTPCredentialsProvider, cfgMgr providers.ConfigManager) (reconcile.Result, error) {
	r.logger.Info("Reconciling SMTPCredentials")

	// Fetch the SMTPCredentials instance
	instance := &integreatlyv1alpha1.SMTPCredentialSet{}
	err := r.client.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	stratMap, err := cfgMgr.GetStrategyMappingForDeploymentType(ctx, instance.Spec.Type)
	if err != nil {
		return reconcile.Result{}, errorUtil.Wrapf(err, "failed to read deployment type config for deployment %s", instance.Spec.Type)
	}
	r.logger.Infof("checking for provider for deployment strategy %s", stratMap.SMTPCredentials)
	for _, p := range providerList {
		if !p.SupportsStrategy(stratMap.SMTPCredentials) {
			r.logger.Debugf("provider %s does not support deployment strategy %s, skipping", p.GetName(), stratMap.SMTPCredentials)
			continue
		}
		if instance.GetDeletionTimestamp() != nil {
			r.logger.Infof("running deletion handler on smtp credential instance %s", instance.Name)
			if err = p.DeleteSMTPCredentials(ctx, instance); err != nil {
				return reconcile.Result{}, errorUtil.Wrapf(err, "failed to run delete handler for smtp credentials instance %s", instance.Name)
			}
			r.logger.Infof("deletion handler for smtp credential instance %s successful, ending reconciliation", instance.Name)
			return reconcile.Result{}, nil
		}
		smtpCredentialSetInst, err := p.CreateSMTPCredentials(ctx, instance)
		if err != nil {
			return reconcile.Result{}, errorUtil.Wrapf(err, "failed to create smtp credential set for instance %s", instance.Name)
		}

		sec := &v1.Secret{
			ObjectMeta: controllerruntime.ObjectMeta{
				Name:      instance.Spec.SecretRef.Name,
				Namespace: instance.Namespace,
			},
		}
		_, err = controllerruntime.CreateOrUpdate(ctx, r.client, sec, func(existing runtime.Object) error {
			e := existing.(*v1.Secret)
			if err = controllerutil.SetControllerReference(instance, e, r.scheme); err != nil {
				return errorUtil.Wrapf(err, "failed to set owner on secret %s", sec.Name)
			}
			e.Data = smtpCredentialSetInst.DeploymentDetails.Data()
			e.Type = v1.SecretTypeOpaque
			return nil
		})
		if err != nil {
			return reconcile.Result{}, errorUtil.Wrapf(err, "failed to reconcile blob storage instance secret %s", sec.Name)
		}
		instance.Status.SecretRef = instance.Spec.SecretRef
		instance.Status.Strategy = stratMap.BlobStorage
		instance.Status.Provider = p.GetName()
		if err = r.client.Status().Update(ctx, instance); err != nil {
			return reconcile.Result{}, errorUtil.Wrapf(err, "failed to update instance %s in namespace %s", instance.Name, instance.Namespace)
		}
		return reconcile.Result{Requeue: true, RequeueAfter: time.Second * 30}, nil
	}

	return reconcile.Result{Requeue: true}, nil
}
