/*
Copyright 2022.

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

package publishingstrategy

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/openshift/cloud-ingress-operator/pkg/cloudclient"

	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/cloud-ingress-operator/api/v1alpha1"
	localctlutils "github.com/openshift/cloud-ingress-operator/pkg/controllerutils"
	"github.com/openshift/cloud-ingress-operator/pkg/ingresscontroller"
	baseutils "github.com/openshift/cloud-ingress-operator/pkg/utils"
)

const (
	ingressControllerNamespace = "openshift-ingress-operator"
	infraNodeLabelKey          = "node-role.kubernetes.io/infra"
	CloudIngressFinalizer      = "cloudingress.managed.openshift.io/finalizer-cloud-ingress-controller"
	ClusterIngressFinalizer    = "ingresscontroller.operator.openshift.io/finalizer-ingresscontroller"
	ELBIdleTimeoutDuration     = 1800
)

var log = logf.Log.WithName("controller_publishingstrategy")

type patchField string

var IngressControllerSelector patchField = "IngressControllerSelector"
var IngressControllerCertificate patchField = "IngressControllerCertificate"
var IngressControllerNodePlacement patchField = "IngressControllerNodePlacement"
var IngressControllerEndPoint patchField = "IngressControllerEndpoint"
var IngressControllerDeleteLBAnnotation string = "ingress.operator.openshift.io/auto-delete-load-balancer"
var IngressControllerELBIdleTimeout metav1.Duration = metav1.Duration{Duration: ELBIdleTimeoutDuration * time.Second}

var _ reconcile.Reconciler = &PublishingStrategyReconciler{}

// PublishingStrategyReconciler reconciles a PublishingStrategy object
type PublishingStrategyReconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.2/pkg/reconcile

// Reconcile reads that state of the cluster for a PublishingStrategy object and makes changes based on the state read
// and what is in the PublishingStrategy.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *PublishingStrategyReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling PublishingStrategy")

	// Fetch the PublishingStrategy instance
	instance := &v1alpha1.PublishingStrategy{}
	err := r.Client.Get(context.TODO(), request.NamespacedName, instance)
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

	// Retrieve the cluster base domain. Discard the error since it's just for logging messages.
	// In case of failure, clusterBaseDomain is an empty string.
	clusterBaseDomain, _ := baseutils.GetClusterBaseDomain(r.Client)

	// Get all IngressControllers on cluster with an annotation that indicates cloud-ingress-operator owns it
	ingressControllerList := &ingresscontroller.IngressControllerList{}
	listOptions := []client.ListOption{
		client.InNamespace("openshift-ingress-operator"),
	}
	err = r.Client.List(context.TODO(), ingressControllerList, listOptions...)
	if err != nil {
		log.Error(err, "Cannot get list of ingresscontroller")
		return reconcile.Result{}, err
	}

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
		// Initialize them all to false as they have not been verified against ApplicationIngress list
		ownedIngressExistingMap[ownedIngress.Name] = false
	}

	result, err := ensureNoNewSecondIngressCreated(reqLogger, instance.Spec.ApplicationIngress, ownedIngressExistingMap)
	if err != nil || result.Requeue {
		return result, err
	}

	// New native ingress managed feature, remove all cloud ingress annotations from items in cluster if >4.13 and feature flag is enabled.
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
	Once the immutable fields are checked, then then mutable fields are. If any of the mutable fields
	differ, the IngressController is patched.
	The default IngressController is special. When its first created, the spec is not filled out.
	Instead, the relevant fields are set in the status. For each of the spec checks, the status
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

		cloudPlatform, err := baseutils.GetPlatformType(r.Client)
		if err != nil {
			return reconcile.Result{}, err
		}
		isAWS := *cloudPlatform == "AWS"
		// Add ProviderParameters if the cloud is AWS to ensure LB type matches
		if isAWS {

			// Default to Classic LB to match default IngressController behavior
			if ingressDefinition.Type == "" {
				ingressDefinition.Type = "Classic"
			}

			desiredIngressController.Spec.EndpointPublishingStrategy.LoadBalancer.ProviderParameters = &ingresscontroller.ProviderLoadBalancerParameters{
				Type: ingresscontroller.AWSLoadBalancerProvider,
				AWS: &ingresscontroller.AWSLoadBalancerParameters{
					Type: ingresscontroller.AWSLoadBalancerType(ingressDefinition.Type),
				},
			}

			// For Classic LB in v4.11+, set the ELB idle connection timeout on the IngressController
			if ingressDefinition.Type == "Classic" && baseutils.IsVersionHigherThan("4.11") {
				desiredIngressController.Spec.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS.ClassicLoadBalancerParameters = &ingresscontroller.AWSClassicLoadBalancerParameters{
					ConnectionIdleTimeout: IngressControllerELBIdleTimeout,
				}
			}
		}

		// Attempt to find the IngressController referenced by the ApplicationIngress
		// by doing a GET of the namespaced name object build above against the k8s api.
		ingressController := &ingresscontroller.IngressController{}
		err = r.Client.Get(context.TODO(), namespacedName, ingressController)
		if err != nil {
			// Attempt to create the CR if not found
			if k8serr.IsNotFound(err) {
				reqLogger.Info(fmt.Sprintf("ApplicationIngress %s not found, attempting to create", ingressName))
				err = r.Client.Create(context.TODO(), desiredIngressController)
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
			reqLogger.Info("Cluster is AWS, checking load balancers")
			result, err := r.ensureAWSLoadBalancerType(reqLogger, ingressController, ingressDefinition)
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

	result, err = r.ensureAnnotationsDefined(reqLogger, ownedIngressExistingMap)
	if err != nil || result.Requeue {
		return result, err
	}

	result, err = r.ensureAliasScope(reqLogger, instance, clusterBaseDomain)
	if err != nil || result.Requeue {
		return result, err
	}

	// If the OCM API sends an empty applicationIngress array, and we are on a version greater than 4.13, assume that
	// we want to 'disown' the native ingress controller. Any remaining ingresses will be deleted as per usual.
	// We also ensure that the scope of the default API server ingress matches the scope of the publishing strategy CR.
	if baseutils.IsVersionHigherThan("4.13") && !ownedIngressExistingMap["default"] {
		reqLogger.Info("applicationIngress is empty, removing cloud-ingress-operator ownership over default ingress. See https://github.com/openshift/cloud-ingress-operator/README.md#publishingstrategyapplicationingress-deprecation for further information.")
		result, err := r.ensureDefaultICOwnedByClusterIngressOperator(reqLogger)
		if err != nil || result.Requeue {
			return result, err
		}

		return result, nil
	}

	result, err = r.deleteUnpublishedIngressControllers(ownedIngressExistingMap)
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

/*
	Compares the static spec fields of the desired Spec against the existing IngressController's status.

Both the Domain and EndpointPublishingStrategy fields in the IngressController spec are static. This means
that editing them will have no effect. Instead, the CR must be fully deleted and recreated with the desired
Domain and EndpointPublishingStrategy filled in. The default IngressController does not have those spec fields
filled in at all when fisrt created. Instead, the default CRs status holds the correct values. This function
is meant to be used when the default CRs spec is not filled in to see if the desired configuration is present in
its status instead. Returns false if at least one of the existing fields don't match the desired.
*/
func validateStaticStatus(ingressController ingresscontroller.IngressController, desiredSpec ingresscontroller.IngressControllerSpec) bool {

	if desiredSpec.Domain != ingressController.Status.Domain {
		return false
	}
	if !baseutils.IsVersionHigherThan("4.10") {
		// Preventing nil pointer errors
		if ingressController.Status.EndpointPublishingStrategy == nil || ingressController.Status.EndpointPublishingStrategy.LoadBalancer == nil {
			return false
		}

		if desiredSpec.EndpointPublishingStrategy.LoadBalancer.Scope != ingressController.Status.EndpointPublishingStrategy.LoadBalancer.Scope {
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
	if desiredSpec.Domain != ingressController.Spec.Domain {
		return false
	}

	if !baseutils.IsVersionHigherThan("4.10") {
		// Preventing nil pointer errors
		if ingressController.Spec.EndpointPublishingStrategy == nil || ingressController.Spec.EndpointPublishingStrategy.LoadBalancer == nil {
			return false
		}

		if desiredSpec.EndpointPublishingStrategy.LoadBalancer.Scope != ingressController.Spec.EndpointPublishingStrategy.LoadBalancer.Scope {
			return false
		}
	}

	return true
}

/*
		If entry exists in desired application ingress,
	    but NOT in owned ingress, and is not default,
		conclude that the user is trying to create a new 'apps2' ingress.
		Throw an error.
*/
func ensureNoNewSecondIngressCreated(reqLogger logr.Logger, ai []v1alpha1.ApplicationIngress, ownedIngressExistingMap map[string]bool) (result reconcile.Result, err error) {
	for _, ingressDefinition := range ai {
		if ingressDefinition.Default {
			continue
		}

		_, existsOnCluster := ownedIngressExistingMap[getIngressName(ingressDefinition.DNSName)]

		if !existsOnCluster {
			err := errors.New("reconciling second ingress controllers using PublishingStrategy is no longer supported. If you have existing second ingress controllers, these can still be updated and deleted as normal. See https://github.com/openshift/cloud-ingress-operator/README.md#publishingstrategyapplicationingress-deprecation for further information")
			reqLogger.Error(err, fmt.Sprintf("Request to create second ingress %s denied, as customer using new native OCP ingress feature. See https://github.com/openshift/cloud-ingress-operator/README.md#publishingstrategyapplicationingress-deprecation for further information.", getIngressName(ingressDefinition.DNSName)))
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func (r *PublishingStrategyReconciler) ensureAWSLoadBalancerType(reqLogger logr.Logger, ic *ingresscontroller.IngressController, ai v1alpha1.ApplicationIngress) (result reconcile.Result, err error) {

	if !validateAWSLoadBalancerType(*ic, ai) {
		if err := r.Client.Delete(context.TODO(), ic); err != nil {
			reqLogger.Error(err, "Error deleting IngressController")
		}
		return reconcile.Result{Requeue: true}, nil
	}

	return result, nil
}

func validateAWSLoadBalancerType(ic ingresscontroller.IngressController, ai v1alpha1.ApplicationIngress) bool {

	if ic.Spec.EndpointPublishingStrategy == nil {
		if ic.Status.EndpointPublishingStrategy == nil {
			return false
		}

		// The status can also hold this information if its not in the spec
		if ic.Status.EndpointPublishingStrategy.LoadBalancer.ProviderParameters != nil {
			return ingresscontroller.AWSLoadBalancerType(ai.Type) == ic.Status.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS.Type
		}

		// When ProviderParameters is not set, the IngressController defaults to Classic, so we only need to ensure the PublishingStrategy Type is not set to  NLB
		if ai.Type == "NLB" {
			return false
		}

		return true
	}

	// If ProviderParameters are set on the IngressController, then the Type in the PublishingStrategy needs to match exacly
	if ic.Spec.EndpointPublishingStrategy.LoadBalancer.ProviderParameters != nil {
		return ingresscontroller.AWSLoadBalancerType(ai.Type) == ic.Spec.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS.Type
	}

	// If ProviderParameters aren't provided, but the LB config is, the default is Classic
	// The return then relies on if the desired is NLB
	if ic.Spec.EndpointPublishingStrategy.LoadBalancer != nil {
		// If desired is Classic then we're at the desired state and return true
		return ai.Type != "NLB"
	}

	return true

}

/*
	Compares the patchable desired Spec against the existing IngressController's status.

The only patchable field that the ApplicationIngress controls that's also stored in the IngressController's
status is the RouteSelector. The default IngressController does not have those spec fields
filled in at all when first created. Instead, the default CRs status holds the correct values. This function
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

/*
	Compares the patchable desired Spec against the existing IngressController's spec.

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

	if desiredSpec.DefaultCertificate.Name != ingressController.Spec.DefaultCertificate.Name {
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

		if desiredSpec.EndpointPublishingStrategy.LoadBalancer.Scope != ingressController.Spec.EndpointPublishingStrategy.LoadBalancer.Scope {
			return false, IngressControllerEndPoint
		}
	}
	if baseutils.IsVersionHigherThan("4.11") {
		if !(reflect.DeepEqual(desiredSpec.EndpointPublishingStrategy.LoadBalancer.ProviderParameters,
			ingressController.Spec.EndpointPublishingStrategy.LoadBalancer.ProviderParameters)) {
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
func (r *PublishingStrategyReconciler) deleteUnpublishedIngressControllers(ownedIngressExistingMap map[string]bool) (result reconcile.Result, err error) {
	// Delete all IngressControllers that are owned by cloud-ingress-operator but not in PublishingStrategy
	for ingress, inPublishingStrategy := range ownedIngressExistingMap {
		if !inPublishingStrategy {
			// Delete requires an object referece, so we must get it first
			ingressToDelete := &ingresscontroller.IngressController{}
			err = r.Client.Get(context.TODO(), types.NamespacedName{Name: ingress, Namespace: ingressControllerNamespace}, ingressToDelete)
			if err != nil {
				return reconcile.Result{}, err
			}
			err = r.Client.Delete(context.TODO(), ingressToDelete)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	}
	return result, err
}

// ensureStaticSpec deletes or marks an IngressController for deletion when a static spec has been changed in the publishing strategy
func (r *PublishingStrategyReconciler) ensureStaticSpec(reqLogger logr.Logger, ingressController, desiredIngressController *ingresscontroller.IngressController) (result reconcile.Result, err error) {
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
				if !localctlutils.Contains(ingressController.GetFinalizers(), CloudIngressFinalizer) {
					err = r.addFinalizer(reqLogger, ingressController, CloudIngressFinalizer)
					if err != nil {
						return reconcile.Result{Requeue: true}, err
					}
				}
				// initiate the delete => asking cluster-ingress-operator to delete the dependencies
				if err := r.Client.Delete(context.TODO(), ingressController); err != nil {
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
			if err := r.Client.Delete(context.TODO(), ingressController); err != nil {
				reqLogger.Error(err, "Error deleting IngressController")
			}
			return reconcile.Result{Requeue: true}, nil
		}
	}
	// do nothing, continue
	return result, err
}

// ensurePatchableSpec patches an IngressController when a patchable field as been changed in the publishingstrategy
func (r *PublishingStrategyReconciler) ensurePatchableSpec(reqLogger logr.Logger, ingressController, desiredIngressController *ingresscontroller.IngressController) (result reconcile.Result, err error) {
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
				err = r.Client.Patch(context.TODO(), ingressController, baseToPatch)
				if err != nil {
					return reconcile.Result{}, err
				}
				return reconcile.Result{Requeue: true}, nil
			}
		} else {
			reqLogger.Info(fmt.Sprintf("Patchable Spec does not match for IngressController %s, patching field %s", desiredIngressController.Name, field))
			// Any other IngressController that is not default needs to be patched if the spec doesn't match
			// Only patch the field that doesn't match
			switch field {
			case IngressControllerSelector:
				// If the RouteSelector doesn't match, replace the existing spec with the desired Spec
				ingressController.Spec.RouteSelector = desiredIngressController.Spec.RouteSelector
			case IngressControllerCertificate:
				// If the DefaultCertificate doesn't match, replace the existing spec with the desired Spec
				ingressController.Spec.DefaultCertificate = desiredIngressController.Spec.DefaultCertificate
			case IngressControllerNodePlacement:
				// If the NodePlacement doesn't match, replace the existing spec with the desired Spec
				ingressController.Spec.NodePlacement = desiredIngressController.Spec.NodePlacement
			case IngressControllerEndPoint:
				// If the EndpointPublishingStrategy doesn't match, replace the existing spec with the desired Spec
				ingressController.Spec.EndpointPublishingStrategy = desiredIngressController.Spec.EndpointPublishingStrategy
			}

			// Perform the patch on the existing IngressController using the base to patch against and the
			// changes added to bring the exsting CR to the desired state
			err = r.Client.Patch(context.TODO(), ingressController, baseToPatch)
			if err != nil {
				return reconcile.Result{}, err
			}
			return reconcile.Result{Requeue: true}, nil
		}
	}
	// do nothing, continue
	return result, err
}

// Replace cloud ingress operator finalizers and ownership references with the ones assumed by the cluster
// ingress operator.
// Also assume that we return the 'auto-delete-lb' annotation to the cluster ingress operator, too
func (r *PublishingStrategyReconciler) ensureDefaultICOwnedByClusterIngressOperator(reqLogger logr.Logger) (result reconcile.Result, err error) {
	if !baseutils.IsVersionHigherThan("4.13") {
		err := errors.New("cannot disown default ingress controller for versions <4.13")
		return reconcile.Result{}, err
	}

	reqLogger.Info("Cluster using native OCP ingress management, remvoing cloud-ingress-operator ownership of default IngressController")
	ingressController := &ingresscontroller.IngressController{}
	namespacedName := types.NamespacedName{Name: "default", Namespace: ingressControllerNamespace}
	if err := r.Client.Get(context.TODO(), namespacedName, ingressController); err != nil {
		return reconcile.Result{}, err
	}

	baseToPatch := client.MergeFrom(ingressController.DeepCopy())
	annotations := map[string]string{
		"Owner":                             "cluster-ingress-operator",
		IngressControllerDeleteLBAnnotation: "true",
	}
	ingressController.Annotations = annotations
	ingressController.Finalizers = []string{ClusterIngressFinalizer}

	reqLogger.Info("IngressController default is being disowned by cloud-ingress-operator")
	reqLogger.Info("IngressController default is being given cluster ingress finalizer")
	if err := r.Client.Patch(context.TODO(), ingressController, baseToPatch); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *PublishingStrategyReconciler) ensureAnnotationsDefined(reqLogger logr.Logger, ownedIngressExistingMap map[string]bool) (result reconcile.Result, err error) {
	// No action if the version is prior 4.10
	// No action if using new Hive ingress feature
	for ingress, inPublishingStrategy := range ownedIngressExistingMap {
		if inPublishingStrategy {

			// Get managed IngressController CRs
			ingressController := &ingresscontroller.IngressController{}
			namespacedName := types.NamespacedName{Name: ingress, Namespace: ingressControllerNamespace}
			if err := r.Client.Get(context.TODO(), namespacedName, ingressController); err != nil {
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
			reqLogger.Info(fmt.Sprintf("IngressController CR of %s is being patched for the missing annotation: %s", ingress, IngressControllerDeleteLBAnnotation))
			if err := r.Client.Patch(context.TODO(), ingressController, baseToPatch); err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	return reconcile.Result{}, nil
}

// ensureAliasScope updates the loadbalancer to match the scope of the ingress in the publishingstrategy
func (r *PublishingStrategyReconciler) ensureAliasScope(reqLogger logr.Logger, instance *v1alpha1.PublishingStrategy, clusterBaseDomain string) (result reconcile.Result, err error) {

	cloudPlatform, err := baseutils.GetPlatformType(r.Client)
	if err != nil {
		log.Error(err, "Failed to create a Cloud Client")
		return reconcile.Result{}, err
	}
	cloudClient := cloudclient.GetClientFor(r.Client, *cloudPlatform)

	if instance.Spec.DefaultAPIServerIngress.Listening == v1alpha1.Internal {
		err := cloudClient.SetDefaultAPIPrivate(context.TODO(), r.Client, instance)
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
		err = cloudClient.SetDefaultAPIPublic(context.TODO(), r.Client, instance)
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
func (r *PublishingStrategyReconciler) ensureIngressController(reqLogger logr.Logger, ingressController, desiredIngressController *ingresscontroller.IngressController) (reconcile.Result, error) {
	// If ingresscontroller still has the ClusterIngressFinalizer, there is no point continuing.
	// Cluster-ingress-operator typically needs a few minutes to delete all dependencies
	if localctlutils.Contains(ingressController.GetFinalizers(), ClusterIngressFinalizer) {
		reqLogger.Info(fmt.Sprintf("%s IngressController is in the process of being deleted, requeing", ingressController.Name))
		return reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
	}

	// At this point, ClusterIngressFinalizer is gone, meaning cluster-ingress-operator has completed dependency cleanup
	// so we can proceed with deleting the IngressController
	if localctlutils.Contains(ingressController.GetFinalizers(), CloudIngressFinalizer) {
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
	if err := r.Client.Delete(context.TODO(), ingressController); err != nil {
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
	if err := r.Client.Create(context.TODO(), desiredIngressController); err != nil {
		reqLogger.Error(err, "Error creating the IngressController")
		return reconcile.Result{Requeue: true}, err
	}
	// If the CR was created, requeue PublishingStrategy
	return reconcile.Result{Requeue: true}, nil

}

// SetupWithManager sets up the controller with the Manager.
func (r *PublishingStrategyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.PublishingStrategy{}).
		Complete(r)
}
