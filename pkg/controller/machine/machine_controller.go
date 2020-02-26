package machine

import (
	"context"

	"github.com/openshift/cloud-ingress-operator/pkg/awsclient"
	"github.com/openshift/cloud-ingress-operator/pkg/config"
	utils "github.com/openshift/cloud-ingress-operator/pkg/controller/utils"
	machineapiv1 "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	awsproviderapi "sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsproviderconfig/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_machine")

var (
	Scheme = runtime.NewScheme()
)

var oldInstanceID string
var newInstanceID string

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Machine Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileMachine{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("machine-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	log.Info("Evaluating machine objects")
	src := &source.Kind{Type: &machineapiv1.Machine{}}

	pred := predicate.Funcs{
		// filter out all Create, Delete, Generic events
		CreateFunc: func(e event.CreateEvent) bool {
			log.Info("Filtering out create event for node", "node", e.Meta.GetName())
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			log.Info("Filtering out delete event for node", "node", e.Meta.GetName())
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			log.Info("Filtering out generic event for node", "node", e.Meta.GetName())
			return false
		},

		UpdateFunc: func(e event.UpdateEvent) bool {
			log.Info("Evaluating machine node", "node", e.MetaNew.GetName())
			// we need to filter events based on whether node is a master,
			// and whether the node's status.providerStatus.instanceID
			// has changed
			if nodeType, typeLabelExist := e.MetaNew.GetLabels()["machine.openshift.io/cluster-api-machine-type"]; typeLabelExist {
				if nodeType != "master" {
					// machine isn't master node, so we don't care about the change
					log.Info("Node isn't master", "node", e.MetaNew.GetName())
					return false
				}
			} else {
				// machine does not have a nodeType label. we should never get here
				log.Info("Node doesn't have a nodeType label", "node", e.MetaNew.GetName())
				return false
			}

			// if we're here, machine is master node, so we need to determine
			// if the status.providerStatus.instanceID changed.

			// first, cast MetaOld and MetaNew to Machine objects
			oldM, ok := e.MetaOld.(*machineapiv1.Machine)
			if !ok {
				log.Error(nil, "Error casting MetaOld to Machine object")
				return false
			}
			newM, ok := e.MetaNew.(*machineapiv1.Machine)
			if !ok {
				log.Error(nil, "Error casting MetaNew to Machine object")
				return false
			}

			// then, decode Machine objects' ProviderStatus (runtime.rawextension)
			awsCodec, err := awsproviderapi.NewCodec()
			if err != nil {
				log.Error(err, "Error creating AWSProviderConfigCodec")
				return false
			}

			oldMProviderStatusObj := &awsproviderapi.AWSMachineProviderStatus{}
			err = awsCodec.DecodeProviderStatus(oldM.Status.ProviderStatus, oldMProviderStatusObj)
			if err != nil {
				log.Error(err, "Error creating old ProviderStatus object")
				return false
			}

			newMProviderStatusObj := &awsproviderapi.AWSMachineProviderStatus{}
			err = awsCodec.DecodeProviderStatus(newM.Status.ProviderStatus, newMProviderStatusObj)
			if err != nil {
				log.Error(err, "Error creating new ProviderStatus object")
				return false
			}

			// this allows us to compare InstanceIDs
			oldInstanceID = *oldMProviderStatusObj.InstanceID
			newInstanceID = *newMProviderStatusObj.InstanceID

			if oldInstanceID == newInstanceID {
				// if the InstanceID hasn't changed, we don't care about the change
				log.Info("Node is a master node, but status.providerStatus.instanceID did not change", "node", e.MetaNew.GetName())
				return false
			}

			// if we're here, this is a master node with a changed instanceID
			log.Info("Node's old instance ID", "instanceID", oldInstanceID)
			log.Info("Node's new instance ID", "instanceID", newInstanceID)
			log.Info("Node is a master node with changed status.providerStatus.instanceID, reconciling node", "node", e.MetaNew.GetName())
			return true
		},
	}

	// Watch for changes to primary resource Machine
	err = c.Watch(src, &handler.EnqueueRequestForObject{}, pred)
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileMachine implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileMachine{}

// ReconcileMachine reconciles a Machine object
type ReconcileMachine struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Machine object and makes changes based on the state read
// and what is in the Machine.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileMachine) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Machine")

	// Fetch the Machine instance
	m := &machineapiv1.Machine{}

	// put instance IDs in arrays (needed by functions below)
	oldInstance := []string{oldInstanceID}
	newInstance := []string{newInstanceID}

	err := r.client.Get(context.TODO(), request.NamespacedName, m)
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

	// get AWS region for AWSClient instance
	region, err := utils.GetClusterRegion(r.client)
	if err != nil {
		reqLogger.Error(err, "Failed to get AWS region")
		return reconcile.Result{}, err
	}

	// create AWSClient instance
	awsClient, err := awsclient.GetAWSClient(r.client, awsclient.NewAwsClientInput{
		AwsRegion:  region,
		SecretName: config.AWSSecretName,
		NameSpace:  config.OperatorNamespace,
	})
	if err != nil {
		reqLogger.Error(err, "Failed to get AWS client")
		return reconcile.Result{}, err
	}

	// get list of LBs from AWSClient instance
	loadBalancerInfo, err := awsClient.ListAllNLBs()
	if err != nil {
		reqLogger.Error(err, "Error listing all NLBs")
		return reconcile.Result{}, err
	}

	// loop over LBs in LB list to remove old instanceID and add new one
	for _, loadbalancer := range loadBalancerInfo {
		loadBalancer := awsclient.AWSLoadBalancer{
			ELBName:   loadbalancer.LoadBalancerName,
			DNSName:   loadbalancer.DNSName,
			DNSZoneId: loadbalancer.CanonicalHostedZoneNameID,
		}
		err = awsClient.RemoveInstancesFromLoadBalancer(loadBalancer.ELBName, oldInstance)
		if err != nil {
			reqLogger.Error(err, "Error removing old instanceID from LB")
			return reconcile.Result{}, err
		}
		err = awsClient.AddLoadBalancerInstances(loadBalancer.ELBName, newInstance)
		if err != nil {
			reqLogger.Error(err, "Error adding new instanceID to LB")
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}
