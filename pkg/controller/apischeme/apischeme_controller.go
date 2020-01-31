package APIScheme

import (
	"context"
	"time"

	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	"github.com/openshift/cloud-ingress-operator/pkg/awsclient"
	"github.com/openshift/cloud-ingress-operator/pkg/config"

	configv1 "github.com/openshift/api"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_APIScheme")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new APIScheme Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileAPIScheme{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("APIScheme-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource APIScheme
	err = c.Watch(&source.Kind{Type: &cloudingressv1alpha1.APIScheme{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileAPIScheme implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileAPIScheme{}

// ReconcileAPIScheme reconciles a APIScheme object
type ReconcileAPIScheme struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile will ensure that the rh-api management api endpoint is created and ready.
// Rough Steps:
// 1. Create AWS ELB (CreatingLoadBalancer)
// 2. Create Security Group with allowed CIDR blocks (UpdatingCIDRAllownaces)
// 3. Add master Node EC2 instances to the load balancer as listeners (6443/TCP) (UpdatingLoadBalancerListeners)
// 4. Update APIServer object to add a record for the rh-api endpoint (UpdatingAPIServer)
// 5. Ready for work (Ready)
func (r *ReconcileAPIScheme) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling APIScheme")

	// TODO: Add controller to observe Machine objects in case the master nodes change (eg updating listeners)

	// Fetch the APIScheme instance
	instance := &cloudingressv1alpha1.APIScheme{}
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

	// We expect this secret to exist in the same namespace Account CR's are created
	// TODO: Get the region of the cluster
	awsClient, err := awsclient.GetAWSClient(r.Client, awsclient.NewAwsClientInput{
		SecretName: config.AWSSecretName,
		NameSpace:  config.OperatorNamespace,
		AwsRegion:  "us-east-1",
	})
	if err != nil {
		reqLogger.Error(err, "Failed to get AWS client")
		return reconcile.Result{}, err
	}
	//	awsClient, err := awsclient.NewClient("access id", "secret", "token", "region")
	if err != nil {
		return reconcile.Result{}, err
	}

	switch instance.Status.State {
	case cloudingressv1alpha1.Pending:
		// if the ELB is created already, go to next step
		if found, err := awsClient.DoesELBExist(config.CloudAdminAPILoadBalancerName); err != nil {
			return reconcile.Result{}, err
		} else if !found {
			//create
			// TODO: Detect the AZs for "master" nodes

			// subnet from config.openshift.io network/cluster object, need to get subnet ID
			// az from machine record for master objects
			// Lisa to ask apiserver folks for an in-cluster representation of subnet id and az
			dnsName, err := awsClient.CreateClassicELB(config.CloudAdminAPILoadBalancerName, []string{"az1", "az2"}, config.AdminAPIListenerPort)
			if err != nil {
				reqLogger.Error(err, "Error while creating Load Balancer", config.CloudAdminAPILoadBalancerName)
				return reconcile.Result{}, err
			}
			reqLogger.Info("DNS Name for ELB from Amazon is %s", dnsName)
			instance.Status.CloudLoadBalancerDNSName = dnsName
			setAPISchemeStatus(reqLogger, instance, "Load Balancer Created", cloudingressv1alpha1.APISchemeCreatedLoadBalancer, "Load Balancer Created")

			return reconcile.Result{}, nil
		} else {
			//found
		}

	case cloudingressv1alpha1.APISchemeCreatedLoadBalancer:
		// if LB is in place update listeners here
		//    awsClient.EnsureCIDRAccess
	case cloudingressv1alpha1.APISchemeUpdatedLoadBalancerListeners:
		// after updating listeners set up SGs allowing CIDRs
	case cloudingressv1alpha1.APISchemeUpdatedCIDRAllowances:
		// update DNS config here
	case cloudingressv1alpha1.APISchemeUpdatedDNS:
		// update API endpoint
	default:
		// idk!
	}

	return reconcile.Result{}, nil
}

func updateCondition(instance *cloudingressv1alpha1.APIScheme, msg, reason string, nextState cloudingressv1alpha1.ManagementState) error {
	instance.Status.State = nextState
	return nil
}

func setAPISchemeStatus(reqLogger logr.Logger, APIScheme *cloudingressv1alpha1.APIScheme, message string, ctype cloudingressv1alpha1.APISchemeConditionType, state string) {
	APIScheme.Status.Conditions = controllerutils.SetAccountCondition(
		APIScheme.Status.Conditions,
		ctype,
		corev1.ConditionTrue,
		state,
		message,
		controllerutils.UpdateConditionNever)
	APIScheme.Status.State = state
}

func ensureCloudLoadBalancer(reqLogger logr.Logger, awsAPI *awsclient.awsClient, endpointName string, elbSubnets []string) (*awsclient.AWSLoadBalancer, error) {
	var awsObj *awsclient.AWSLoadBalancer
	found := false
	for i := 1; i <= config.MaxAPIRetries; i++ {
		found, awsObj, err := c.DoesELBExist(endpointName)
		if err != nil {
			reqLogger.Info("Couldn't determine if the Admin API ELB exists due to error: " + err.Error())
			if i == config.MaxAPIRetries {
				reqLogger.Info("Out of retries")
				return nil, err
			} else {
				reqLogger.Info(fmt.Sprintf("Sleeping %d seconds before retrying...", i))
				time.Sleep(time.Duration(i * time.Second))
			}
		} else {
			// We have a successful response from the API
			break
		}
	}
	if !found {
		reqLogger.Info(fmt.Sprintf("ELB exists for Admin API in DNS zone %s with DNS name %s", awsObj.DNSZoneId, awsObj.DNSName))
	} else {
		reqLogger.Info("Need to create ELB for Admin API")
		for i := 1; i <= config.MaxAPIRetries; i++ {
			awsObj, err = awsAPI.CreateClassicELB(endpointName, elbSubnets, 6443)
			if err != nil {
				fmt.Printf("Error creating ELB: " + err.Error())
				if i == config.MaxAPIRetries {
					reqLogger.Info("Out of retries")
					return nil, err
				} else {
					reqLogger.Info(fmt.Sprintf("Sleeping %d seconds before retrying...", i))
					time.Sleep(time.Duration(i * time.Second))
				}
			} else {
				reqLogger.Info(fmt.Sprintf("Created ELB for Admin API in DNS zone %s with DNS name %s", awsObj.DNSZoneId, awsObj.DNSName))
				break
			}
		}
	}
	return awsObj, nil
}

func ensureCIDRAccess(reqLogger logr.Logger, crObject *cloudingressv1alpha1.APIScheme, awsAPI *awsclient.awsClient, endpointName, securityGroupName, securityGroupPVCName string) error {
	for i := 1; i <= config.MaxAPIRetries; i++ {
		err = awsAPI.EnsureCIDRAccess(endpointName, securityGroupName, securityGroupPVCName, crObject.Spec.AllowedCIDRBlocks)
		if err != nil {
			reqLogger.Info("Error ensuring CIDR access for the security group: " + err.Error())
			if i == config.MaxAPIRetries {
				reqLogger.Info("Out of retries")
				return err
			} else {
				reqLogger.Info(fmt.Sprintf("Sleeping %d seconds before retrying...", i))
				time.Sleep(time.Duration(i * time.Second))
			}
		} else {
			reqLogger.Info("Security Group CIDR access ensured")
			break
		}
	}
	return nil
}

func ensureLoadBalancerInstances(reqLogger logr.Logger, awsAPI *awsclient.awsClient, endpointName string, primaryInstanceIDs []string) error {
	for i := 1; i <= config.MaxAPIRetries; i++ {
		err = awsAPI.AddLoadBalancerInstances(endpointName, primaryInstanceIDs)
		if err != nil {
			reqLogger.Info(fmt.Sprintf("Couldn't add instances %s to ELB %s: %s", primaryInstanceIDs, endpointName, err.Error()))
			if i == config.MaxAPIRetries {
				reqLogger.Info("Out of retries")
				return err
			} else {
				reqLogger.Info(fmt.Sprintf("Sleeping %d seconds before retrying...", i))
				time.Sleep(time.Duration(i * time.Second))
			}
		} else {
			reqLogger.Info(fmt.Sprintf("Added instances %s to ELB", primaryInstanceIDs))
			break
		}
	}
	return nil
}

func ensureDNSRecord(reqLogger logr.Logger, awsAPI *awsclient.awsClient, baseClusterDomain, endpointName string, awsObj *awsclient.AWSLoadBalancer) error {
	for i := 1; i <= config.MaxAPIRetries; i++ {
		err = awsAPI.AddManagementDNSRecord(baseClusterDomain, endpointName, awsObj)
		if err != nil {
			reqLogger.Info("Couldn't upsert a DNS record: " + err.Error())
			if i == config.MaxAPIRetries {
				reqLogger.Info("Out of retries")
				return err
			} else {
				reqLogger.Info(fmt.Sprintf("Sleeping %d seconds before retrying...", i))
				time.Sleep(time.Duration(i * time.Second))
			}
		} else {
			break
		}
	}
	return nil
}

// ensureAdminAPIEndpoint will ensure the Admin API endpoint exists. Returns any error
// This function is idempotent
func ensureAdminAPIEndpoint(reqLogger logr.Logger, crObject *cloudingressv1alpha1.APIScheme, awsAPI *awsclient.awsClient,
	endpointName, securityGroupName, securityGroupPVCName, baseClusterDomain string, elbSubnets, primaryInstanceIDs []string) error {

	// First, let's ensure an ELB exists
	awsObj, err := ensureCloudLoadBalancer(reqLogger, awsAPI, endpointName, elbSubnets)
	if err != nil {
		// This is actually fatal due to the retries in ensureCloudLoadBalancer
		return err
	}
	// Now, ensure CIDR access
	err = ensureCIDRAccess(reqLogger, crObject, awsAPI, endpointName, securityGroupName, securityGroupPVCName)
	if err != nil {
		// This is actually fatal
		return err
	}
	// Add the "master" instances are attached to the ELB
	err = ensureLoadBalancerInstances(reqLogger, awsAPI, endpointName, primaryInstanceIDs)
	if err != nil {
		// This is actually fatal
		return err
	}
	err = ensureDNSRecord(reqLogger, awsAPI, baseClusterDomain, endpointName, awsObj)
	if err != nil {
		// This is actually fatal
		return err
	}
	// Finally, ensure the DNS name is present in the zone
	return nil
}

// addAdminAPIToApiServerObject will add the +domainName+ to the
// ApiServer/cluster object for the admin api endpoint
// Two ways to do this:
// 1. Re-use the existing certificate, but add a new hostname for the apiserver
// to listen on
// 2. Add a new TLS certificate and hostname
// We will use option 1 and trust that the existing TLS cert has an entry for
// +domainName+
// Option 1 will look like this:
//
// apiVersion: config.openshift.io/v1
// kind: APIServer
// metadata:
//   name: cluster
// spec:
//   clientCA:
//     name: ""
//   servingCerts:
//     defaultServingCertificate:
//       name: ""
//     namedCertificates:
//     - names:
//       - api.<cluster-domain>
//       - rh-adpi.<cluster-domain>  <-- Add this
//       servingCertificate:
//         name: <cluster-name-primary-cert-bundle-secret
//
// For completeness, option 2 looks like
//
// apiVersion: config.openshift.io/v1
// kind: APIServer
// metadata:
//   name: cluster
// spec:
//   clientCA:
//     name: ""
//   servingCerts:
//     defaultServingCertificate:
//       name: ""
//     namedCertificates:
//     - names:
//       - api.<cluster-domain>
//       servingCertificate:
//         name: <cluster-name>-primary-cert-bundle-secret
//     - names: <-- Add this
//       - rh-api.<cluster-domain>
//       servingCertificate:
//         name: rh-api-endpoint-cert-bundle-secret <-- openshift-config namespace
func (r *ReconcileApiScheme) addAdminAPIToApiServerObject(logger *logr.Logger, domainName string) error {
	//TODO: Validate this logic and object typing
	api := &configv1.ApiServer{}
	ns := types.NamespacedName{
		Namespace: "",
		Name:      "",
	}
	err := r.client.Get(context.TODO(), ns, api)
	if err != nil {
		return err
	}
	for i, name := range api.Spec.ServingCerts.NamedCertificates.Names {
		if strings.HasPrefix(name, "api.") {
			api.Spec.ServingCerts.NamedCertificates[i].Names = append(api.Spec.ServingCerts.NamedCertificates[i].Names, domainName)
			return r.client.Update(context.TODO(), api)
		}
	}
	return fmt.Errorf("Couldn't find api name for APIServer. Did no work")
}
