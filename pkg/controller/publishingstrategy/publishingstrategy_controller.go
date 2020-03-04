package publishingstrategy

import (
	"context"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	"github.com/openshift/cloud-ingress-operator/pkg/awsclient"
	"github.com/openshift/cloud-ingress-operator/pkg/config"
	utils "github.com/openshift/cloud-ingress-operator/pkg/controller/utils"

	// machineapiv1 "sigs.k8s.io/cluster-api/pkg/apis/deprecated/v1alpha1"

	// "github.com/aws/aws-sdk-go/aws"

	// corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_publishingstrategy")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new PublishingStrategy Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcilePublishingStrategy{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("publishingstrategy-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource PublishingStrategy
	err = c.Watch(&source.Kind{Type: &cloudingressv1alpha1.PublishingStrategy{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcilePublishingStrategy implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcilePublishingStrategy{}

// ReconcilePublishingStrategy reconciles a PublishingStrategy object
type ReconcilePublishingStrategy struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a PublishingStrategy object and makes changes based on the state read
// and what is in the PublishingStrategy.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcilePublishingStrategy) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling PublishingStrategy")

	// Fetch the PublishingStrategy instance
	instance := &cloudingressv1alpha1.PublishingStrategy{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
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

	// Fetch the machineapi instance
	// machineapi := machineapiv1.Machine{}
	// err = r.client.Get(context.TODO(), request.NamespacedName, machineapi)
	// if err != nil {
	// 	if errors.IsNotFound(err) {
	// 		return reconcile.Result{}, nil
	// 	}
	// 	return reconcile.Result{}, err
	// }

	// machineapi.Spec.ProviderSpec

	// Reconcile will handle
	// 1. Set the cluster API to Internal
	// 2. Set the cluster API to External (Internet-facing)
	// 3. Set the default ingress (application) to Internal
	// 4. Set the default ingress (application) to External (Internet-facing)

	// get region
	region, err := utils.GetClusterRegion(r.client)
	if err != nil {
		return reconcile.Result{}, err
	}
	// Secret should exist in the same namespace Account CR's are created
	awsClient, err := awsclient.GetAWSClient(r.client, awsclient.NewAwsClientInput{
		SecretName: config.AWSSecretName,
		NameSpace:  config.OperatorNamespace,
		AwsRegion:  region,
	})
	if err != nil {
		reqLogger.Error(err, "Failed to get AWS client")
		return reconcile.Result{}, err
	}

	// if CR is wanted the default API server to be internal-facing only, we
	// delete the external NLB for port 6443/TCP and change api.<cluster-domain> DNS record to point to internal NLB
	if instance.Spec.DefaultAPIServerIngress.Listening == cloudingressv1alpha1.Internal {
		// loadbalancerInfo returns list of all non-classic ELBs
		loadBalancerInfo, err := awsClient.ListAllNLBs()
		if err != nil {
			log.Error(err, "Error listing all NLBs")
			return reconcile.Result{}, err
		}

		var intDNSName string
		var intHostedZoneID string
		// delete the external NLB
		for _, loadBalancer := range loadBalancerInfo {
			if loadBalancer.Scheme == "internet-facing" {
				err := awsClient.DeleteExternalLoadBalancer(loadBalancer.LoadBalancerArn)
				if err != nil {
					log.Error(err, "error deleting external LB")
				}
			}
			// get internal dnsName and HostID for UpsertCNAME func
			// when we refactor multi-cloud we can figure out what aws lb arn looks like
			// and construct it from the machine object
			if loadBalancer.Scheme == "internal" {
				intDNSName = loadBalancer.DNSName
				intHostedZoneID = loadBalancer.CanonicalHostedZoneNameID
			}
		}

		// change Alias of resource record set of external LB in public hosted zone to internal LB
		domainName := configv1.DNSSpec{}.BaseDomain // in form of ```samn-test.j5u3.s1.devshift.org```

		// In order to update DNS we need the route53 public zone name
		// which happens to be the domainName minus the name of the cluster
		// Since there are NO object on cluster with just clusterName,
		// we will index the first period and parse right
		pubDomainName := domainName[strings.Index(domainName, ".")+1 : len(domainName)] // pubDomainName in form of ```j5u3.s1.devshift.org```
		apiDNSName := "api." + domainName + "."
		comment := "Update api.<clusterName> alias to internal NLB"

		// upsert resource record to change api.<clusterName> from external NLB to internal NLB
		err = awsClient.UpsertCNAME(pubDomainName, intDNSName, intHostedZoneID, apiDNSName, comment, false)
		if err != nil {
			log.Error(err, "Error updating api.<clusterName> alias to internal NLB")
		}
		return reconcile.Result{}, nil
	}

	// if CR is wanted the default server API to be internet-facing, we
	// create the external NLB for port 6443/TCP and add api.<cluster-name> DNS record to point to external NLB
	if instance.Spec.DefaultAPIServerIngress.Listening == cloudingressv1alpha1.External {
		// get a list of all non-classic ELBs
		loadBalancerInfo, err := awsClient.ListAllNLBs()
		if err != nil {
			log.Error(err, "error listing all NLBs")
			return reconcile.Result{}, err
		}

		// check if external NLB exists
		// if it does no action needed
		for _, loadBalancer := range loadBalancerInfo {
			if loadBalancer.Scheme == "internet-facing" {
				log.Info("External LoadBalancer already exists")
				return reconcile.Result{}, nil
			}
		}

		// create a new external NLB

	}

	return reconcile.Result{}, nil
}
