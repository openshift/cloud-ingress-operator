package publishingstrategy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/awserr"

	operatorv1 "github.com/openshift/api/operator/v1"
	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	"github.com/openshift/cloud-ingress-operator/pkg/awsclient"
	"github.com/openshift/cloud-ingress-operator/pkg/config"
	utils "github.com/openshift/cloud-ingress-operator/pkg/controller/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

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

	// ----------------------------TODO: remove these debug logs afterwards --------------------------------------
	var serializedIngressControllerList bytes.Buffer
	_ = serializer.Encode(ingressControllerList, &serializedIngressControllerList)
	log.Info(fmt.Sprintf("initial ingresscontroller list: %s", serializedIngressControllerList.String()))
	log.Info(fmt.Sprintf("appingress on publishingstrategy CR: %+v", instance.Spec.ApplicationIngress))
	// -----------------------------------------------------------------------------------------------------------

	for _, appingress := range instance.Spec.ApplicationIngress {
		newCertificate := &corev1.LocalObjectReference{
			Name: appingress.Certificate.Name,
		}

		err := r.createOrUpdateIngressController(appingress, newCertificate)
		if err != nil {
			log.Error(err, fmt.Sprintf("failed to create or update ingresscontroller %v", appingress))
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

	masterList, err := utils.GetMasterMachines(r.client)
	if err != nil {
		log.Error(err, "Couldn't fetch list of master nodes")
		return reconcile.Result{}, err
	}

	domainName, err := utils.GetClusterBaseDomain(r.client) // in form of ```samn-test.j5u3.s1.devshift.org```
	if err != nil {
		log.Error(err, "Couldn't obtain the cluster's base domain")
		return reconcile.Result{}, err
	}
	log.Info(fmt.Sprintf("domain name is %s", domainName))

	// append "api" at beginning of domainName and add "." at the end
	apiDNSName := fmt.Sprintf("api.%s.", domainName)

	// In order to update DNS we need the route53 public zone name
	// which happens to be the domainName minus the name of the cluster
	// Since there are NO object on cluster with just clusterName,
	// we will index the first period and parse right
	pubDomainName := domainName[strings.Index(domainName, ".")+1:] // pubDomainName in form of ```j5u3.s1.devshift.org```

	// if CR is wanted the default API server to be internal-facing only, we
	// delete the external NLB for port 6443/TCP and change api.<cluster-domain> DNS record to point to internal NLB
	if instance.Spec.DefaultAPIServerIngress.Listening == cloudingressv1alpha1.Internal {
		loadBalancerInfo, err := awsClient.ListAllNLBs()
		if err != nil {
			log.Error(err, "Error listing all NLBs")
			return reconcile.Result{}, err
		}

		var intDNSName string
		var intHostedZoneID string
		var lbName string
		// delete the external NLB
		for _, loadBalancer := range loadBalancerInfo {
			if loadBalancer.Scheme == "internet-facing" {
				lbName = loadBalancer.LoadBalancerName
				log.Info("Trying to remove external LB", "LB", lbName)
				err = awsClient.DeleteExternalLoadBalancer(loadBalancer.LoadBalancerArn)
				if err != nil {
					log.Error(err, "error deleting external LB")
					return reconcile.Result{}, err
				}
				err := utils.RemoveAWSLBFromMasterMachines(r.client, lbName, masterList)
				if err != nil {
					log.Error(err, "Error removing external LB from master machine objects")
					return reconcile.Result{}, err
				}
				log.Info("Load balancer removed from master machine objects", "LB", lbName)
				log.Info(fmt.Sprintf("external LB %v deleted", loadBalancer.LoadBalancerArn))
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
		comment := "Update api.<clusterName> alias to internal NLB"

		// upsert resource record to change api.<clusterName> from external NLB to internal NLB
		err = awsClient.UpsertARecord(pubDomainName+".", intDNSName, intHostedZoneID, apiDNSName, comment, false)
		if err != nil {
			log.Error(err, "Error updating api.<clusterName> alias to internal NLB")
			return reconcile.Result{}, err
		}
		log.Info(fmt.Sprintf("%s successful", comment))
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
		infrastructureName, err := utils.GetClusterName(r.client)
		if err != nil {
			log.Error(err, "cannot get infrastructure name")
			return reconcile.Result{}, err
		}
		extNLBName := infrastructureName + "-ext"

		// Get both public and private subnet names for master Machines
		// Note: master Machines have only one listed (private one) in their spec, but
		// this returns both public and private. We need the public one.
		subnets, err := utils.GetMasterNodeSubnets(r.client)
		if err != nil {
			log.Error(err, "Couldn't get the subnets used by master nodes")
			return reconcile.Result{}, err
		}
		subnetIDs, err := awsClient.SubnetNameToSubnetIDLookup([]string{subnets["public"]})
		if err != nil {
			log.Error(err, "Couldn't get subnetIDs")
			return reconcile.Result{}, err
		}
		newNLBs, err := awsClient.CreateNetworkLoadBalancer(extNLBName, "internet-facing", subnetIDs[0])
		if err != nil {
			log.Error(err, "couldn't create external NLB")
			return reconcile.Result{}, err
		}
		log.Info(fmt.Sprintf("new external NLB: %v", newNLBs))

		if len(newNLBs) != 1 {
			log.Error(err, "more than one NLB or no NLB detected, but we expect one")
			return reconcile.Result{}, err
		}

		err = awsClient.AddTagsForNLB(newNLBs[0].LoadBalancerArn, infrastructureName)
		if err != nil {
			log.Error(err, "Couldn't add tags for external NLB")
		}

		// ATTEMPT TO USE EXISTING TG
		targetGroupName := fmt.Sprintf("%s-aext", infrastructureName)
		log.Info(targetGroupName)
		targetGroupArn, err := awsClient.GetTargetGroupArn(targetGroupName)
		if err != nil {
			log.Error(err, "cannot get existing targetGroupName")
			return reconcile.Result{}, err
		}

		// create listener for new external NLB
		err = awsClient.CreateListenerForNLB(targetGroupArn, newNLBs[0].LoadBalancerArn)
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				if aerr.Code() == "TargetGroupAssociationLimit" {
					log.Info("another load balancer associated with targetGroup")
					// not possible to modify LB, we'd have to create a new targetGroup
					// return reconcile for now, but need to create new TG later
					return reconcile.Result{}, nil
				}
				return reconcile.Result{}, err
			}
			log.Error(err, "cannot create listerner for new external NLB")
			return reconcile.Result{}, err
		}

		// TODO: HAVE NOT TESTED THIS FUNCTION YET
		// TODO: test when management api is confirmed working
		// upsert resource record to change api.<clusterName> from internal NLB to external NLB
		comment := "Update api.<clusterName> alias to external NLB"
		err = awsClient.UpsertARecord(pubDomainName+".", newNLBs[0].DNSName, newNLBs[0].CanonicalHostedZoneNameID, apiDNSName, comment, false)
		if err != nil {
			log.Error(err, "Error updating api.<clusterName> alias to internal NLB")
			return reconcile.Result{}, err
		}
		log.Info(fmt.Sprintf("%s successful ", comment))
		err = utils.AddAWSLBToMasterMachines(r.client, extNLBName, masterList)
		if err != nil {
			log.Error(err, "Error adding new LB to master machines' providerSpecs")
			return reconcile.Result{}, err
		}
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

// createOrUpdateIngressController will create or update the given ingresscontroller with fields from publishingstrategySpec.ApplicationIngress
func (r *ReconcilePublishingStrategy) createOrUpdateIngressController(appingress cloudingressv1alpha1.ApplicationIngress, newCertificate *corev1.LocalObjectReference) error {
	ingressControllerName := defaultIngressName
	// get name from the domain
	if appingress.Default != true {
		ingressControllerName = getIngressName(appingress.DNSName)
	}
	// try to get the ingresscontroller
	ingressController := &operatorv1.IngressController{}
	err := r.client.Get(context.TODO(), types.NamespacedName{
		Namespace: ingressControllerNamespace,
		Name:      ingressControllerName,
	}, ingressController)

	// ingress controller doesn't exist, so create it
	if err != nil {
		newIngressController, err := newApplicationIngressControllerCR(ingressControllerName, string(appingress.Listening), appingress.DNSName, newCertificate, appingress.RouteSelector.MatchLabels)
		if err != nil {
			log.Error(err, fmt.Sprintf("failed to generate information for default ingresscontroller with domain %s", appingress.DNSName))
			return err
		}
		err = r.client.Create(context.TODO(), newIngressController)
		if err != nil {
			log.Error(err, fmt.Sprintf("got error trying to create %s", ingressControllerName))
			return err
		}
		log.Info(fmt.Sprintf("created %s ingresscontroller successfully", ingressControllerName))
	} else {
		err = modifyApplicationIngressControllerCR(ingressController, string(appingress.Listening), appingress.DNSName, newCertificate, appingress.RouteSelector.MatchLabels)
		if err != nil {
			log.Error(err, fmt.Sprintf("got error trying to modify %s", ingressControllerName))
			return err
		}
		err = r.client.Update(context.TODO(), ingressController)
		if err != nil {
			log.Error(err, fmt.Sprintf("got error trying to update %s", ingressControllerName))
			return err
		}
		log.Info(fmt.Sprintf("updated %s ingresscontroller successfully", ingressControllerName))
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

// modifyApplicationIngressControllerCR modifies an existing IngressController CR
func modifyApplicationIngressControllerCR(existing *operatorv1.IngressController, scope, dnsName string, certificate *corev1.LocalObjectReference, matchLabels map[string]string) error {
	loadBalancerScope := operatorv1.LoadBalancerScope("")
	switch scope {
	case "internal":
		loadBalancerScope = operatorv1.InternalLoadBalancer
	case "external":
		loadBalancerScope = operatorv1.ExternalLoadBalancer
	default:
		return errors.New("ErrUpdatingIngressController")
	}

	existing.Spec.DefaultCertificate = certificate
	existing.Spec.Domain = dnsName
	existing.Spec.EndpointPublishingStrategy = &operatorv1.EndpointPublishingStrategy{
		Type: operatorv1.LoadBalancerServiceStrategyType,
		LoadBalancer: &operatorv1.LoadBalancerStrategy{
			Scope: loadBalancerScope,
		},
	}
	existing.Spec.RouteSelector = &metav1.LabelSelector{
		MatchLabels: matchLabels,
	}
	return nil
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
