package publishingstrategy

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	"github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	"github.com/openshift/cloud-ingress-operator/pkg/cloudclient"
	ctlutils "github.com/openshift/cloud-ingress-operator/pkg/controller/utils"
	"github.com/openshift/cloud-ingress-operator/pkg/ingresscontroller"
	baseutils "github.com/openshift/cloud-ingress-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"time"
)

const (
	ingressControllerNamespace = "openshift-ingress-operator"
	infraNodeLabelKey          = "node-role.kubernetes.io/infra"
	CloudIngressFinalizer      = "cloudingress.managed.openshift.io/finalizer-cloud-ingress-controller"
	ClusterIngressFinalizer    = "ingresscontroller.operator.openshift.io/finalizer-ingresscontroller"
)

var log = logf.Log.WithName("controller_publishingstrategy")

type patchField string

var IngressControllerSelector patchField = "IngressControllerSelector"
var IngressControllerCertificate patchField = "IngressControllerCertificate"
var IngressControllerNodePlacement patchField = "IngressControllerNodePlacement"
var IngressControllerEndPoint patchField = "IngressControllerEndpoint"
var IngressControllerDeleteLBAnnotation string = "ingress.operator.openshift.io/auto-delete-load-balancer"

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
	err = c.Watch(&source.Kind{Type: &v1alpha1.PublishingStrategy{}}, &handler.EnqueueRequestForObject{})
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
func (r *ReconcilePublishingStrategy) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling PublishingStrategy")

	// Fetch the PublishingStrategy instance
	instance := &v1alpha1.PublishingStrategy{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if k8serr.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Get all IngressControllers on cluster with an annotation that indicates cloud-ingress-operator owns it
	ingressControllerList := &ingresscontroller.IngressControllerList{}
	listOptions := []client.ListOption{
		client.InNamespace("openshift-ingress-operator"),
	}
	err = r.client.List(context.TODO(), ingressControllerList, listOptions...)
	if err != nil {
		log.Error(err, "Cannot get list of ingresscontroller")
		return reconcile.Result{}, err
	}

	// Retrieve the cluster base domain. Discard the error since it's just for logging messages.
	// In case of failure, clusterBaseDomain is an empty string.
	clusterBaseDomain, _ := baseutils.GetClusterBaseDomain(r.client)

	ownedIngressControllers := getIngressWithCloudIngressOpreatorOwnerAnnotation(*ingressControllerList)

	/* To ensure that the set of all IngressControllers owned by cloud-ingress-operator
	match the list of ApplicationIngresses in the PublishingStrategy, a map is created to
	tie IngressControllers existing on cluster with the owner annotation
	to the ApplicationIngresses existing in the PublishingStrategy CR. If an IngressController CR
	owned by cloud-ingress-operator exists on cluster but an associated ApplicationIngress does not exist in
	the PublishingStrategy CR (likely removed from the list of ApplicationIngress),
	that IngressController CR should be deleted.
	As each ApplicationIngress is processed, and the corresponding IngressController CR is confirmed to exist,
	that entry in the map will be turned `true`. When all ApplicationIngresses have been processed, any remaining
	IngressController CRs in the map with the value `false` will be deleted.
	*/
	ownedIngressExistingMap := make(map[string]bool, 1)
	for _, ownedIngress := range ownedIngressControllers.Items {
		// Initalize them all to false as they have not been verified against ApplicationIngress list
		ownedIngressExistingMap[ownedIngress.Name] = false
	}

	/* Each ApplicationIngress defines a desired spec for an IngressController
	// The following loop goes through each ApplicationIngress defined in the
	PublishingStrategy CR. For each ApplicationIngress, a desired IngressController spec
	is generated using the definition in the PublishingStrategy. In an IngressController
	there are some fields that are immutable, and some that are. For the immutable fields
	the IngressController must be deleted and recreated with the desired spec fields.
	For the mutable fields, a patch to the object will suffice.
	First, a check is done to see if the desired IngressController exists. If it doesn't,
	then its created. If the IngressController does exist, the immutable fields are check.
	If any immutable field differs between the desired and the generated spec, the IngressController
	is deleted and should be created the next reconcile.
	Once the immuatble fields are checked, then then mutable fields are. If any of the mutable fields
	differ, the IngressController is patched.
	The default IngressController is special. When its first created, the spec is not filled out.
	Instead, the relavent fields are set in the status. For each of the spec checks, the status
	will also be checked if the ApplicationIngress references the default IngressController
	*/
	for _, ingressDefinition := range instance.Spec.ApplicationIngress {

		// Set the IngressController CRs name based on the DNSName
		ingressName := getIngressName(ingressDefinition.DNSName)

		if ingressDefinition.Default {
			// The default IngressController should be named "default" which is expected by cluster-ingress-operator
			ingressName = "default"

			// Safety check, to ensure that the default ingress controller DNS name matches the cluster's base domain
			// This protects against malformed publishing strategies
			if !strings.HasSuffix(ingressDefinition.DNSName, clusterBaseDomain) {
				return reconcile.Result{}, fmt.Errorf("default ingress DNS doesn't match cluster's base domain: got %v, expected to end in %v", ingressDefinition.DNSName, clusterBaseDomain)
			}
		}

		reqLogger.Info(fmt.Sprintf("Checking ApplicationIngress for %s IngressController CR", ingressName))

		/* Each ApplicationIngress refers to an IngressController CR. Here, the namespaced name
		is built based on that reference so that an attempt can be made to GET the IngressController
		This verifies that the IngressController exists and uses that for other checks, or triggers a creation
		if it doesn't. getIngressName is a function which returns the name of an IngressController CR given its
		DNS uri. It's used here to properly get the name to build a namespaced name.
		*/
		namespacedName := types.NamespacedName{Name: ingressName, Namespace: ingressControllerNamespace}

		// Generate the desired IngressController spec based on the ApplicationIngress definition.
		// This generated spec will be compared against the actual spec as desrcibed above
		desiredIngressController := generateIngressController(ingressDefinition)

		cloudPlatform, err := baseutils.GetPlatformType(r.client)
		if err != nil {
			return reconcile.Result{}, err
		}
		isAWS := *cloudPlatform == "AWS"
		// Add ProviderParameters if the cloud is AWS to ensure LB type matches
		if isAWS {

			// Default to Classic LB to match default IngressController behavior
			if instance.Spec.DefaultAPIServerIngress.Type == "" {
				instance.Spec.DefaultAPIServerIngress.Type = "Classic"
			}

			desiredIngressController.Spec.EndpointPublishingStrategy.LoadBalancer.ProviderParameters = &ingresscontroller.ProviderLoadBalancerParameters{
				Type: ingresscontroller.AWSLoadBalancerProvider,
				AWS: &ingresscontroller.AWSLoadBalancerParameters{
					Type: ingresscontroller.AWSLoadBalancerType(instance.Spec.DefaultAPIServerIngress.Type),
				},
			}
		}

		// Attempt to find the IngressController referenced by the ApplicationIngress
		// by doing a GET of the namespaced name object build above against the k8s api.
		ingressController := &ingresscontroller.IngressController{}
		err = r.client.Get(context.TODO(), namespacedName, ingressController)
		if err != nil {
			// Attempt to create the CR if not found
			if k8serr.IsNotFound(err) {
				reqLogger.Info(fmt.Sprintf("ApplicationIngress %s not found, attempting to create", ingressName))
				err = r.client.Create(context.TODO(), desiredIngressController)
				if err != nil {
					return reconcile.Result{}, err
				}
				// If the CR was created, requeue PublishingStrategy
				return reconcile.Result{Requeue: true}, nil
			}
			return reconcile.Result{}, err
		}

		// Mark the IngressControllers as existing
		ownedIngressExistingMap[ingressController.Name] = true

		// When an ingresscontroller is being deleted, it takes time as it needs to delete several
		// services (ie the load balancer service has finalizers for the cloud provider resource cleanup)
		if !ingressController.DeletionTimestamp.IsZero() {
			return r.ensureIngressController(reqLogger, ingressController, desiredIngressController)
		}

		// For AWS, ensure the LB type matches between the IngressController and PublishingStrategy
		if isAWS {
			result, err := r.ensureAWSLoadBalancerType(reqLogger, ingressController, *instance)
			if err != nil || result.Requeue {
				return result, err
			}

		}

		result, err := r.ensureStaticSpec(reqLogger, ingressController, desiredIngressController)
		if err != nil || result.Requeue {
			return result, err
		}

		result, err = r.ensurePatchableSpec(reqLogger, ingressController, desiredIngressController)
		if err != nil || result.Requeue {
			return result, err
		}
	}

	result, err := r.ensureAnnotationsDefined(reqLogger, ownedIngressExistingMap)
	if err != nil || result.Requeue {
		return result, err
	}

	result, err = r.deleteUnpublishedIngressControllers(ownedIngressExistingMap)
	if err != nil || result.Requeue {
		return result, err
	}

	result, err = r.ensureAliasScope(reqLogger, instance, clusterBaseDomain)
	if err != nil || result.Requeue {
		return result, err
	}

	return reconcile.Result{}, nil
}

// getIngressName takes the domain name and returns the name of the IngressController CR
func getIngressName(dnsName string) string {
	firstPeriodIndex := strings.Index(dnsName, ".")
	newIngressName := dnsName[:firstPeriodIndex]
	return newIngressName
}

// Generates an IngressController CR object based on the configuration of an ApplicationIngress instance
func generateIngressController(appIngress v1alpha1.ApplicationIngress) *ingresscontroller.IngressController {
	// Translate the ApplicationIngress listening string into the matching type for the IngressController
	loadBalancerScope := ingresscontroller.LoadBalancerScope("")
	switch appIngress.Listening {
	case "internal":
		loadBalancerScope = ingresscontroller.InternalLoadBalancer
	case "external":
		loadBalancerScope = ingresscontroller.ExternalLoadBalancer
	default:
		loadBalancerScope = ingresscontroller.ExternalLoadBalancer
	}

	ingressName := getIngressName(appIngress.DNSName)
	if appIngress.Default {
		ingressName = "default"
	}

	// Builds the IngressController CR object based on the ApplicationIngress
	return &ingresscontroller.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressName,
			Namespace: ingressControllerNamespace,
			Annotations: map[string]string{
				"Owner": "cloud-ingress-operator",
			},
		},
		Spec: ingresscontroller.IngressControllerSpec{
			DefaultCertificate: &corev1.LocalObjectReference{
				Name: appIngress.Certificate.Name,
			},
			NodePlacement: &ingresscontroller.NodePlacement{
				NodeSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{infraNodeLabelKey: ""},
				},
				Tolerations: []corev1.Toleration{
					{
						Key:      infraNodeLabelKey,
						Effect:   corev1.TaintEffectNoSchedule,
						Operator: corev1.TolerationOpExists,
					},
				},
			},
			Domain: appIngress.DNSName,
			EndpointPublishingStrategy: &ingresscontroller.EndpointPublishingStrategy{
				Type: ingresscontroller.LoadBalancerServiceStrategyType,
				LoadBalancer: &ingresscontroller.LoadBalancerStrategy{
					Scope: loadBalancerScope,
				},
			},
			RouteSelector: &metav1.LabelSelector{
				MatchLabels: appIngress.RouteSelector.MatchLabels,
			},
		},
	}
}

/* Compares the static spec fields of the desired Spec against the existing IngressController's status.
Both the Domain and EndpointPublishingStrategy fields in the IngressController spec are static. This means
that editing them will have no effect. Instead, the CR must be fully deleted and recreated with the desired
Domain and EndpointPublishingStrategy filled in. The default IngressController does not have those spec fields
filled in at all when fisrt created. Instead, the default CRs status holds the correct values. This function
is meant to be used when the default CRs spec is not filled in to see if the desired configuration is present in
its status instead. Returns false if at least one of the existing fields don't match the desired.
*/
func validateStaticStatus(ingressController ingresscontroller.IngressController, desiredSpec ingresscontroller.IngressControllerSpec) bool {

	if !(desiredSpec.Domain == ingressController.Status.Domain) {
		return false
	}
	if !baseutils.IsVersionHigherThan("4.10") {
		// Preventing nil pointer errors
		if ingressController.Status.EndpointPublishingStrategy == nil || ingressController.Status.EndpointPublishingStrategy.LoadBalancer == nil {
			return false
		}

		if !(desiredSpec.EndpointPublishingStrategy.LoadBalancer.Scope == ingressController.Status.EndpointPublishingStrategy.LoadBalancer.Scope) {
			return false
		}
	}

	return true
}

/* Compares the static spec fields of the desired Spec against the existing IngressController's spec.
Both the Domain and EndpointPublishingStrategy fields in the IngressController spec are static. This means
that editing them will have no effect. Instead, the CR must be fully deleted and recreated with the desired
Domain and EndpointPublishingStrategy filled in. Returns false if at least one of the existing fields don't match the desired
*/

func validateStaticSpec(ingressController ingresscontroller.IngressController, desiredSpec ingresscontroller.IngressControllerSpec) bool {
	if !(desiredSpec.Domain == ingressController.Spec.Domain) {
		return false
	}

	if !baseutils.IsVersionHigherThan("4.10") {
		// Preventing nil pointer errors
		if ingressController.Spec.EndpointPublishingStrategy == nil || ingressController.Spec.EndpointPublishingStrategy.LoadBalancer == nil {
			return false
		}

		if !(desiredSpec.EndpointPublishingStrategy.LoadBalancer.Scope == ingressController.Spec.EndpointPublishingStrategy.LoadBalancer.Scope) {
			return false
		}
	}

	return true
}

func (r *ReconcilePublishingStrategy) ensureAWSLoadBalancerType(reqLogger logr.Logger, ic *ingresscontroller.IngressController, ps v1alpha1.PublishingStrategy) (result reconcile.Result, err error) {

	if !validateAWSLoadBalancerType(*ic, ps) {

		if err := r.client.Delete(context.TODO(), ic); err != nil {
			reqLogger.Error(err, "Error deleting IngressController")
		}
		return reconcile.Result{Requeue: true}, nil
	}

	return result, nil
}

func validateAWSLoadBalancerType(ic ingresscontroller.IngressController, ps v1alpha1.PublishingStrategy) bool {

	if ic.Spec.EndpointPublishingStrategy == nil {
		return false
	}

	// If ProviderParameters are set on the IngressController, then the Type in the PublishingStrategy needs to match exacly
	if ic.Spec.EndpointPublishingStrategy.LoadBalancer.ProviderParameters != nil {

		return ingresscontroller.AWSLoadBalancerType(ps.Spec.DefaultAPIServerIngress.Type) == ic.Spec.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS.Type
	}

	if ic.Status.EndpointPublishingStrategy == nil {
		return false
	}

	// The status can also hold this information if its not in the spec
	if ic.Status.EndpointPublishingStrategy.LoadBalancer.ProviderParameters != nil {

		return ingresscontroller.AWSLoadBalancerType(ps.Spec.DefaultAPIServerIngress.Type) == ic.Status.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS.Type
	}

	// When ProviderParameters is not set, the IngressController defaults to Classic, so we only need to ensure the PublishingStrategy Type is not set to  NLB
	if ps.Spec.DefaultAPIServerIngress.Type == "NLB" {
		return false
	}

	return true

}

/* Compares the patchable desired Spec against the existing IngressController's status.
The only patchable field that the ApplicationIngress controlls thats also stored in the IngressController's
status is the RouteSelector. The default IngressController does not have those spec fields
filled in at all when fisrt created. Instead, the default CRs status holds the correct values. This function
is meant to be used when the default CRs spec is not filled in to see if the desired configuration is present in
its status instead. Returns false if the RouteSelector in the status is empty or does not match the desired Spec.
*/
func validatePatchableStatus(ingressController ingresscontroller.IngressController, desiredSpec ingresscontroller.IngressControllerSpec) (bool, patchField) {
	ingressControllerSelector, _ := metav1.ParseToLabelSelector(ingressController.Status.Selector)
	if ingressControllerSelector != nil {
		if !(reflect.DeepEqual(desiredSpec.RouteSelector.MatchLabels, ingressControllerSelector.MatchLabels)) {
			return false, IngressControllerSelector
		}
	} else {
		if desiredSpec.RouteSelector != nil {
			return false, IngressControllerSelector
		}
	}

	return true, ""
}

/* Compares the patchable desired Spec against the existing IngressController's spec.
Both the DefaultCertificate and the RouteSelector fields in the IngressController spec are patchable.
The function returns false if a field doesn't match and which field specifically should be changed.
*/
func validatePatchableSpec(ingressController ingresscontroller.IngressController, desiredSpec ingresscontroller.IngressControllerSpec) (bool, patchField) {

	// Preventing nil pointer errors
	if ingressController.Spec.RouteSelector == nil {
		return false, IngressControllerSelector
	}

	if !(reflect.DeepEqual(desiredSpec.RouteSelector.MatchLabels, ingressController.Spec.RouteSelector.MatchLabels)) {
		return false, IngressControllerSelector
	}

	// Preventing nil pointer errors
	if ingressController.Spec.DefaultCertificate == nil {
		return false, IngressControllerCertificate
	}

	if !(desiredSpec.DefaultCertificate.Name == ingressController.Spec.DefaultCertificate.Name) {
		return false, IngressControllerCertificate
	}

	// Preventing nil pointer errors
	if ingressController.Spec.NodePlacement == nil {
		return false, IngressControllerNodePlacement
	}

	if !(reflect.DeepEqual(desiredSpec.NodePlacement.NodeSelector.MatchLabels, ingressController.Spec.NodePlacement.NodeSelector.MatchLabels)) {
		return false, IngressControllerNodePlacement
	}
	if baseutils.IsVersionHigherThan("4.10") {
		// Preventing nil pointer errors
		if ingressController.Spec.EndpointPublishingStrategy == nil || ingressController.Spec.EndpointPublishingStrategy.LoadBalancer == nil {
			return false, IngressControllerEndPoint
		}

		if !(desiredSpec.EndpointPublishingStrategy.LoadBalancer.Scope == ingressController.Spec.EndpointPublishingStrategy.LoadBalancer.Scope) {
			return false, IngressControllerEndPoint
		}
	}

	return true, ""
}

// Given an IngressControllerList, returns only IngressControllers with the cloud-ingress-operator Owner annotation
func getIngressWithCloudIngressOpreatorOwnerAnnotation(ingressList ingresscontroller.IngressControllerList) *ingresscontroller.IngressControllerList {

	ownedIngressList := &ingresscontroller.IngressControllerList{}

	for _, ingress := range ingressList.Items {
		if _, ok := ingress.Annotations["Owner"]; ok {
			if ingress.Annotations["Owner"] == "cloud-ingress-operator" {
				ownedIngressList.Items = append(ownedIngressList.Items, ingress)
			}
		}
	}

	return ownedIngressList
}

// deleteUnpublishedIngressControllers deletes all IngressControllers owned by cloud-ingress-controller which are not in the publishingstategy
func (r *ReconcilePublishingStrategy) deleteUnpublishedIngressControllers(ownedIngressExistingMap map[string]bool) (result reconcile.Result, err error) {
	// Delete all IngressControllers that are owned by cloud-ingress-operator but not in PublishingStrategy
	for ingress, inPublishingStrategy := range ownedIngressExistingMap {
		if !inPublishingStrategy {
			// Delete requires an object referece, so we must get it first
			ingressToDelete := &ingresscontroller.IngressController{}
			err = r.client.Get(context.TODO(), types.NamespacedName{Name: ingress, Namespace: ingressControllerNamespace}, ingressToDelete)
			if err != nil {
				return reconcile.Result{}, err
			}
			err = r.client.Delete(context.TODO(), ingressToDelete)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	}
	return result, err
}

// ensureStaticSpec deletes or marks an IngressController for deletion when a static spec has been changed in the publishing strategy
func (r *ReconcilePublishingStrategy) ensureStaticSpec(reqLogger logr.Logger, ingressController, desiredIngressController *ingresscontroller.IngressController) (result reconcile.Result, err error) {
	reqLogger.Info(fmt.Sprintf("Checking Static Spec for IngressController %s ", desiredIngressController.Name))
	// Compare the Spec fields that cannot be patched in the desired IngressController and the actual IngressController
	if !validateStaticSpec(*ingressController, desiredIngressController.Spec) {
		// "The default" CR also needs a status check as the config isn't always guaranteed to be in the spec
		if desiredIngressController.Name == "default" {
			reqLogger.Info("Static Spec does not match for default IngressController, checking Status")
			if !validateStaticStatus(*ingressController, desiredIngressController.Spec) {
				// Since the default IngressController CR spec + status does not match the desired IngressController
				// spec that was generated based on the ApplicationIngress, and the fields checked are immutable,
				// the actual default IngressController must be deleted
				reqLogger.Info("Static Spec and Status do not match for default IngressController, deleting")
				// TODO: Should we return an error here if this delete fails?

				// Prior to deleting the ingress controller, we are  adding a finalizer.
				// While we need cluster-ingress-operator to delete dependencies,
				// cloud-ingress will take care of the final IngressController delete.
				if !ctlutils.Contains(ingressController.GetFinalizers(), CloudIngressFinalizer) {
					err = r.addFinalizer(reqLogger, ingressController, CloudIngressFinalizer)
					if err != nil {
						return reconcile.Result{Requeue: true}, err
					}
				}
				// initiate the delete => asking cluster-ingress-operator to delete the dependencies
				if err := r.client.Delete(context.TODO(), ingressController); err != nil {
					reqLogger.Error(err, "Error deleting IngressController")
				}
				return reconcile.Result{Requeue: true}, nil
			}
		} else {
			// Since the default IngressController CR spec does not match the desired IngressController
			// spec that was generated based on the ApplicationIngress, and the fields checked are immutable,
			// the IngressController must be deleted
			reqLogger.Info(fmt.Sprintf("Static Spec does not match for for IngressController %s, deleting", desiredIngressController.Name))
			// TODO: Should we return an error here if this delete fails?
			if err := r.client.Delete(context.TODO(), ingressController); err != nil {
				reqLogger.Error(err, "Error deleting IngressController")
			}
			return reconcile.Result{Requeue: true}, nil
		}
	}
	// do nothing, continue
	return result, err
}

// ensurePatchableSpec patches an IngressController when a patchable field as been changed in the publishingstrategy
func (r *ReconcilePublishingStrategy) ensurePatchableSpec(reqLogger logr.Logger, ingressController, desiredIngressController *ingresscontroller.IngressController) (result reconcile.Result, err error) {
	reqLogger.Info(fmt.Sprintf("Checking Patchable Spec for IngressController %s ", desiredIngressController.Name))
	// All the remaining fields are mutable and don't require a deletion of the IngresscController
	// If any of the fields are differet, that field will be patched
	if valid, field := validatePatchableSpec(*ingressController, desiredIngressController.Spec); !valid {
		// Generates the base CR to patch against
		baseToPatch := client.MergeFrom(ingressController.DeepCopy())
		// "The default" CR also needs a status check as the config isn't always guaranteed to be in the spec
		if desiredIngressController.Name == "default" && field == IngressControllerSelector {
			reqLogger.Info("Patchable Spec does not match for default IngressController, checking Status")
			// "The default" CR also needs a status check as the config isn't always guaranteed to be in the spec
			if valid, _ := validatePatchableStatus(*ingressController, desiredIngressController.Spec); !valid {
				// Check Status can only return RouteSelector or nil
				// If the RouteSelector doesn't match, replace the existing spec with the desired Spec
				reqLogger.Info("Patchable Spec and Status do not match for default IngressController, patching RouteSelector")
				ingressController.Spec.RouteSelector = desiredIngressController.Spec.RouteSelector
				// Perform the patch on the existing IngressController using the base to patch against and the
				// changes added to bring the exsting CR to the desired state
				err = r.client.Patch(context.TODO(), ingressController, baseToPatch)
				if err != nil {
					return reconcile.Result{}, err
				}
				return reconcile.Result{Requeue: true}, nil
			}
		} else {
			reqLogger.Info(fmt.Sprintf("Patchable Spec does not match for IngressController %s, patching field %s", desiredIngressController.Name, field))
			// Any other IngressController that is not default needs to be patched if the spec doesn't match
			// Only patch the field that doesn't match
			if field == IngressControllerSelector {
				// If the RouteSelector doesn't match, replace the existing spec with the desired Spec
				ingressController.Spec.RouteSelector = desiredIngressController.Spec.RouteSelector
			} else if field == IngressControllerCertificate {
				// If the DefaultCertificate doesn't match, replace the existing spec with the desired Spec
				ingressController.Spec.DefaultCertificate = desiredIngressController.Spec.DefaultCertificate
			} else if field == IngressControllerNodePlacement {
				// If the NodePlacement doesn't match, replace the existing spec with the desired Spec
				ingressController.Spec.NodePlacement = desiredIngressController.Spec.NodePlacement
			} else if field == IngressControllerEndPoint {
				// If the EndpointPublishingStrategy doesn't match, replace the existing spec with the desired Spec
				ingressController.Spec.EndpointPublishingStrategy = desiredIngressController.Spec.EndpointPublishingStrategy
			}

			// Perform the patch on the existing IngressController using the base to patch against and the
			// changes added to bring the exsting CR to the desired state
			err = r.client.Patch(context.TODO(), ingressController, baseToPatch)
			if err != nil {
				return reconcile.Result{}, err
			}
			return reconcile.Result{Requeue: true}, nil
		}
	}
	// do nothing, continue
	return result, err
}
func (r *ReconcilePublishingStrategy) ensureAnnotationsDefined(reqLogger logr.Logger, ownedIngressExistingMap map[string]bool) (result reconcile.Result, err error) {
	// No action if the version is prior 4.10
	if !baseutils.IsVersionHigherThan("4.10") {
		return reconcile.Result{}, nil
	}

	for ingress, inPublishingStrategy := range ownedIngressExistingMap {
		if inPublishingStrategy {

			// Get managed IngressController CRs
			ingressController := &ingresscontroller.IngressController{}
			namespacedName := types.NamespacedName{Name: ingress, Namespace: ingressControllerNamespace}
			if err := r.client.Get(context.TODO(), namespacedName, ingressController); err != nil {
				return reconcile.Result{}, err
			}

			// Check if the annotation exists, return if so
			if _, ok := ingressController.Annotations[IngressControllerDeleteLBAnnotation]; ok {
				return reconcile.Result{}, nil
			}

			// Generate Annotation and apply to object
			baseToPatch := client.MergeFrom(ingressController.DeepCopy())
			annotations := map[string]string{
				"Owner":                             "cloud-ingress-operator",
				IngressControllerDeleteLBAnnotation: "",
			}
			ingressController.Annotations = annotations

			// Patch
			reqLogger.Info(fmt.Sprintf("IngressController's CR of %s is being patched for the missing annotation: %s", ingress, IngressControllerDeleteLBAnnotation))
			if err := r.client.Patch(context.TODO(), ingressController, baseToPatch); err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	return reconcile.Result{}, nil
}

// ensureAliasScope updates the loadbalancer to match the scope of the ingress in the publishingstrategy
func (r *ReconcilePublishingStrategy) ensureAliasScope(reqLogger logr.Logger, instance *v1alpha1.PublishingStrategy, clusterBaseDomain string) (result reconcile.Result, err error) {
	cloudPlatform, err := baseutils.GetPlatformType(r.client)
	if err != nil {
		log.Error(err, "Failed to create a Cloud Client")
		return reconcile.Result{}, err
	}
	cloudClient := cloudclient.GetClientFor(r.client, *cloudPlatform)

	if instance.Spec.DefaultAPIServerIngress.Listening == v1alpha1.Internal {
		err := cloudClient.SetDefaultAPIPrivate(context.TODO(), r.client, instance)
		if err != nil {
			log.Error(err, fmt.Sprintf("Error updating api.%s alias to internal NLB", clusterBaseDomain))
			return reconcile.Result{}, err
		}
		log.Info(fmt.Sprintf("Update api.%s alias to internal NLB successful", clusterBaseDomain))
		return reconcile.Result{}, nil
	}

	// if CR is wanted the default server API to be internet-facing, we
	// create the external NLB for port 6443/TCP and add api.<cluster-name> DNS record to point to external NLB
	if instance.Spec.DefaultAPIServerIngress.Listening == v1alpha1.External {
		err = cloudClient.SetDefaultAPIPublic(context.TODO(), r.client, instance)
		if err != nil {
			log.Error(err, fmt.Sprintf("Error updating api.%s alias to external NLB", clusterBaseDomain))
			return reconcile.Result{}, err
		}
		log.Info(fmt.Sprintf("Update api.%s alias to external NLB successful", clusterBaseDomain))
		return reconcile.Result{}, nil
	}
	return result, err
}

// ensureIngressController makes sure that an IngressController being deleted, gets recreated by cloud-ingress-operator, instead of cluster-ingress-operator
func (r *ReconcilePublishingStrategy) ensureIngressController(reqLogger logr.Logger, ingressController, desiredIngressController *ingresscontroller.IngressController) (reconcile.Result, error) {
	// If ingresscontroller still has the ClusterIngressFinalizer, there is no point continuing.
	// Cluster-ingress-operator typically needs a few minutes to delete all dependencies
	if ctlutils.Contains(ingressController.GetFinalizers(), ClusterIngressFinalizer) {
		reqLogger.Info(fmt.Sprintf("%s IngressController's  is in the process of being deleted, requeing", ingressController.Name))
		return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
	}

	// At this point, ClusterIngressFinalizer is gone, meaning cluster-ingress-operator has completed dependency cleanup
	// so we can proceed with deleting the IngressController
	if ctlutils.Contains(ingressController.GetFinalizers(), CloudIngressFinalizer) {
		// First remove the CloudIngressFinalizer, to allow it to be deleted
		if err := r.removeFinalizer(reqLogger, ingressController, CloudIngressFinalizer); err != nil {
			return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
		}
	}

	// Make sure there is no other finalizer preventing the delete
	if len(ingressController.GetFinalizers()) != 0 {
		reqLogger.Info(fmt.Sprintf("IngressController %s  still has the following Finalizer(s) %v , requeing", ingressController.Name, ingressController.GetFinalizers()))
		return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
	}

	// At this point, if the IngressController still exists, and has no Finalizer
	// Therefore, it is ready to be deleted
	if err := r.client.Delete(context.TODO(), ingressController); err != nil {
		if k8serr.IsNotFound(err) {
			// It is possible that cluster-ingress-operator might be faster to delete the IngressController
			// If that's the case, we proceed
			reqLogger.Info("IngressController already deleted")
		} else {
			reqLogger.Error(err, "Error deleting IngressController")
			return reconcile.Result{Requeue: true}, err
		}
	}

	// At this point, the IngressController doesn't exist anymore
	// Create the desiredIngressController (hopefully before cluster-ingress-operator did)
	reqLogger.Info(fmt.Sprintf("Create IngressController %s", ingressController.Name))
	if err := r.client.Create(context.TODO(), desiredIngressController); err != nil {
		reqLogger.Error(err, "Error creating the IngressController")
		return reconcile.Result{Requeue: true}, err
	}
	// If the CR was created, requeue PublishingStrategy
	return reconcile.Result{Requeue: true}, nil

}
