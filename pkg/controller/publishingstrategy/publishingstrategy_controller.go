package publishingstrategy

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	"github.com/openshift/cloud-ingress-operator/pkg/cloudclient"
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
)

const (
	ingressControllerNamespace = "openshift-ingress-operator"
)

var log = logf.Log.WithName("controller_publishingstrategy")

type patchField string

var IngressControllerSelector patchField = "IngressControllerSelector"
var IngressControllerCertificate patchField = "IngressControllerCertificate"

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
	ingressControllerList := &operatorv1.IngressControllerList{}
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

		// Attempt to find the IngressController referenced by the ApplicationIngress
		// by doing a GET of the namespaced name object build above against the k8s api.
		ingressController := &operatorv1.IngressController{}
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

		/* When an ingresscontroller is being deleted, it takes time as it needs to delete several
		services (ie the load balancer service has finalizers for the cloud provider resource cleanup)/
		If ingresscontroller still has these finalizers, there is no point in trying to re-create it. This
		check ensures ingresscontroller is deleted before we try and create so we don't fill up cluster-ingress-operator
		logs with a bunch of error messages
		*/
		if !ingressController.DeletionTimestamp.IsZero() {
			reqLogger.Info(fmt.Sprintf("IngressController %s is in the process of being deleted, requeing", ingressName))
			return reconcile.Result{Requeue: true}, nil
		}

		reqLogger.Info(fmt.Sprintf("Checking Static Spec for IngressController %s ", ingressName))
		// Compare the Spec fields that cannot be patched in the desired IngressController and the actual IngressController
		if !validateStaticSpec(*ingressController, desiredIngressController.Spec) {
			// "The default" CR also needs a status check as the config isn't always guaranteed to be in the spec
			if ingressDefinition.Default {
				reqLogger.Info("Static Spec does not match for default IngressController, checking Status")
				if !validateStaticStatus(*ingressController, desiredIngressController.Spec) {
					// Since the default IngressController CR spec + status does not match the desired IngressController
					// spec that was generated based on the ApplicationIngress, and the fields checked are immutable,
					// the actual default IngressController must be deleted
					reqLogger.Info("Static Spec and Status do not match for default IngressController, deleting")
					// TODO: Should we return an error here if this delete fails?
					if err := r.client.Delete(context.TODO(), ingressController); err != nil {
						reqLogger.Error(err, "Error deleting IngressController")
					}
					return reconcile.Result{Requeue: true}, nil
				}
			} else {
				// Since the default IngressController CR spec does not match the desired IngressController
				// spec that was generated based on the ApplicationIngress, and the fields checked are immutable,
				// the IngressController must be deleted
				reqLogger.Info(fmt.Sprintf("Static Spec does not match for for IngressController %s, deleting", ingressName))
				// TODO: Should we return an error here if this delete fails?
				if err := r.client.Delete(context.TODO(), ingressController); err != nil {
					reqLogger.Error(err, "Error deleting IngressController")
				}
				return reconcile.Result{Requeue: true}, nil
			}
		}

		reqLogger.Info(fmt.Sprintf("Checking Patchable Spec for IngressController %s ", ingressName))
		// All the remaining fields are mutable and don't require a deletion of the IngresscController
		// If any of the fields are differet, that field will be patched
		if valid, field := validatePatchableSpec(*ingressController, desiredIngressController.Spec); !valid {
			// Generates the base CR to patch against
			baseToPatch := client.MergeFrom(ingressController.DeepCopy())
			// "The default" CR also needs a status check as the config isn't always guaranteed to be in the spec
			if ingressDefinition.Default && field == IngressControllerSelector {
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
				reqLogger.Info(fmt.Sprintf("Patchable Spec does not match for IngressController %s, patching field %s", ingressName, field))
				// Any other IngressController that is not default needs to be patched if the spec doesn't match
				// Only patch the field that doesn't match
				if field == IngressControllerSelector {
					// If the RouteSelector doesn't match, replace the existing spec with the desired Spec
					ingressController.Spec.RouteSelector = desiredIngressController.Spec.RouteSelector
				} else if field == IngressControllerCertificate {
					// If the DefaultCertificate doesn't match, replace the existing spec with the desired Spec
					ingressController.Spec.DefaultCertificate = desiredIngressController.Spec.DefaultCertificate
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
	}

	// Delete all IngressControllers that are owned by cloud-ingress-operator but not in PublishingStrategy
	for ingress, inPublishingStrategy := range ownedIngressExistingMap {
		if !inPublishingStrategy {
			// Delete requires an object referece, so we must get it first
			ingressToDelete := &operatorv1.IngressController{}
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

	cloudPlatform, err := baseutils.GetPlatformType(r.client)
	if err != nil {
		log.Error(err, "Failed to create a Cloud Client")
		return reconcile.Result{}, err
	}
	cloudClient := cloudclient.GetClientFor(r.client, *cloudPlatform)

	if instance.Spec.DefaultAPIServerIngress.Listening == cloudingressv1alpha1.Internal {
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
	if instance.Spec.DefaultAPIServerIngress.Listening == cloudingressv1alpha1.External {
		err := cloudClient.SetDefaultAPIPublic(context.TODO(), r.client, instance)
		if err != nil {
			log.Error(err, fmt.Sprintf("Error updating api.%s alias to external NLB", clusterBaseDomain))
			return reconcile.Result{}, err
		}
		log.Info(fmt.Sprintf("Update api.%s alias to external NLB successful", clusterBaseDomain))
		return reconcile.Result{}, nil
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
func generateIngressController(appIngress v1alpha1.ApplicationIngress) *operatorv1.IngressController {
	// Translate the ApplicationIngress listening string into the matching type for the IngressController
	loadBalancerScope := operatorv1.LoadBalancerScope("")
	switch appIngress.Listening {
	case "internal":
		loadBalancerScope = operatorv1.InternalLoadBalancer
	case "external":
		loadBalancerScope = operatorv1.ExternalLoadBalancer
	default:
		loadBalancerScope = operatorv1.ExternalLoadBalancer
	}

	ingressName := getIngressName(appIngress.DNSName)
	if appIngress.Default {
		ingressName = "default"
	}

	// Builds the IngressController CR object based on the ApplicationIngress
	return &operatorv1.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressName,
			Namespace: ingressControllerNamespace,
			Annotations: map[string]string{
				"Owner": "cloud-ingress-operator",
			},
		},
		Spec: operatorv1.IngressControllerSpec{
			DefaultCertificate: &corev1.LocalObjectReference{
				Name: appIngress.Certificate.Name,
			},
			Domain: appIngress.DNSName,
			EndpointPublishingStrategy: &operatorv1.EndpointPublishingStrategy{
				Type: operatorv1.LoadBalancerServiceStrategyType,
				LoadBalancer: &operatorv1.LoadBalancerStrategy{
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
func validateStaticStatus(ingressController operatorv1.IngressController, desiredSpec operatorv1.IngressControllerSpec) bool {

	if !(desiredSpec.Domain == ingressController.Status.Domain) {
		return false
	}

	// Preventing nil pointer errors
	if ingressController.Status.EndpointPublishingStrategy == nil || ingressController.Status.EndpointPublishingStrategy.LoadBalancer == nil {
		return false
	}

	if !(desiredSpec.EndpointPublishingStrategy.LoadBalancer.Scope == ingressController.Status.EndpointPublishingStrategy.LoadBalancer.Scope) {
		return false
	}

	return true
}

/* Compares the static spec fields of the desired Spec against the existing IngressController's spec.
Both the Domain and EndpointPublishingStrategy fields in the IngressController spec are static. This means
that editing them will have no effect. Instead, the CR must be fully deleted and recreated with the desired
Domain and EndpointPublishingStrategy filled in. Returns false if at least one of the existing fields don't match the desired
*/

func validateStaticSpec(ingressController operatorv1.IngressController, desiredSpec operatorv1.IngressControllerSpec) bool {
	if !(desiredSpec.Domain == ingressController.Spec.Domain) {
		return false
	}

	// Preventing nil pointer errors
	if ingressController.Spec.EndpointPublishingStrategy == nil || ingressController.Spec.EndpointPublishingStrategy.LoadBalancer == nil {
		return false
	}

	if !(desiredSpec.EndpointPublishingStrategy.LoadBalancer.Scope == ingressController.Spec.EndpointPublishingStrategy.LoadBalancer.Scope) {
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
func validatePatchableStatus(ingressController operatorv1.IngressController, desiredSpec operatorv1.IngressControllerSpec) (bool, patchField) {
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
func validatePatchableSpec(ingressController operatorv1.IngressController, desiredSpec operatorv1.IngressControllerSpec) (bool, patchField) {

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

	return true, ""
}

// Given an IngressControllerList, returns only IngressControllers with the cloud-ingress-operator Owner annotation
func getIngressWithCloudIngressOpreatorOwnerAnnotation(ingressList operatorv1.IngressControllerList) *operatorv1.IngressControllerList {

	ownedIngressList := &operatorv1.IngressControllerList{}

	for _, ingress := range ingressList.Items {
		if _, ok := ingress.Annotations["Owner"]; ok {
			if ingress.Annotations["Owner"] == "cloud-ingress-operator" {
				ownedIngressList.Items = append(ownedIngressList.Items, ingress)
			}
		}
	}

	return ownedIngressList
}
