package publishingstrategy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	operatorv1 "github.com/openshift/api/operator/v1"
	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	"github.com/openshift/cloud-ingress-operator/pkg/cloudclient"
	baseutils "github.com/openshift/cloud-ingress-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	defaultIngressName         = "default"
	ingressControllerNamespace = "openshift-ingress-operator"
)

var log = logf.Log.WithName("controller_publishingstrategy")
var serializer = json.NewSerializerWithOptions(nil, nil, nil, json.SerializerOptions{})

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
		if k8serr.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// get a list of all ingress on the cluster
	ingressControllerList := &operatorv1.IngressControllerList{}
	listOptions := []client.ListOption{
		client.InNamespace("openshift-ingress-operator"),
	}
	err = r.client.List(context.TODO(), ingressControllerList, listOptions...)
	if err != nil {
		log.Error(err, "Cannot get list of ingresscontroller")
		return reconcile.Result{}, err
	}

	// delete non-default with proper annotation if not publishingStratagy CR
	err = r.deleteIngressWithAnnotation(instance.Spec.ApplicationIngress, ingressControllerList)
	if err != nil {
		log.Error(err, "Cannot delete ingresscontroller with annotation")
		return reconcile.Result{}, err
	}

	// create list of applicationIngress
	var ingressNotOnCluster []cloudingressv1alpha1.ApplicationIngress

	exisitingIngressMap := convertIngressControllerToMap(ingressControllerList.Items)

	// loop through every applicationingress in publishing strategy
	for _, publishingStrategyIngress := range instance.Spec.ApplicationIngress {
		if !checkExistingIngress(exisitingIngressMap, &publishingStrategyIngress) {
			ingressNotOnCluster = append(ingressNotOnCluster, publishingStrategyIngress)
		}
	}

	// ----------------------------TODO: remove these debug logs afterwards --------------------------------------
	var serializedIngressControllerList bytes.Buffer
	_ = serializer.Encode(ingressControllerList, &serializedIngressControllerList)
	log.Info(fmt.Sprintf("initial ingresscontroller list: %s", serializedIngressControllerList.String()))
	log.Info(fmt.Sprintf("appingress on publishingstrategy CR: %+v", instance.Spec.ApplicationIngress))
	log.Info(fmt.Sprintf("ingress to reconcile: %+v", ingressNotOnCluster))
	// -----------------------------------------------------------------------------------------------------------

	for _, appingress := range ingressNotOnCluster {
		newCertificate := &corev1.LocalObjectReference{
			Name: appingress.Certificate.Name,
		}

		if appingress.Default == true {
			err := r.defaultIngressHandle(appingress, ingressControllerList, newCertificate)
			if err != nil {
				log.Error(err, fmt.Sprintf("failed to handle default ingresscontroller %v", appingress))
				return reconcile.Result{}, err
			}
			continue
		}
		err := r.nonDefaultIngressHandle(appingress, ingressControllerList, newCertificate)
		if err != nil {
			log.Error(err, fmt.Sprintf("failed to handle non-default ingresscontroller %v", appingress))
		}
	}

	ingressList := &operatorv1.IngressControllerList{}
	listOptions = []client.ListOption{
		client.InNamespace("openshift-ingress-operator"),
	}
	err = r.client.List(context.TODO(), ingressList, listOptions...)
	if err != nil {
		log.Error(err, "Cannot get list of ingresscontroller")
		return reconcile.Result{}, err
	}

	// since we don't own ingresscontroller object, adding this check ensures that the proper ingresscontrollers have been created by cluster-ingress-operator
	// if check fails, requeue the reconcile and try to re-create the ingresscontrollers until successful
	if !r.ensureIngressControllersExist(instance.Spec.ApplicationIngress, ingressList) {
		reqLogger.Info("IngressController does not match PublishingStrategy. Requeue and try again")
		return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, nil
	}

	cloudPlatform, err := baseutils.GetPlatformType(r.client)
	if err != nil {
		log.Error(err, "Failed to create a Cloud Client")
		return reconcile.Result{}, err
	}
	cloudClient := cloudclient.GetClientFor(r.client, *cloudPlatform)

	// Discard the error since it's just for logging messages.
	// In case of failure, clusterBaseDomain is an empty string.
	clusterBaseDomain, _ := baseutils.GetClusterBaseDomain(r.client)

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
			log.Error(err, fmt.Sprintf("Error updating api.%s alias to internal NLB", clusterBaseDomain))
			return reconcile.Result{}, err
		}
		log.Info(fmt.Sprintf("Update api.%s alias to internal NLB successful", clusterBaseDomain))
		return reconcile.Result{}, nil
	}
	return reconcile.Result{}, nil
}

// ensure ingresscontrollers on publishingstrategy CR are present on the cluster
func (r *ReconcilePublishingStrategy) ensureIngressControllersExist(appIngressList []cloudingressv1alpha1.ApplicationIngress, ingressControllerList *operatorv1.IngressControllerList) bool {

	for _, appIngress := range appIngressList {

		if !doesIngressControllerExist(appIngress, ingressControllerList) {
			return false
		}
	}

	return true
}

func doesIngressControllerExist(appIngress cloudingressv1alpha1.ApplicationIngress, ingressControllerList *operatorv1.IngressControllerList) bool {

	for _, ingress := range ingressControllerList.Items {

		// prevent nil pointer error
		if !validateIngress(ingress) {
			return false
		}

		listening := string(appIngress.Listening)
		capListening := strings.Title(strings.ToLower(listening))
		if ingress.Spec.Domain == appIngress.DNSName && capListening == string(ingress.Status.EndpointPublishingStrategy.LoadBalancer.Scope) {
			return true
		}
	}
	return false
}

func validateIngress(ingressController operatorv1.IngressController) bool {
	if ingressController.Spec.Domain == "" ||
		ingressController.Status.EndpointPublishingStrategy == nil ||
		ingressController.Status.EndpointPublishingStrategy.LoadBalancer == nil {
		return false
	}

	return true
}

// get a list of all ingress on cluster that has annotation owner cloud-ingress-operator
// and delete all non-default ingresses
func (r *ReconcilePublishingStrategy) deleteIngressWithAnnotation(appIngressList []cloudingressv1alpha1.ApplicationIngress, ingressControllerList *operatorv1.IngressControllerList) error {
	for _, ingress := range ingressControllerList.Items {
		// if ingress does not have correct annotations, continue to next one
		if _, ok := ingress.Annotations["Owner"]; !ok {
			continue
		}
		// if ingress is default, skip it since we are only looking at non-default ingresses
		if ingress.Name == "default" {
			continue
		}
		// only delete the ingress that does not exist on publishingStrategy
		// true means the ingress (on cluster) exists on the publishingStrategy CR as well, so no deletion
		// false means the ingress (on cluster) do not exist on the CR, so delete to have desired result
		if !contains(appIngressList, &ingress) {
			err := r.client.Delete(context.TODO(), &ingress)
			if err != nil {
				log.Error(err, "Failed to delete ingresscontroller")
				return err
			}
			// wait 60 seconds for deletion to be completed
			log.Info("waited 60 seconds for necessary ingresscontroller deletions")
			time.Sleep(time.Duration(60) * time.Second)
		}
	}
	return nil
}

// contains check if an individual non-default ingress on cluster matches with any non-default applicationingress
// return true if ingress (on cluster) matches with any applicationIngress in PublishingStrategy CR
// return false if ingress (on cluster) DOES NOT match with ANY applicationIngress in PublishingStrategy CR
// this helps to determine if ingress needs to be deleted or not. The end goal is to have all ApplicationIngress on PublishingStrategy CR on cluster
func contains(appIngressList []cloudingressv1alpha1.ApplicationIngress, ingressController *operatorv1.IngressController) bool {
	var isContained bool
	for _, app := range appIngressList {
		log.Info(fmt.Sprintf("app being processed %s", app.DNSName))
		// if the ApplicationIngress (on CR) is default then set bool to false
		// eg. ingresscontroller (on cluster): apps2
		//     appIngressList (in CR) : [default]
		// since apps2 does NOT exist in appIngressList, set bool to false and ready for deletion
		if app.Default == true {
			isContained = false
		}
		// set bool to true if it is non-default and have the proper annotations
		if ingressController.Name != "default" && ingressController.Annotations["Owner"] == "cloud-ingress-operator" {
			if ingressController.Spec.Domain == app.DNSName {
				isContained = true
			}
		}
	}
	return isContained
}

// defaultIngressHandle will delete the existing default ingresscontroller, and create a new one with fields from publishingstrategySpec.ApplicationIngress
func (r *ReconcilePublishingStrategy) defaultIngressHandle(appingress cloudingressv1alpha1.ApplicationIngress, ingressControllerList *operatorv1.IngressControllerList, newCertificate *corev1.LocalObjectReference) error {
	// delete the default appingress on cluster
	for _, ingresscontroller := range ingressControllerList.Items {
		if ingresscontroller.Name == defaultIngressName {
			err := r.client.Delete(context.TODO(), &ingresscontroller)
			if err != nil {
				log.Error(err, "failed to delete existing ingresscontroller")
				return err
			}
		}
	}
	newDefaultIngressController, err := newApplicationIngressControllerCR(defaultIngressName, string(appingress.Listening), appingress.DNSName, newCertificate, appingress.RouteSelector.MatchLabels)
	if err != nil {
		log.Error(err, fmt.Sprintf("failed to generate information for default ingresscontroller with domain %s", appingress.DNSName))
		return err
	}
	err = r.client.Create(context.TODO(), newDefaultIngressController)
	if err != nil {
		if k8serr.IsAlreadyExists(err) {
			for i := 0; i < 60; i++ {
				if i == 60 {
					log.Error(err, "out of retries")
					return err
				}
				time.Sleep(time.Duration(1) * time.Second)

				err = r.client.Create(context.TODO(), newDefaultIngressController)
				if err != nil {
					continue
				}
				// if err not nil then successful
				log.Info(fmt.Sprintf("successfully created default ingresscontroller for %s", newDefaultIngressController.Spec.Domain))
				break
			}
		} else {
			log.Error(err, fmt.Sprintf("failed to create new ingresscontroller with domain %s", appingress.DNSName))
			return err
		}
	}
	return nil
}

// nonDefaultIngressHandle will delete the existing non-default ingresscontroller, and create a new one with fields from publishingstrategySpec.ApplicationIngress
func (r *ReconcilePublishingStrategy) nonDefaultIngressHandle(appingress cloudingressv1alpha1.ApplicationIngress, ingressControllerList *operatorv1.IngressControllerList, newCertificate *corev1.LocalObjectReference) error {
	newIngressControllerName := getIngressName(appingress.DNSName)
	// if ingress with same name exists on cluster then delete
	for _, ingresscontroller := range ingressControllerList.Items {
		if ingresscontroller.Name == newIngressControllerName {
			err := r.client.Delete(context.TODO(), &ingresscontroller)
			if err != nil {
				log.Error(err, "failed to delete existing ingresscontroller")
				return err
			}
		}
	}

	// create the ingress
	newIngressController, err := newApplicationIngressControllerCR(newIngressControllerName, string(appingress.Listening), appingress.DNSName, newCertificate, appingress.RouteSelector.MatchLabels)
	if err != nil {
		log.Error(err, fmt.Sprintf("failed to generate information for ingresscontroller with domain %s", appingress.DNSName))
	}
	err = r.client.Create(context.TODO(), newIngressController)
	if err != nil {
		if k8serr.IsAlreadyExists(err) {
			for i := 1; i < 24; i++ {
				if i == 24 {
					log.Error(err, "out of retries to create non-default ingress")
				}
				time.Sleep(time.Duration(i) * time.Second)
				err = r.client.Create(context.TODO(), newIngressController)
				if err != nil {
					continue
				}
				log.Info(fmt.Sprintf("successfully created non-default ingresscontroller for %s", newIngressController.Spec.Domain))
				break
			}
		} else {
			log.Error(err, fmt.Sprintf("got error trying to create %s", newIngressController.GetName()))
			return err
		}
	}
	return nil
}

// getIngressName takes the domain name and returns the first part
func getIngressName(dnsName string) string {
	firstPeriodIndex := strings.Index(dnsName, ".")
	newIngressName := dnsName[:firstPeriodIndex]
	return newIngressName
}

// newApplicationIngressControllerCR creates a new IngressController CR
func newApplicationIngressControllerCR(ingressControllerCRName, scope, dnsName string, certificate *corev1.LocalObjectReference, matchLabels map[string]string) (*operatorv1.IngressController, error) {
	loadBalancerScope := operatorv1.LoadBalancerScope("")
	switch scope {
	case "internal":
		loadBalancerScope = operatorv1.InternalLoadBalancer
	case "external":
		loadBalancerScope = operatorv1.ExternalLoadBalancer
	default:
		return &operatorv1.IngressController{}, errors.New("ErrCreatingIngressController")
	}

	return &operatorv1.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressControllerCRName,
			Namespace: ingressControllerNamespace,
			Annotations: map[string]string{
				"Owner": "cloud-ingress-operator",
			},
		},
		Spec: operatorv1.IngressControllerSpec{
			DefaultCertificate: certificate,
			Domain:             dnsName,
			EndpointPublishingStrategy: &operatorv1.EndpointPublishingStrategy{
				Type: operatorv1.LoadBalancerServiceStrategyType,
				LoadBalancer: &operatorv1.LoadBalancerStrategy{
					Scope: loadBalancerScope,
				},
			},
			RouteSelector: &metav1.LabelSelector{
				MatchLabels: matchLabels,
			},
		},
	}, nil
}

// convertIngressControllerToMap takes in on cluster ingresscontroller list and returns them as a map with key Spec.Domain and value operatorv1.IngressController
func convertIngressControllerToMap(existingIngress []operatorv1.IngressController) map[string]operatorv1.IngressController {
	ingressMap := make(map[string]operatorv1.IngressController)

	for _, ingress := range existingIngress {
		ingressMap[ingress.Spec.Domain] = ingress
	}
	return ingressMap
}

// checkExistingIngress returns false if applicationIngress do not match any existing ingresscontroller on cluster
func checkExistingIngress(existingMap map[string]operatorv1.IngressController, publishingStrategyIngress *cloudingressv1alpha1.ApplicationIngress) bool {
	if _, ok := existingMap[publishingStrategyIngress.DNSName]; !ok {
		log.Info(fmt.Sprintf("IngressController for %q not found", publishingStrategyIngress.DNSName))
		return false
	}
	if !isOnCluster(publishingStrategyIngress, existingMap[publishingStrategyIngress.DNSName]) {
		return false
	}
	return true
}

// doesIngressMatch checks if application ingress in PublishingStrategy CR matches with IngressController CR
func isOnCluster(publishingStrategyIngress *cloudingressv1alpha1.ApplicationIngress, ingressController operatorv1.IngressController) bool {
	if publishingStrategyIngress.DNSName != ingressController.Spec.Domain {
		log.Info("ApplicationIngress.DNSName mismatch",
			"ApplicationIngress", publishingStrategyIngress,
			"IngressController", ingressController)
		return false
	}
	if publishingStrategyIngress.Certificate.Name != ingressController.Spec.DefaultCertificate.Name {
		log.Info("ApplicationIngress.Certificate.Name mismatch",
			"ApplicationIngress", publishingStrategyIngress,
			"IngressController", ingressController)
		return false
	}
	listening := string(publishingStrategyIngress.Listening)
	capListening := strings.Title(strings.ToLower(listening))
	if ingressController.Status.EndpointPublishingStrategy.LoadBalancer == nil && capListening == "Internal" {
		log.Info("ApplicationIngress.Listening mismatch",
			"ApplicationIngress", publishingStrategyIngress,
			"IngressController", ingressController)
		return false
	}
	if ingressController.Status.EndpointPublishingStrategy.LoadBalancer == nil && capListening == "External" {
		return true
	}
	if capListening != string(ingressController.Status.EndpointPublishingStrategy.LoadBalancer.Scope) {
		log.Info("ApplicationIngress.Listening mismatch",
			"ApplicationIngress", publishingStrategyIngress,
			"IngressController", ingressController)
		return false
	}
	if publishingStrategyIngress.RouteSelector.MatchLabels == nil {
		return true
	}
	if (publishingStrategyIngress.RouteSelector.MatchLabels == nil) != (ingressController.Spec.RouteSelector == nil) {
		log.Info("ApplicationIngress.RouteSelector.MatchLabels mismatch",
			"ApplicationIngress", publishingStrategyIngress,
			"IngressController", ingressController)
		return false
	}
	isRouteSelectorEqual := reflect.DeepEqual(ingressController.Spec.RouteSelector.MatchLabels, publishingStrategyIngress.RouteSelector.MatchLabels)
	if !isRouteSelectorEqual {
		log.Info("ApplicationIngress.RouteSelector.MatchLabels mismatch",
			"ApplicationIngress", publishingStrategyIngress,
			"IngressController", ingressController)
		return false
	}
	return true
}
