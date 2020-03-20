package publishingstrategy

import (
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
	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	awsproviderapi "sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsproviderconfig/v1beta1"
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

	// create list of applicationIngress
	var ingressNotOnCluster []cloudingressv1alpha1.ApplicationIngress

	exisitingIngressMap := convertIngressControllerToMap(ingressControllerList.Items)

	// loop through every applicationingress in publishing strategy
	for _, publishingStrategyIngress := range instance.Spec.ApplicationIngress {
		if !checkExistingIngress(exisitingIngressMap, &publishingStrategyIngress) {
			ingressNotOnCluster = append(ingressNotOnCluster, publishingStrategyIngress)
		}
	}

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
		var extDNSName string
		// delete the external NLB
		for _, loadBalancer := range loadBalancerInfo {
			if loadBalancer.Scheme == "internet-facing" {
				extDNSName = loadBalancer.DNSName
				log.Info("Trying to remove external LB", "LB", extDNSName)
				err = awsClient.DeleteExternalLoadBalancer(loadBalancer.LoadBalancerArn)
				if err != nil {
					log.Error(err, "error deleting external LB")
					return reconcile.Result{}, err
				}
				err := RemoveLBFromMasterMachines(r.client, extDNSName, masterList)
				if err != nil {
					log.Error(err, "Error removing external LB from master machine objects")
					return reconcile.Result{}, err
				}
				log.Info("Load balancer removed from master machine objects", "LB", extDNSName)
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

		// create a new external NLB (TODO: add tags)
		infrastructureName, err := utils.GetClusterName(r.client)
		if err != nil {
			log.Error(err, "cannot get infrastructure name")
			return reconcile.Result{}, err
		}
		extNLBName := infrastructureName + "-test"

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
		err = AddLBToMasterMachines(r.client, extNLBName, masterList)
		if err != nil {
			log.Error(err, "Error adding new LB to master machines' providerSpecs")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	return reconcile.Result{}, nil
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
			log.Info("default ingresscontroller already exists on cluster. Enter retry...")
			for i := 0; i < 60; i++ {
				if i == 60 {
					log.Error(err, "out of retries")
					return err
				}
				log.Info(fmt.Sprintf("sleeping %d second before retrying again", i))
				time.Sleep(time.Duration(1) * time.Second)

				err = r.client.Create(context.TODO(), newDefaultIngressController)
				if err != nil {
					log.Info("not able to create new default ingresscontroller" + err.Error())
					continue
				}
				// if err not nil then successful
				log.Info("successfully created default ingresscontroller")
				break
			}
		} else {
			log.Error(err, fmt.Sprintf("failed to create new ingresscontroller with domain %s", appingress.DNSName))
			return err
		}
	}
	log.Info("successfully created new default ingresscontroller")
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
			log.Info("ingresscontroller already exists on cluster. Enter retry...")
			for i := 0; i < 60; i++ {
				if i == 60 {
					log.Error(err, "out of retries")
					return err
				}
				log.Info(fmt.Sprintf("sleeping %d second before retrying again", i))
				time.Sleep(time.Duration(1) * time.Second)

				err = r.client.Create(context.TODO(), newIngressController)
				if err != nil {
					log.Info("not able to create new ingresscontroller" + err.Error())
					continue
				}
				// if err not nil then successful
				log.Info("create successful. Breaking out of for loop")
				break
			}
		} else {
			log.Error(err, fmt.Sprintf("failed to create new ingresscontroller with domain %s", appingress.DNSName))
			return err
		}
	}
	log.Info("successfully created new ingresscontroller")
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
		return false
	}
	if !isOnCluster(publishingStrategyIngress, existingMap[publishingStrategyIngress.DNSName]) {
		return false
	}
	return true
}

// doesIngressMatch checks if application ingress in PublishingStrategy CR matches with IngressController CR
func isOnCluster(publishingStrategyIngress *cloudingressv1alpha1.ApplicationIngress, ingressController operatorv1.IngressController) bool {

	listening := string(publishingStrategyIngress.Listening)
	capListening := strings.Title(strings.ToLower(listening))
	if capListening != string(ingressController.Status.EndpointPublishingStrategy.LoadBalancer.Scope) {
		return false
	}
	if publishingStrategyIngress.DNSName != ingressController.Spec.Domain {
		return false
	}
	if publishingStrategyIngress.Certificate.Name != ingressController.Spec.DefaultCertificate.Name {
		return false
	}

	isRouteSelectorEqual := reflect.DeepEqual(ingressController.Spec.RouteSelector.MatchLabels, publishingStrategyIngress.RouteSelector.MatchLabels)
	if !isRouteSelectorEqual {
		return false
	}
	return true
}

func RemoveLBFromMasterMachines(kclient client.Client, elbName string, masterNodes *machineapi.MachineList) error {
	for _, machine := range masterNodes.Items {
		providerSpecDecoded, err := GetDecodedProviderSpec(machine)
		if err != nil {
			log.Error(err, "Error retrieving decoded ProviderSpec for machine", "machine", machine.Name)
			return err
		}
		lbList := providerSpecDecoded.LoadBalancers
		newLBList := []awsproviderapi.LoadBalancerReference{}
		for _, lb := range lbList {
			if !strings.HasPrefix(elbName, lb.Name) {
				log.Info("Machine's LB does not match LB to remove", "Machine LB", lb.Name, "LB to remove", elbName)
				log.Info("Keeping machine's LB in machine object", "LB", lb.Name, "Machine", machine.Name)
				newLBList = append(newLBList, lb)
			}
		}
		err = UpdateLBList(kclient, lbList, newLBList, machine, providerSpecDecoded)
		if err != nil {
			log.Error(err, "Error updating LB list for machine", "machine", machine.Name)
			return err
		}
	}
	return nil
}

func AddLBToMasterMachines(kclient client.Client, elbName string, masterNodes *machineapi.MachineList) error {
	for _, machine := range masterNodes.Items {
		providerSpecDecoded, err := GetDecodedProviderSpec(machine)
		if err != nil {
			log.Error(err, "Error retrieving decoded ProviderSpec for machine", "machine", machine.Name)
			return err
		}
		lbList := providerSpecDecoded.LoadBalancers
		newLBList := []awsproviderapi.LoadBalancerReference{}
		for _, lb := range lbList {
			if lb.Name != elbName {
				newLBList = append(newLBList, lb)
			}
		}
		newLB := awsproviderapi.LoadBalancerReference{
			Name: elbName,
			Type: awsproviderapi.NetworkLoadBalancerType,
		}
		newLBList = append(newLBList, newLB)
		err = UpdateLBList(kclient, lbList, newLBList, machine, providerSpecDecoded)
		if err != nil {
			log.Error(err, "Error updating LB list for machine", "machine", machine.Name)
			return err
		}
	}
	return nil
}

func GetDecodedProviderSpec(machine machineapi.Machine) (*awsproviderapi.AWSMachineProviderConfig, error) {
	awsCodec, err := awsproviderapi.NewCodec()
	if err != nil {
		log.Error(err, "Error creating AWSProviderConfigCodec")
		return nil, err
	}
	providerSpecEncoded := machine.Spec.ProviderSpec
	providerSpecDecoded := &awsproviderapi.AWSMachineProviderConfig{}
	err = awsCodec.DecodeProviderSpec(&providerSpecEncoded, providerSpecDecoded)
	if err != nil {
		log.Error(err, "Error decoding provider spec for machine", "machine", machine.Name)
		return nil, err
	}
	return providerSpecDecoded, nil
}

func UpdateLBList(kclient client.Client, oldLBList []awsproviderapi.LoadBalancerReference, newLBList []awsproviderapi.LoadBalancerReference, machineToPatch machineapi.Machine, providerSpecDecoded *awsproviderapi.AWSMachineProviderConfig) error {
	baseToPatch := client.MergeFrom(machineToPatch.DeepCopy())
	awsCodec, err := awsproviderapi.NewCodec()
	if err != nil {
		log.Error(err, "Error creating AWSProviderConfigCodec")
		return err
	}
	if !reflect.DeepEqual(oldLBList, newLBList) {
		providerSpecDecoded.LoadBalancers = newLBList
		newProviderSpecEncoded, err := awsCodec.EncodeProviderSpec(providerSpecDecoded)
		if err != nil {
			log.Error(err, "Error encoding provider spec for machine", "machine", machineToPatch.Name)
			return err
		}
		machineToPatch.Spec.ProviderSpec = *newProviderSpecEncoded
		machineObj := machineToPatch.DeepCopyObject()
		if err := kclient.Patch(context.Background(), machineObj, baseToPatch); err != nil {
			log.Error(err, "Failed to update LBs in machine's providerSpec", "machine", machineToPatch.Name)
			return err
		}
		log.Info("Updated master machine's LBs in providerSpec", "masterMachine", machineToPatch.Name)
		return nil
	}
	log.Info("No need to update LBs for master machine", "masterMachine", machineToPatch.Name)
	return nil
}
