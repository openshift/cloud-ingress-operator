package publishingstrategy

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/aws/aws-sdk-go/aws/awserr"
	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	"github.com/openshift/cloud-ingress-operator/pkg/awsclient"
	"github.com/openshift/cloud-ingress-operator/pkg/config"
	utils "github.com/openshift/cloud-ingress-operator/pkg/controller/utils"
	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"

	// corev1 "k8s.io/api/core/v1"
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

var log = logf.Log.WithName("controller_publishingstrategy")

// namespace where we will be creating the ingresscontroller types
var ingressControllerNamespace = "openshift-ingress-operator"

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

	masterList, err := utils.GetMasterNodes(r.client)
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
