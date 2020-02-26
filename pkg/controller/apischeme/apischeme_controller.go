package apischeme

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	"github.com/openshift/cloud-ingress-operator/pkg/awsclient"
	"github.com/openshift/cloud-ingress-operator/pkg/config"

	utils "github.com/openshift/cloud-ingress-operator/pkg/controller/utils"

	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"

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

var log = logf.Log.WithName("controller_apischeme")

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
	c, err := controller.New("apischeme-controller", mgr, controller.Options{Reconciler: r})
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

// LoadBalancer contains the relevant information to create a Load Balancer
// TODO: Move this into pkg/client
type LoadBalancer struct {
	EndpointName     string            // FQDN of what it should be called
	BaseDomain       string            // What is the base domain (DNS zone) for the EndpointName record?
	Subnets          []string          // On which subnets it should operate (provider-native IDs)
	Tags             map[string]string // Map of tags to apply to the Load Balancer
	MachineInstances []string          // What are the cloud provider instance IDs that should receive traffic?
}

// CIDRAccess represents the information needed to ensure proper access to the named LoadBalancer
// TODO: Move this into pkg/client
type CIDRAccess struct {
	LoadBalancer         *LoadBalancer     // For what LoadBalancer does these CIDR restrictions apply?
	SecurityGroupName    string            // What should the Security Group be called?
	SecurityGroupVPCName string            // To what VPC should the Security Group be attached?
	Tags                 map[string]string // Tags to apply to the Security Group and (non-LoadBalancer) related assets?
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

	ownerTags, err := utils.AWSOwnerTag(r.client)
	if err != nil {
		reqLogger.Error(err, "Couldn't get the cluster owner tags")
		SetAPISchemeStatus(reqLogger, instance, "Couldn't reconcile", "Couldn't get the cluster owner tags", cloudingressv1alpha1.ConditionError)
		r.client.Status().Update(context.TODO(), instance)
		return reconcile.Result{}, err
	}

	region, err := utils.GetClusterRegion(r.client)
	if err != nil {
		reqLogger.Error(err, "Couldn't get the cluster owner tags")
		SetAPISchemeStatus(reqLogger, instance, "Couldn't reconcile", "Couldn't get the cluster's AWS region", cloudingressv1alpha1.ConditionError)
		r.client.Status().Update(context.TODO(), instance)
		return reconcile.Result{}, err
	}
	// We expect this secret to exist in the same namespace Account CR's are created
	// TODO: Get the region of the cluster
	awsClient, err := awsclient.GetAWSClient(r.client, awsclient.NewAwsClientInput{
		SecretName: config.AWSSecretName,
		NameSpace:  config.OperatorNamespace,
		AwsRegion:  region,
	})
	if err != nil {
		reqLogger.Error(err, "Failed to get AWS client")
		SetAPISchemeStatus(reqLogger, instance, "Couldn't reconcile", "Couldn't create an AWS client", cloudingressv1alpha1.ConditionError)
		r.client.Status().Update(context.TODO(), instance)
		return reconcile.Result{}, err
	}
	subnets, err := utils.GetMasterNodeSubnets(r.client)
	if err != nil {
		reqLogger.Error(err, "Couldn't get the subnets used by master nodes")
		SetAPISchemeStatus(reqLogger, instance, "Couldn't reconcile", "Couldn't get the cluster's subnets", cloudingressv1alpha1.ConditionError)
		r.client.Status().Update(context.TODO(), instance)
		return reconcile.Result{}, err
	}

	clusterBaseDomain, err := utils.GetClusterBaseDomain(r.client)
	if err != nil {
		reqLogger.Error(err, "Couldn't obtain the cluster's base domain")
		SetAPISchemeStatus(reqLogger, instance, "Couldn't reconcile", "Couldn't get the cluster's base domain", cloudingressv1alpha1.ConditionError)
		r.client.Status().Update(context.TODO(), instance)
		return reconcile.Result{}, err
	}
	masterNodeInstances, err := utils.GetClusterMasterInstances(r.client)
	if err != nil {
		reqLogger.Error(err, "Couldn't detect the AWS instances for master nodes")
		SetAPISchemeStatus(reqLogger, instance, "Couldn't reconcile", "Couldn't find the cluster's AWS instances for master nodes", cloudingressv1alpha1.ConditionError)
		r.client.Status().Update(context.TODO(), instance)
		return reconcile.Result{}, err
	}

	// In theory this could return more than one VPC for master nodes. That would be
	// odd since the security group is associated with a single VPC.
	vpcs, err := utils.GetMasterNodeVPCs(r.client)
	if err != nil {
		reqLogger.Error(err, "Couldn't get the VPC in use by master nodes")
		SetAPISchemeStatus(reqLogger, instance, "Couldn't reconcile", "Couldn't get the VPC id for master nodes", cloudingressv1alpha1.ConditionError)
		r.client.Status().Update(context.TODO(), instance)
		return reconcile.Result{}, err
	}
	if len(vpcs) > 1 {
		reqLogger.Info(fmt.Sprintf("Multiple VPCs detected for master nodes. Using %s for security group", vpcs[0]))
	}

	lb := &LoadBalancer{
		EndpointName:     config.AdminAPIName,
		Subnets:          subnets,
		Tags:             ownerTags,
		MachineInstances: masterNodeInstances,
		BaseDomain:       clusterBaseDomain,
	}
	cidrAccess := &CIDRAccess{
		LoadBalancer:         lb,
		SecurityGroupName:    config.AdminAPISecurityGroupName,
		SecurityGroupVPCName: vpcs[0],
		Tags:                 ownerTags,
	}

	// Now try to make sure all the things for Admin API are present in AWS
	err = ensureAdminAPIEndpoint(reqLogger, instance, awsClient, lb, cidrAccess)
	if err != nil {
		reqLogger.Error(err, "Couldn't ensure the admin API endpoint")
		SetAPISchemeStatus(reqLogger, instance, "Couldn't reconcile", "Couldn't ensure the admin API endpoint: "+err.Error(), cloudingressv1alpha1.ConditionError)
		r.client.Status().Update(context.TODO(), instance)
		return reconcile.Result{}, err
	}

	// And now, tell the APIServer/cluster object about it.
	err = r.addAdminAPIToAPIServerObject(reqLogger, config.AdminAPIName+"."+clusterBaseDomain)
	if err != nil {
		reqLogger.Error(err, "Couldn't update APIServer/cluster object")
		SetAPISchemeStatus(reqLogger, instance, "Couldn't reconcile", "Couldn't update APIServer/cluster object", cloudingressv1alpha1.ConditionError)
		r.client.Status().Update(context.TODO(), instance)
		return reconcile.Result{}, err
	}
	SetAPISchemeStatus(reqLogger, instance, "Success", "Admin API Endpoint created", cloudingressv1alpha1.ConditionReady)
	r.client.Status().Update(context.TODO(), instance)

	return reconcile.Result{}, nil
}

func ensureCloudLoadBalancer(reqLogger logr.Logger, awsAPI *awsclient.AwsClient, lb *LoadBalancer) (*awsclient.AWSLoadBalancer, error) {
	var awsObj *awsclient.AWSLoadBalancer
	found := false
	var err error
	for i := 1; i <= config.MaxAPIRetries; i++ {
		found, awsObj, err = awsAPI.DoesELBExist(lb.EndpointName)
		if err != nil {
			reqLogger.Info("Couldn't determine if the Admin API ELB exists due to error: " + err.Error())
			if i == config.MaxAPIRetries {
				reqLogger.Info("Out of retries")
				return nil, err
			}
			reqLogger.Info(fmt.Sprintf("Sleeping %d seconds before retrying...", i))
			time.Sleep(time.Duration(i) * time.Second)
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
			awsObj, err = awsAPI.CreateClassicELB(lb.EndpointName, lb.Subnets, 6443, lb.Tags)
			if err != nil {
				if i == config.MaxAPIRetries {
					reqLogger.Info("Out of retries")
					return nil, err
				}
				reqLogger.Info(fmt.Sprintf("Sleeping %d seconds before retrying...", i))
				time.Sleep(time.Duration(i) * time.Second)
			} else {
				reqLogger.Info(fmt.Sprintf("Created ELB for Admin API in DNS zone %s with DNS name %s", awsObj.DNSZoneId, awsObj.DNSName))
				break
			}
		}
	}
	return awsObj, nil
}

func ensureCIDRAccess(reqLogger logr.Logger, crObject *cloudingressv1alpha1.APIScheme, awsAPI *awsclient.AwsClient, cidrAccess *CIDRAccess) error {
	for i := 1; i <= config.MaxAPIRetries; i++ {

		err := awsAPI.EnsureCIDRAccess(cidrAccess.LoadBalancer.EndpointName, cidrAccess.SecurityGroupName, cidrAccess.SecurityGroupVPCName, crObject.Spec.ManagementAPIServerIngress.AllowedCIDRBlocks, cidrAccess.Tags)
		if err != nil {
			reqLogger.Info("Error ensuring CIDR access for the security group: " + err.Error())
			if i == config.MaxAPIRetries {
				reqLogger.Info("Out of retries")
				return err
			}
			reqLogger.Info(fmt.Sprintf("Sleeping %d seconds before retrying...", i))
			time.Sleep(time.Duration(i) * time.Second)
		} else {
			reqLogger.Info("Security Group CIDR access ensured")
			break
		}
	}
	return nil
}

func ensureLoadBalancerInstances(reqLogger logr.Logger, awsAPI *awsclient.AwsClient, lb *LoadBalancer) error {
	for i := 1; i <= config.MaxAPIRetries; i++ {
		err := awsAPI.AddLoadBalancerInstances(lb.EndpointName, lb.MachineInstances)
		if err != nil {
			reqLogger.Info(fmt.Sprintf("Couldn't add instances %s to ELB %s: %s", lb.MachineInstances, lb.EndpointName, err.Error()))
			if i == config.MaxAPIRetries {
				reqLogger.Info("Out of retries")
				return err
			}
			reqLogger.Info(fmt.Sprintf("Sleeping %d seconds before retrying...", i))
			time.Sleep(time.Duration(i) * time.Second)
		} else {
			reqLogger.Info(fmt.Sprintf("Added instances %s to ELB", lb.MachineInstances))
			break
		}
	}
	return nil
}

func ensureDNSRecord(reqLogger logr.Logger, awsAPI *awsclient.AwsClient, lb *LoadBalancer, awsObj *awsclient.AWSLoadBalancer) error {
	for i := 1; i <= config.MaxAPIRetries; i++ {
		err := awsAPI.UpsertCNAME(lb.BaseDomain, lb.EndpointName, awsObj.DNSZoneId, lb.EndpointName+"."+lb.BaseDomain, "RH API Endpoint", false)
		if err != nil {
			reqLogger.Info("Couldn't upsert a DNS record: " + err.Error())
			if i == config.MaxAPIRetries {
				reqLogger.Info("Out of retries")
				return err
			}
			reqLogger.Info(fmt.Sprintf("Sleeping %d seconds before retrying...", i))
			time.Sleep(time.Duration(i) * time.Second)
		} else {
			break
		}
	}
	return nil
}

// ensureAdminAPIEndpoint will ensure the Admin API endpoint exists. Returns any error
// This function is idempotent
func ensureAdminAPIEndpoint(reqLogger logr.Logger, crObject *cloudingressv1alpha1.APIScheme, awsAPI *awsclient.AwsClient, lb *LoadBalancer, cidrAccess *CIDRAccess) error {

	// First, let's ensure an ELB exists
	awsObj, err := ensureCloudLoadBalancer(reqLogger, awsAPI, lb)
	if err != nil {
		// This is actually fatal due to the retries in ensureCloudLoadBalancer
		return err
	}
	// Now, ensure CIDR access
	err = ensureCIDRAccess(reqLogger, crObject, awsAPI, cidrAccess)
	if err != nil {
		// This is actually fatal
		return err
	}
	// Add the "master" instances are attached to the ELB
	err = ensureLoadBalancerInstances(reqLogger, awsAPI, lb)
	if err != nil {
		// This is actually fatal
		return err
	}
	err = ensureDNSRecord(reqLogger, awsAPI, lb, awsObj)
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
func (r *ReconcileAPIScheme) addAdminAPIToAPIServerObject(logger logr.Logger, domainName string) error {
	api := &configv1.APIServer{}
	ns := types.NamespacedName{
		Namespace: "",
		Name:      "cluster",
	}
	err := r.client.Get(context.TODO(), ns, api)
	if err != nil {
		return err
	}
	for i, name := range api.Spec.ServingCerts.NamedCertificates {
		if strings.HasPrefix(name.ServingCertificate.Name, "api.") {
			api.Spec.ServingCerts.NamedCertificates[i].Names = append(api.Spec.ServingCerts.NamedCertificates[i].Names, domainName)
			return r.client.Update(context.TODO(), api)
		}
	}
	return fmt.Errorf("Couldn't find api name for APIServer. Did no work")
}

// SetAPISchemeStatus will set the status on the APISscheme object with a human message, as in an error situation
func SetAPISchemeStatus(reqLogger logr.Logger, crObject *cloudingressv1alpha1.APIScheme, reason, message string, ctype cloudingressv1alpha1.APISchemeConditionType) {
	crObject.Status.Conditions = utils.SetAPISchemeCondition(
		crObject.Status.Conditions,
		ctype,
		corev1.ConditionTrue,
		reason,
		message,
		utils.UpdateConditionNever)
	crObject.Status.State = ctype
}
