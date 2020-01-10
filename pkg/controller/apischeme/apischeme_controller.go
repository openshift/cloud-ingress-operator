package apischeme

import (
	"context"

	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	"github.com/openshift/cloud-ingress-operator/pkg/awsclient"
	"github.com/openshift/cloud-ingress-operator/pkg/config"

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

var log = logf.Log.WithName("controller_apischeme")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new ApiScheme Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileApiScheme{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("apischeme-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ApiScheme
	err = c.Watch(&source.Kind{Type: &cloudingressv1alpha1.ApiScheme{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileApiScheme implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileApiScheme{}

// ReconcileApiScheme reconciles a ApiScheme object
type ReconcileApiScheme struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile will ensure that the rh-api management api endpoint is created and ready.
// Rough Steps:
// Initial state is Pending
// 1. Create AWS ELB (CreatingLoadBalancer)
// 2. Create Security Group with allowed CIDR blocks (UpdatingCIDRAllownaces)
// 3. Add master Node EC2 instances to the load balancer as listeners (6443/TCP) (UpdatingLoadBalancerListeners)
// 4. Update APIServer object to add a record for the rh-api endpoint (UpdatingAPIServer)
// 5. Ready for work (Ready)
func (r *ReconcileApiScheme) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ApiScheme")

	// TODO: Add controller to observe Machine objects in case the master nodes change (eg updating listeners)

	// Fetch the ApiScheme instance
	instance := &cloudingressv1alpha1.ApiScheme{}
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

	awsClient, err := awsclient.NewClient("access id", "secret", "token", "region")
	if err != nil {
		return reconcile.Result{}, err
	}

	switch instance.Status.State {
	case cloudingressv1alpha1.Pending:
		err = updateCondition(instance,
			"Moving to Create Load Balancer",
			"Transition",
			cloudingressv1alpha1.CreatingLoadBalancer)
		return reconcile.Result{}, nil
	case cloudingressv1alpha1.CreatingLoadBalancer:
		// if the ELB is created already, go to next step
		if found, err := awsClient.DoesELBExist(config.CloudAdminAPILoadBalancerName); err != nil {
			return reconcile.Result{}, err
		} else if !found {
			//create
			// TODO: Detect the az and subnets

			// subnet from config.openshift.io network/cluster object, need to get subnet ID
			// az from machine record for master objects
			// Lisa to ask apiserver folks for an in-cluster representation of subnet id and az
			dnsName, err := awsClient.CreateClassicELB(config.CloudAdminAPILoadBalancerName, []string{}, []string{}, config.AdminAPIListenerPort)
			if err != nil {
				reqLogger.Error(err, "Error while creating Load Balancer", config.CloudAdminAPILoadBalancerName)
				return reconcile.Result{}, err
			}
			reqLogger.Info("DNS Name for ELB from Amazon is %s", dnsName)
			// Rogerio TODO: update dns name, state and status

			return reconcile.Result{}, nil
		} else {
			//found
		}

	case cloudingressv1alpha1.UpdatingCIDRAllowances:
		// if the CIDR list is synced, go to next step
	case cloudingressv1alpha1.UpdatingLoadBalancerListeners:
		// if listeners are current, go to next step
	case cloudingressv1alpha1.UpdatingAPIServer:
		// if APIServer is current, go to next step
	default:
		// idk!

	}

	return reconcile.Result{}, nil
}

func updateCondition(instance *cloudingressv1alpha1.ApiScheme, msg, reason string, nextState cloudingressv1alpha1.ManagementState) error {
	instance.Status.State = nextState
	return nil
}

/*
func setAccountStatus(reqLogger logr.Logger, awsAccount *awsv1alpha1.Account, message string, ctype awsv1alpha1.AccountConditionType, state string) {
	awsAccount.Status.Conditions = controllerutils.SetAccountCondition(
			awsAccount.Status.Conditions,
			ctype,
			corev1.ConditionTrue,
			state,
			message,
			controllerutils.UpdateConditionNever)
	awsAccount.Status.State = state
	reqLogger.Info(fmt.Sprintf("Account %s status updated", awsAccount.Name))
}
*/
