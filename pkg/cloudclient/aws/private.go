package aws

// "Private" or non-interface conforming methods

import (
	"bytes"
	"context"
	goError "errors"
	"fmt"
	"path"
	"reflect"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	jsonserializer "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/types"
	k8s "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/cloud-ingress-operator/config"
	"github.com/openshift/cloud-ingress-operator/pkg/errors"
	baseutils "github.com/openshift/cloud-ingress-operator/pkg/utils"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/route53"
)

type awsLoadBalancer struct {
	elbName   string
	dnsName   string
	dnsZoneID string
}

type loadBalancer struct {
	endpointName string // from APIScheme
	baseDomain   string // cluster base domain
}

type loadBalancerV2 struct {
	canonicalHostedZoneNameID string
	dnsName                   string
	loadBalancerArn           string
	loadBalancerName          string
	scheme                    string
	vpcID                     string
}

// installConfig represents the bare minimum requirement to get the AWS cluster region from the install-config
// See https://bugzilla.redhat.com/show_bug.cgi?id=1814332
type installConfig struct {
	Platform struct {
		AWS struct {
			Region string `json:"region"`
		} `json:"aws"`
	} `json:"platform"`
}

// removeAWSLBFromMasterMachines removes a Load Balancer (with name elbName) from
// the spec.providerSpec.value.loadBalancers list for each of the master machine
// objects in a cluster
func removeAWSLBFromMasterMachines(kclient k8s.Client, elbName string, masterNodes *machinev1beta1.MachineList) error {
	for _, machine := range masterNodes.Items {
		providerSpecDecoded, err := getAWSDecodedProviderSpec(machine, kclient.Scheme())
		if err != nil {
			log.Error(err, "Error retrieving decoded ProviderSpec for machine", "machine", machine.Name)
			return err
		}
		lbList := providerSpecDecoded.LoadBalancers
		newLBList := []machinev1beta1.LoadBalancerReference{}
		for _, lb := range lbList {
			if lb.Name != elbName {
				log.Info("Machine's LB does not match LB to remove", "Machine LB", lb.Name, "LB to remove", elbName)
				log.Info("Keeping machine's LB in machine object", "LB", lb.Name, "Machine", machine.Name)
				newLBList = append(newLBList, lb)
			}
		}
		err = updateAWSLBList(kclient, lbList, newLBList, machine, providerSpecDecoded)
		if err != nil {
			log.Error(err, "Error updating LB list for machine", "machine", machine.Name)
			return err
		}
	}
	return nil
}

// getAWSDecodedProviderSpec casts the spec.providerSpec of an OpenShift machine
// object to an AWSMachineProviderConfig object, which is required to read and
// interact with fields in a machine's providerSpec
func getAWSDecodedProviderSpec(machine machinev1beta1.Machine, r *runtime.Scheme) (*machinev1beta1.AWSMachineProviderConfig, error) {
	decoder := serializer.NewCodecFactory(r).UniversalDecoder()
	providerSpecEncoded := machine.Spec.ProviderSpec
	providerSpecDecoded := &machinev1beta1.AWSMachineProviderConfig{}

	_, _, err := decoder.Decode(providerSpecEncoded.Value.Raw, nil, providerSpecDecoded)
	if err != nil {
		log.Error(err, "unable to decode AWS Machine Provider Spec")
		return nil, err
	}
	return providerSpecDecoded, nil
}

// updateAWSLBList compares an oldLBList to a newLBList and updates a machine
// object's spec.providerSpec.value.loadBalancers list with the newLBList if
// the old and new lists are not equal. this function requires the decoded
// ProviderSpec (as an AWSMachineProviderConfig object) that the
// getAWSDecodedProviderSpec function will provide
func updateAWSLBList(kclient k8s.Client, oldLBList []machinev1beta1.LoadBalancerReference, newLBList []machinev1beta1.LoadBalancerReference, machineToPatch machinev1beta1.Machine, providerSpecDecoded *machinev1beta1.AWSMachineProviderConfig) error {
	baseToPatch := k8s.MergeFrom(machineToPatch.DeepCopy())
	if !reflect.DeepEqual(oldLBList, newLBList) {
		providerSpecDecoded.LoadBalancers = newLBList

		rawEncodedSpec, err := encodeAWSMachineProviderSpec(providerSpecDecoded, kclient.Scheme())
		if err != nil {
			log.Error(err, "Error encoding provider spec for machine", "machine", machineToPatch.Name)
			return err
		}
		machineToPatch.Spec.ProviderSpec.Value = rawEncodedSpec
		machineObj := machineToPatch.DeepCopy()
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

// ensureAdminAPIDNS ensure the DNS record for the rh-api "admin API" for
// APIScheme is present and mapped to the corresponding Service's AWS
// LoadBalancer
func (ac *Client) ensureAdminAPIDNS(ctx context.Context, kclient k8s.Client, instance *cloudingressv1alpha1.APIScheme, svc *corev1.Service) error {
	return ac.ensureDNSForService(ctx, kclient, svc, instance.Spec.ManagementAPIServerIngress.DNSName, "RH API Endpoint")
}

// deleteAdminAPIDNS removes the DNS record for the rh-api "admin API" for
// APIScheme
func (ac *Client) deleteAdminAPIDNS(ctx context.Context, kclient k8s.Client, instance *cloudingressv1alpha1.APIScheme, svc *corev1.Service) error {
	return ac.removeDNSForService(ctx, kclient, svc, instance.Spec.ManagementAPIServerIngress.DNSName, "RH API Endpoint")
}

// setDefaultAPIPrivate sets the default api (api.<cluster-domain>) to private
// scope
func (ac *Client) setDefaultAPIPrivate(ctx context.Context, kclient k8s.Client, _ *cloudingressv1alpha1.PublishingStrategy) error {
	// Delete the NLB and remove the NLB from the master Machine objects in
	// cluster. At the same time, get the name of the DNS zone and base domain for
	// the internal load balancer
	intDNSName, intHostedZoneID, err := ac.removeLoadBalancerFromMasterNodes(ctx, kclient)
	if err != nil {
		return err
	}

	baseDomain, err := baseutils.GetClusterBaseDomain(kclient)
	if err != nil {
		return err
	}

	pubDomainName := baseDomain[strings.Index(baseDomain, ".")+1:]
	apiDNSName := fmt.Sprintf("api.%s.", baseDomain)
	comment := "Update api.<clusterName> alias to internal NLB"
	err = ac.upsertARecord(pubDomainName+".", intDNSName, intHostedZoneID, apiDNSName, comment, false)
	if err != nil {
		return err
	}
	return nil
}

// setDefaultAPIPublic sets the default API (api.<cluster-domain>) to public
// scope
func (ac *Client) setDefaultAPIPublic(ctx context.Context, kclient k8s.Client, instance *cloudingressv1alpha1.PublishingStrategy) error {
	nlbs, err := ac.listOwnedNLBs(kclient)
	if err != nil {
		return err
	}

	for _, networkLoadBalancer := range nlbs {
		if networkLoadBalancer.scheme == "internet-facing" && strings.HasSuffix(networkLoadBalancer.loadBalancerName, "-ext") {
			// nothing to do
			return nil
		}
	}
	// create new ext nlb
	infrastructureName, err := baseutils.GetClusterName(kclient)
	if err != nil {
		return err
	}
	extNLBName := infrastructureName + "-ext"

	subnetIDs, err := ac.getPublicSubnets(kclient)
	if err != nil {
		return err
	}
	if len(subnetIDs) == 0 {
		err = goError.New("no public subnets, can't change API to public")
		return err
	}

	tags := ac.GetTags(infrastructureName)

	newNLBs, err := ac.createNetworkLoadBalancer(extNLBName, "internet-facing", subnetIDs[0], tags)
	if err != nil {
		return err
	}
	if len(newNLBs) != 1 {
		return fmt.Errorf("more than one NLB, or no new NLB detected (expected 1, got %d)", len(newNLBs))
	}

	// attempt to use existing TargetGroup
	targetGroupName := fmt.Sprintf("%s-aext", infrastructureName)
	targetGroupARN, err := ac.getTargetGroupArn(targetGroupName)
	if err != nil {
		return err
	}
	err = ac.createListenerForNLB(targetGroupARN, newNLBs[0].loadBalancerArn)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == "TargetGroupAssociationLimit" {
				// not possible to modify lb, we'd have to create a new targetGroup
				return nil
			}
			return err
		}
		// TODO: log - cant create listener for new ext nlb
		return err
	}

	// can't create listener for new ext nlb
	baseDomain, err := baseutils.GetClusterBaseDomain(kclient)
	if err != nil {
		return err
	}
	pubDomainName := baseDomain[strings.Index(baseDomain, ".")+1:]
	apiDNSName := fmt.Sprintf("api.%s.", baseDomain)
	// not tested yet
	comment := "Update api.<clusterName> alias to external NLB"
	err = ac.upsertARecord(pubDomainName+".",
		newNLBs[0].dnsName,
		newNLBs[0].canonicalHostedZoneNameID,
		apiDNSName,
		comment,
		false)
	if err != nil {
		return err
	}
	// success
	return nil
}

// getMasterNodeSubnets returns all the subnets for Machines with 'master' label.
// return structure:
//
//	{
//	  public => subnetname,
//	  private => subnetname,
//	}
func getMasterNodeSubnets(kclient k8s.Client) (map[string]string, error) {
	subnets := make(map[string]string)
	machineList, err := baseutils.GetMasterMachines(kclient)
	if err != nil {
		return subnets, err
	}
	if len(machineList.Items) == 0 {
		return subnets, fmt.Errorf("did not find any master Machine objects")
	}

	// Obtain the availability zone
	awsconfig, err := getAWSDecodedProviderSpec(machineList.Items[0], kclient.Scheme())
	if err != nil {
		return subnets, err
	}

	// Infra object gives us the Infrastructure name, which is the combination of
	// cluster name and identifier.
	infra, err := baseutils.GetInfrastructureObject(kclient)
	if err != nil {
		return subnets, err
	}
	subnets["public"] = fmt.Sprintf("%s-public-%s", infra.Status.InfrastructureName, awsconfig.Placement.AvailabilityZone)
	subnets["private"] = fmt.Sprintf("%s-private-%s", infra.Status.InfrastructureName, awsconfig.Placement.AvailabilityZone)

	return subnets, nil
}

func (ac *Client) getPublicSubnets(kclient k8s.Client) ([]string, error) {

	var publicSubnets []string

	machineList, err := baseutils.GetMasterMachines(kclient)

	if err != nil {
		log.Error(err, "No master machines found")
		return nil, err
	}

	// Get the first master machine in the list
	masterMachine := machineList.Items[0]

	// Get the instance ID of the machine in the form of aws:///us-east-1a/i-<hash>
	instanceIDLong := masterMachine.Spec.ProviderID

	split := strings.Split(*instanceIDLong, "/")

	// The instance ID should be the last element of the split
	instanceID := split[len(split)-1]

	// Ensure we actually have an instnace ID by erroring if its missing
	if instanceID == "" {
		err = goError.New("instance ID is blank")
		return nil, err
	}

	// Get VPC the instance is in
	describeInstanceOutput, err := ac.ec2Client.DescribeInstances(
		&ec2.DescribeInstancesInput{
			InstanceIds: []*string{aws.String(instanceID)},
		},
	)
	if err != nil {
		return nil, err
	}

	// Extract the VPC ID from the subnet metadata
	targetVPC := describeInstanceOutput.Reservations[0].Instances[0].VpcId

	// List all subnets in the VPC
	allSubnets, err := ac.getAllSubnetsInVPC(*targetVPC)
	if err != nil {
		return nil, err
	}

	// List all route tables associated with the VPC
	routeTables, err := ac.getAllRouteTablesInVPC(*targetVPC)
	if err != nil {
		return nil, err
	}

	for _, subnet := range allSubnets {
		isPublic, err := isSubnetPublic(routeTables, *subnet.SubnetId)

		if err != nil {
			log.Error(err, "Error while determining if subnet is public")
			return nil, err
		}
		if isPublic {
			publicSubnets = append(publicSubnets, *subnet.SubnetId)
		}
	}

	return publicSubnets, nil
}

func (ac *Client) getAllSubnetsInVPC(vpcID string) ([]*ec2.Subnet, error) {

	var subnetIDs []*ec2.Subnet
	token := aws.String("initString")

	for token != nil {
		describeSubnetOutput, err := ac.ec2Client.DescribeSubnets(
			&ec2.DescribeSubnetsInput{
				Filters: []*ec2.Filter{
					{
						Name:   aws.String("vpc-id"),
						Values: []*string{aws.String(vpcID)},
					},
				},
			})
		if err != nil {
			log.Error(err, "Error while describing subnets")
			return nil, err
		}
		subnetIDs = append(subnetIDs, describeSubnetOutput.Subnets...)

		token = describeSubnetOutput.NextToken
	}

	return subnetIDs, nil
}

func (ac *Client) getAllRouteTablesInVPC(vpcID string) ([]*ec2.RouteTable, error) {

	var routeTables []*ec2.RouteTable
	token := aws.String("initString")

	for token != nil {
		describeRouteTablesOutput, err := ac.ec2Client.DescribeRouteTables(&ec2.DescribeRouteTablesInput{Filters: []*ec2.Filter{{Name: aws.String("vpc-id"), Values: []*string{aws.String(vpcID)}}}})
		if err != nil {
			log.Error(err, "Error while describing route tables")
			return nil, err
		}
		routeTables = append(routeTables, describeRouteTablesOutput.RouteTables...)

		token = describeRouteTablesOutput.NextToken
	}

	return routeTables, nil
}

func isSubnetPublic(rt []*ec2.RouteTable, subnetID string) (bool, error) {
	var subnetTable *ec2.RouteTable
	for _, table := range rt {
		for _, assoc := range table.Associations {
			if aws.StringValue(assoc.SubnetId) == subnetID {
				subnetTable = table
				break
			}
		}
	}

	if subnetTable == nil {
		// If there is no explicit association, the subnet will be implicitly
		// associated with the VPC's main routing table.
		for _, table := range rt {
			for _, assoc := range table.Associations {
				if aws.BoolValue(assoc.Main) {
					log.Info(fmt.Sprintf(
						"Assuming implicit use of main routing table %s for %s",
						aws.StringValue(table.RouteTableId), subnetID))
					subnetTable = table
					break
				}
			}
		}
	}

	if subnetTable == nil {
		return false, fmt.Errorf("could not locate routing table for %s", subnetID)
	}

	for _, route := range subnetTable.Routes {
		// There is no direct way in the AWS API to determine if a subnet is public or private.
		// A public subnet is one which has an internet gateway route
		// we look for the gatewayId and make sure it has the prefix of igw to differentiate
		// from the default in-subnet route which is called "local"
		// or other virtual gateway (starting with vgv)
		// or vpc peering connections (starting with pcx).
		if strings.HasPrefix(aws.StringValue(route.GatewayId), "igw") {
			return true, nil
		}
	}

	return false, nil
}

// getClusterRegion returns the installed cluster's AWS region
func getClusterRegion(kclient k8s.Client) (string, error) {
	infra, err := baseutils.GetInfrastructureObject(kclient)
	if err != nil {
		return "", err
	} else if infra.Status.PlatformStatus == nil {
		// Try the deprecated configmap. See https://bugzilla.redhat.com/show_bug.cgi?id=1814332
		return readClusterRegionFromConfigMap(kclient)
	}
	return infra.Status.PlatformStatus.AWS.Region, nil
}

func readClusterRegionFromConfigMap(kclient k8s.Client) (string, error) {
	cm, err := getClusterConfigMap(kclient)
	if err != nil {
		return "", err
	}
	return parseClusterRegionFromConfigMap(cm)
}

func getClusterConfigMap(kclient k8s.Client) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	ns := types.NamespacedName{
		Namespace: "kube-system",
		Name:      "cluster-config-v1",
	}
	err := kclient.Get(context.TODO(), ns, cm)
	return cm, err
}

func parseClusterRegionFromConfigMap(cm *corev1.ConfigMap) (string, error) {
	if cm == nil || cm.Data == nil {
		return "", fmt.Errorf("unexpected nil configmap or nil configmap Data")
	}
	data, ok := cm.Data["install-config"]
	if !ok {
		return "", fmt.Errorf("missing install-config in configmap")
	}
	var ic installConfig
	if err := yaml.Unmarshal([]byte(data), &ic); err != nil {
		return "", fmt.Errorf("invalid install-config: %v\njson:%s", err, data)
	}
	return ic.Platform.AWS.Region, nil
}

/* Helper functions below, sorted by AWS API type */

// ELB (v1)
func (ac *Client) doesELBExist(elbName string) (*awsLoadBalancer, error) {
	input := &elb.DescribeLoadBalancersInput{
		LoadBalancerNames: []*string{aws.String(elbName)},
	}
	output, err := ac.elbClient.DescribeLoadBalancers(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case elb.ErrCodeAccessPointNotFoundException:
				return &awsLoadBalancer{}, errors.NewLoadBalancerNotReadyError()
			default:
				return &awsLoadBalancer{}, err
			}
		}
	}
	return &awsLoadBalancer{
			elbName:   elbName,
			dnsName:   *output.LoadBalancerDescriptions[0].DNSName,
			dnsZoneID: *output.LoadBalancerDescriptions[0].CanonicalHostedZoneNameID},
		nil
}

// route53

func (ac *Client) ensureDNSForService(ctx context.Context, kclient k8s.Client, svc *corev1.Service, dnsName, dnsComment string) error {
	// Get the ELB name from the Service's UID. Truncate to 32 characters for AWS
	elbName := strings.ReplaceAll("a"+string(svc.UID), "-", "")
	if len(elbName) > 32 {
		// Truncate to 32 characters
		elbName = elbName[0:32]
	}
	awsELB, err := ac.doesELBExist(elbName)
	// Primarily checking to see if this exists. It is an error if it does not,
	// likely because AWS is still creating it and the Reconcile should be retried
	if err != nil {
		return err
	}
	// ELB exists, now let's set the DNS
	clusterBaseDomain, err := baseutils.GetClusterBaseDomain(kclient)
	if err != nil {
		return err
	}
	lb := &loadBalancer{
		endpointName: dnsName,
		baseDomain:   clusterBaseDomain,
	}
	return ac.ensureDNSRecord(lb, awsELB, dnsComment)
}

// removeDNSForService will remove a DNS entry for a particular Service
func (ac *Client) removeDNSForService(ctx context.Context, kclient k8s.Client, svc *corev1.Service, dnsName, dnsComment string) error {
	// Get the ELB name from the Service's UID. Truncate to 32 characters for AWS
	elbName := strings.ReplaceAll("a"+string(svc.UID), "-", "")[0:32]
	awsELB, err := ac.doesELBExist(elbName)
	// Primarily checking to see if this exists. It is an error if it does not,
	// likely because AWS is still creating it and the Reconcile should be retried
	if err != nil {
		return err
	}
	// ELB exists, now let's set the DNS
	clusterBaseDomain, err := baseutils.GetClusterBaseDomain(kclient)
	if err != nil {
		return err
	}
	return ac.ensureDNSRecordsRemoved(
		clusterBaseDomain,
		awsELB.dnsName,
		awsELB.dnsZoneID,
		dnsName+"."+clusterBaseDomain,
		dnsComment,
		false)
}

func (ac *Client) deleteARecord(clusterDomain, DNSName, aliasDNSZoneID, resourceRecordSetName string, targetHealth bool) error {
	publicHostedZoneID, err := ac.getPublicHostedZoneID(clusterDomain)
	if err != nil {
		return err
	}

	change := &route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action: aws.String("DELETE"),
					ResourceRecordSet: &route53.ResourceRecordSet{
						AliasTarget: &route53.AliasTarget{
							DNSName:              aws.String(DNSName),
							EvaluateTargetHealth: aws.Bool(targetHealth),
							HostedZoneId:         aws.String(aliasDNSZoneID),
						},
						Name: aws.String(resourceRecordSetName),
						Type: aws.String("A"),
					},
				},
			},
		},
		HostedZoneId: aws.String(publicHostedZoneID),
	}
	_, err = ac.route53Client.ChangeResourceRecordSets(change)
	if err != nil {
		// If the DNS entry was not found, disregard the error.
		//
		// XXX The error code in this case is InvalidChangeBatch
		//     with no other errors in awserr.Error.OrigErr() or
		//     in awserr.BatchedErrors.OrigErrs().
		//
		//     So there seems to be no way, short of parsing the
		//     message string, to verify the error was caused by
		//     a missing DNS entry and not something else.
		//
		if awsErr, ok := err.(awserr.Error); ok {
			if awsErr.Code() == route53.ErrCodeInvalidChangeBatch {
				return nil
			}
		}
	}

	return err
}

// recordExists checks if a specific RecordSet already exist in route53
func (ac *Client) recordExists(resourceRecordSet *route53.ResourceRecordSet, publicHostedZoneID string) (bool, error) {
	if resourceRecordSet == nil {
		return false, goError.New("resourceRecordSet can't be nil")
	}
	if resourceRecordSet.Name == nil {
		return false, goError.New("resourceRecordSet Name is required")
	}
	// Route53 assumes that the domain name that you specify is fully qualified.
	// Therefore, it doesn't mind if the trailing "." is missing.
	// However, adding the trailing "." make the future comparaison easier, since route53 listResourceRecordSets contains trailing dots
	if !strings.HasSuffix(*resourceRecordSet.Name, ".") {
		*resourceRecordSet.Name += "."
	}
	if resourceRecordSet.AliasTarget != nil && !strings.HasSuffix(*resourceRecordSet.AliasTarget.DNSName, ".") {
		*resourceRecordSet.AliasTarget.DNSName += "."
	}

	var recordExists bool
	input := &route53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(publicHostedZoneID),
	}

	// Check if any property of the record was changed.
	// aws-go-sdk may potentially give all results in multiple pages, so this will go
	// through every page given in response to the API call
	err := ac.route53Client.ListResourceRecordSetsPages(input, func(p *route53.ListResourceRecordSetsOutput, lastPage bool) bool {
		for _, record := range p.ResourceRecordSets {
			if *record.Name == *resourceRecordSet.Name && *record.Type == *resourceRecordSet.Type && reflect.DeepEqual(record.AliasTarget, resourceRecordSet.AliasTarget) {
				recordExists = true
				return false
			}
		}
		// we didn't find the record in this page, keep looking
		return true
	})

	return recordExists, err
}

func (ac *Client) upsertARecord(clusterDomain, DNSName, aliasDNSZoneID, resourceRecordSetName, comment string, targetHealth bool) error {
	publicHostedZoneID, err := ac.getPublicHostedZoneID(clusterDomain)
	if err != nil {
		return err
	}

	resourceRecordSet := &route53.ResourceRecordSet{
		AliasTarget: &route53.AliasTarget{
			DNSName:              aws.String(DNSName),
			EvaluateTargetHealth: aws.Bool(targetHealth),
			HostedZoneId:         aws.String(aliasDNSZoneID),
		},
		Name: aws.String(resourceRecordSetName),
		Type: aws.String("A"),
	}

	recordExists, err := ac.recordExists(resourceRecordSet, publicHostedZoneID)
	if err != nil || recordExists {
		return err
	}

	change := &route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action:            aws.String("UPSERT"),
					ResourceRecordSet: resourceRecordSet,
				},
			},
			Comment: aws.String(comment),
		},
		HostedZoneId: aws.String(publicHostedZoneID),
	}
	_, err = ac.route53Client.ChangeResourceRecordSets(change)
	return err
}

func (ac *Client) getPublicHostedZoneID(clusterDomain string) (string, error) {
	input := &route53.ListHostedZonesByNameInput{
		DNSName: aws.String(clusterDomain),
	}
	output, err := ac.route53Client.ListHostedZonesByName(input)
	if err != nil {
		return "", err
	}
	for _, zone := range output.HostedZones {
		if *zone.Name == clusterDomain {
			return path.Base(aws.StringValue(zone.Id)), nil
		}
	}

	return "", fmt.Errorf("route53 Zone not found for %s", clusterDomain)

}

func (ac *Client) ensureDNSRecord(lb *loadBalancer, awsObj *awsLoadBalancer, comment string) error {
	// private zone

	for i := 1; i <= config.MaxAPIRetries; i++ {
		err := ac.upsertARecord(
			lb.baseDomain+".",
			awsObj.dnsName,
			awsObj.dnsZoneID,
			lb.endpointName+"."+lb.baseDomain,
			comment,
			false)
		if err != nil {
			log.Error(err, "Couldn't upsert A record for private zone",
				"retryAttempt", i,
				"publicZone", lb.baseDomain+".",
				"dnsName", awsObj.dnsName,
				"dnsZoneID", awsObj.dnsZoneID,
				"endpointName", lb.endpointName+".", lb.baseDomain)
			if i == config.MaxAPIRetries {
				log.Error(err, "Couldn't upsert A record for private zone: Retries Exhausted")
				return err
			}
			// TODO: Logging - sleep
			time.Sleep(time.Duration(i) * time.Second)
		} else {
			// success
			break
		}
	}

	// Public zone
	// The public zone omits the cluster name. So an example:
	// A cluster's base domain of alice-cluster.l4s7.s1.domain.com will need an
	// entry made in l4s7.s1.domain.com. zone.
	// Public zone
	// The public zone omits the cluster name. So an example:
	// A cluster's base domain of alice-cluster.l4s7.s1.domain.com will need an
	// entry made in l4s7.s1.domain.com. zone.
	publicZone := lb.baseDomain[strings.Index(lb.baseDomain, ".")+1:]

	for i := 1; i <= config.MaxAPIRetries; i++ {
		// Append a . to get the zone name
		err := ac.upsertARecord(
			publicZone+".",
			awsObj.dnsName,
			awsObj.dnsZoneID,
			lb.endpointName+"."+lb.baseDomain,
			"RH API Endpoint",
			false)
		if err != nil {
			log.Error(err, "Couldn't upsert A record for public zone",
				"retryAttempt", i,
				"publicZone", publicZone+".",
				"dnsName", awsObj.dnsName,
				"dnsZoneID", awsObj.dnsZoneID,
				"endpointName", lb.endpointName+".", lb.baseDomain)
			if i == config.MaxAPIRetries {
				log.Error(err, "Couldn't upsert A record for public zone: Retries Exhausted")
				return err
			}
			time.Sleep(time.Duration(i) * time.Second)
		} else {
			// success
			break
		}
	}
	return nil
}

// ensureDNSRecordsRemoved undoes ensureDNSRecord
func (ac *Client) ensureDNSRecordsRemoved(clusterDomain, DNSName, aliasDNSZoneID, resourceRecordSetName, comment string, targetHealth bool) error {
	for i := 1; i <= config.MaxAPIRetries; i++ {
		err := ac.deleteARecord(
			clusterDomain+".",
			DNSName,
			aliasDNSZoneID,
			resourceRecordSetName,
			targetHealth)
		if err != nil {
			// retry
			// TODO: logging
			if i == config.MaxAPIRetries {
				// TODO: logging
				return err
			}
			// TODO: logging
			time.Sleep(time.Duration(i) * time.Second)
		} else {
			break
		}
	}
	for i := 1; i <= config.MaxAPIRetries; i++ {
		err := ac.deleteARecord(
			// The public zone name omits the cluster name.
			// e.g. mycluster.abcd.s1.openshift.com -> abcd.s1.openshift.com
			clusterDomain[strings.Index(clusterDomain, ".")+1:]+".",
			DNSName,
			aliasDNSZoneID,
			resourceRecordSetName,
			targetHealth)
		if err != nil {
			// retry
			// TODO: logging
			if i == config.MaxAPIRetries {
				// TODO: logging
				return err
			}
			// TODO: logging
			time.Sleep(time.Duration(i) * time.Second)
		} else {
			break
		}
	}
	// public
	return nil
}

// ELBv2

// removeLoadBalancerFromMasterNodes
func (ac *Client) removeLoadBalancerFromMasterNodes(ctx context.Context, kclient k8s.Client) (string, string, error) {
	clusterName, err := baseutils.GetClusterName(kclient)
	if err != nil {
		return "", "", err
	}

	nlbs, err := ac.listOwnedNLBs(kclient)
	if err != nil {
		return "", "", err
	}

	// Detect if this is a CPMS active/inactive cluster and choose the right strategy:
	// 1. Remove the CPMS if needed
	// 2. Remove the LBs
	// 3. Readd the CPMS if needed
	masterList, err := baseutils.GetMasterMachines(kclient)
	if err != nil {
		return "", "", err
	}
	var cpms *machinev1.ControlPlaneMachineSet
	cpms, err = baseutils.GetControlPlaneMachineSet(kclient)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return "", "", err
		}
		// If there is no CPMS we handle it as an inactive one.
		cpms = &machinev1.ControlPlaneMachineSet{
			Spec: machinev1.ControlPlaneMachineSetSpec{
				State: machinev1.ControlPlaneMachineSetStateInactive,
			},
		}
	}
	removalClosure := getLoadBalancerRemovalFunc(ctx, kclient, masterList, cpms)
	var intDNSName, intHostedZoneID, lbName string
	for _, networkLoadBalancer := range nlbs {
		if networkLoadBalancer.scheme == "internet-facing" {

			lbName = networkLoadBalancer.loadBalancerName

			canDelete, err := ac.canDeleteNlb(networkLoadBalancer, clusterName)
			if err != nil {
				log.Error(err, "Problem attempting to remove", "NLB", networkLoadBalancer.loadBalancerName)
				return "", "", err
			}

			if canDelete {

				log.Info("Removing from cluster", "NLB", networkLoadBalancer.loadBalancerName)
				err = ac.deleteExternalLoadBalancer(networkLoadBalancer.loadBalancerArn)
				if err != nil {
					return "", "", err
				}
				err = removalClosure(lbName)
				if err != nil {
					return "", "", err
				}
			}

		}
	}

	internalAPINLB, err := ac.getInteralAPINLB(kclient)
	if err != nil {
		return "", "", err
	}

	// Only use the NLB specifically created for internal traffic
	// This is to avoid other internal NLBs that may come from Service objects
	intDNSName = internalAPINLB.dnsName
	intHostedZoneID = internalAPINLB.canonicalHostedZoneNameID

	return intDNSName, intHostedZoneID, nil
}

func (ac *Client) getInteralAPINLB(kclient k8s.Client) (loadBalancerV2, error) {

	// Build the load balancer tag to look for.
	clusterName, err := baseutils.GetClusterName(kclient)
	if err != nil {
		return loadBalancerV2{}, err
	}

	nlbs, err := ac.listOwnedNLBs(kclient)
	if err != nil {
		return loadBalancerV2{}, err
	}

	nameTag := &elbv2.Tag{
		Key:   aws.String("Name"),
		Value: aws.String(clusterName + "-int"),
	}

	for _, nlb := range nlbs {
		if nlb.scheme == "internal" {
			tagsOutput, err := ac.elbv2Client.DescribeTags(
				&elbv2.DescribeTagsInput{
					ResourceArns: []*string{aws.String(nlb.loadBalancerArn)},
				},
			)
			if err != nil {
				return loadBalancerV2{}, err
			}
			for _, tagDescription := range tagsOutput.TagDescriptions {
				for _, tag := range tagDescription.Tags {
					if reflect.DeepEqual(tag, nameTag) {
						return nlb, nil
					}
				}
			}
		}
	}

	return loadBalancerV2{}, fmt.Errorf("could not find internal API NLB")
}

// listOwnedNLBs uses the DescribeLoadBalancersV2 to get back a list of all
// Network Load Balancers, then filters the list to those owned by the cluster
func (ac *Client) listOwnedNLBs(kclient k8s.Client) ([]loadBalancerV2, error) {
	// Build the load balancer tag to look for.
	clusterName, err := baseutils.GetClusterName(kclient)
	if err != nil {
		return []loadBalancerV2{}, err
	}
	ownedTag := &elbv2.Tag{
		Key:   aws.String("kubernetes.io/cluster/" + clusterName),
		Value: aws.String("owned"),
	}

	// Collect all load balancers into a map, indexed by ARN.
	// Simultaneously, collect all load balancer ARNs into a slice.
	// The slice is used to request load balancer tags in batches.
	resourceArns := make([]string, 0, 20)
	loadBalancerMap := make(map[string]*elbv2.LoadBalancer)
	err = ac.elbv2Client.DescribeLoadBalancersPages(
		&elbv2.DescribeLoadBalancersInput{},
		func(page *elbv2.DescribeLoadBalancersOutput, lastPage bool) bool {
			for _, loadBalancer := range page.LoadBalancers {
				arn := aws.StringValue(loadBalancer.LoadBalancerArn)
				resourceArns = append(resourceArns, arn)
				loadBalancerMap[arn] = loadBalancer
			}
			return true
		},
	)
	if err != nil {
		return []loadBalancerV2{}, err
	}

	// Request tags for up to 20 load balancers at a time.
	for i := 0; i < len(resourceArns); i += 20 {
		end := i + 20
		if end > len(resourceArns) {
			end = len(resourceArns)
		}
		tagsOutput, err := ac.elbv2Client.DescribeTags(
			&elbv2.DescribeTagsInput{
				ResourceArns: aws.StringSlice(resourceArns[i:end]),
			},
		)
		if err != nil {
			return []loadBalancerV2{}, err
		}

		// Keep only load balancers owned by the cluster.
		for _, tagDescription := range tagsOutput.TagDescriptions {
			var foundTag bool
			for _, tag := range tagDescription.Tags {
				if reflect.DeepEqual(tag, ownedTag) {
					foundTag = true
					break
				}
			}
			if !foundTag {
				arn := aws.StringValue(tagDescription.ResourceArn)
				delete(loadBalancerMap, arn)
			}
		}
	}

	loadBalancers := make([]loadBalancerV2, 0, len(loadBalancerMap))
	for _, loadBalancer := range loadBalancerMap {
		loadBalancers = append(loadBalancers, loadBalancerV2{
			canonicalHostedZoneNameID: aws.StringValue(loadBalancer.CanonicalHostedZoneId),
			dnsName:                   aws.StringValue(loadBalancer.DNSName),
			loadBalancerArn:           aws.StringValue(loadBalancer.LoadBalancerArn),
			loadBalancerName:          aws.StringValue(loadBalancer.LoadBalancerName),
			scheme:                    aws.StringValue(loadBalancer.Scheme),
			vpcID:                     aws.StringValue(loadBalancer.VpcId),
		})
	}
	return loadBalancers, nil
}

// deleteExternalLoadBalancer takes in the external LB arn and deletes the entire LB
func (ac *Client) deleteExternalLoadBalancer(extLoadBalancerArn string) error {
	i := elbv2.DeleteLoadBalancerInput{
		LoadBalancerArn: aws.String(extLoadBalancerArn),
	}
	_, err := ac.elbv2Client.DeleteLoadBalancer(&i)
	return err
}

// createNetworkLoadBalancer should only return one new NLB at a time
func (ac *Client) createNetworkLoadBalancer(lbName, scheme, subnet string, tags []*elbv2.Tag) ([]loadBalancerV2, error) {
	i := &elbv2.CreateLoadBalancerInput{
		Name:   aws.String(lbName),
		Scheme: aws.String(scheme),
		Subnets: []*string{
			aws.String(subnet),
		},
		Type: aws.String("network"),
		Tags: tags,
	}

	result, err := ac.elbv2Client.CreateLoadBalancer(i)
	if err != nil {
		return []loadBalancerV2{}, err
	}

	// there should only be 1 NLB made, but since CreateLoadBalancerOutput takes in slice
	// we return it as slice
	loadBalancers := make([]loadBalancerV2, 0)
	for _, loadBalancer := range result.LoadBalancers {
		loadBalancers = append(loadBalancers, loadBalancerV2{
			canonicalHostedZoneNameID: aws.StringValue(loadBalancer.CanonicalHostedZoneId),
			dnsName:                   aws.StringValue(loadBalancer.DNSName),
			loadBalancerArn:           aws.StringValue(loadBalancer.LoadBalancerArn),
			loadBalancerName:          aws.StringValue(loadBalancer.LoadBalancerName),
			scheme:                    aws.StringValue(loadBalancer.Scheme),
			vpcID:                     aws.StringValue(loadBalancer.VpcId),
		})
	}
	return loadBalancers, nil
}

// createListenerForNLB creates a listener between target group and nlb given their arn
func (ac *Client) createListenerForNLB(targetGroupArn, loadBalancerArn string) error {
	i := &elbv2.CreateListenerInput{
		DefaultActions: []*elbv2.Action{
			{
				TargetGroupArn: aws.String(targetGroupArn),
				Type:           aws.String("forward"),
			},
		},
		LoadBalancerArn: aws.String(loadBalancerArn),
		Port:            aws.Int64(6443),
		Protocol:        aws.String("TCP"),
	}

	_, err := ac.elbv2Client.CreateListener(i)
	if err != nil {
		return err
	}
	return nil
}

// getTargetGroupArn by passing in targetGroup Name
func (ac *Client) getTargetGroupArn(targetGroupName string) (string, error) {
	i := &elbv2.DescribeTargetGroupsInput{
		Names: []*string{
			aws.String(targetGroupName),
		},
	}

	result, err := ac.elbv2Client.DescribeTargetGroups(i)
	if err != nil {
		return "", err
	}
	return aws.StringValue(result.TargetGroups[0].TargetGroupArn), nil
}

func (ac *Client) GetTags(clusterName string) []*elbv2.Tag {
	tags := []*elbv2.Tag{
		{
			Key:   aws.String("kubernetes.io/cluster/" + clusterName),
			Value: aws.String("owned"),
		},
		{
			Key:   aws.String("Name"),
			Value: aws.String(clusterName + "-ext"), //in form of samn-test-qb58m-ext
		},
		{
			Key:   aws.String("red-hat-managed"),
			Value: aws.String("true"),
		},
	}
	return tags
}

func encodeAWSMachineProviderSpec(awsProviderSpec *machinev1beta1.AWSMachineProviderConfig, scheme *runtime.Scheme) (*runtime.RawExtension, error) {
	serializer := jsonserializer.NewSerializer(jsonserializer.DefaultMetaFactory, scheme, scheme, false)
	var buffer bytes.Buffer
	err := serializer.Encode(awsProviderSpec, &buffer)
	if err != nil {
		return nil, err
	}
	return &runtime.RawExtension{
		Raw: buffer.Bytes(),
	}, nil
}

// This code should run on non-CPMS clusters
func removeLoadBalancerMachineSet(ctx context.Context, kclient k8s.Client, lbName string, masterList *machinev1beta1.MachineList) error {
	err := removeAWSLBFromMasterMachines(kclient, lbName, masterList)
	if err != nil {
		return err
	}
	return nil
}

func removeLoadBalancerCPMS(ctx context.Context, kclient k8s.Client, lbName string, cpms *machinev1.ControlPlaneMachineSet) error {
	cpmsPatch := k8s.MergeFrom(cpms.DeepCopy())
	rawExtension := cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec.ProviderSpec.Value
	spec, err := baseutils.ConvertFromRawExtension[machinev1beta1.AWSMachineProviderConfig](rawExtension)
	if err != nil {
		return err
	}
	var remainingLoadBalancers []machinev1beta1.LoadBalancerReference
	for _, lb := range spec.LoadBalancers {
		if lb.Name == lbName {
			log.Info("Removing loadbalancer %s from CPMs", "load-balancer-name", lbName)
		} else {
			log.Info("Keeping loadbalancer %s from CPMs", "load-balancer-name", lb.Name)
			remainingLoadBalancers = append(remainingLoadBalancers, lb)
		}
	}
	spec.LoadBalancers = remainingLoadBalancers
	extension, err := baseutils.ConvertToRawBytes(spec)
	if err != nil {
		return err
	}
	cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec.ProviderSpec.Value.Raw = extension
	err = kclient.Patch(ctx, cpms, cpmsPatch)
	if err != nil {
		return fmt.Errorf("could not update CPMS: %v", err)
	}
	return nil
}

func getLoadBalancerRemovalFunc(ctx context.Context, kclient k8s.Client, masterList *machinev1beta1.MachineList, cpms *machinev1.ControlPlaneMachineSet) func(string) error {
	if cpms.Spec.State == machinev1.ControlPlaneMachineSetStateActive {
		return func(lbName string) error {
			log.Info("Removing active CPMS")
			err := baseutils.DeleteCPMS(ctx, kclient, cpms)
			if err != nil {
				log.Error(err, "failed to delete CPMS")
				return err
			}
			// It seems this has to actually do both - the machine api is not
			// able to remove machines that still reference a LB that does not
			// exist:
			// E1117 14:42:49.349992       1 controller.go:255] fbergman-14568-tn7t8-master-0: failed to delete machine: fbergman-14568-tn7t8-master-0: reconciler failed to Delete machine: failed to updated update load balancers: LoadBalancerNotFound: Load balancers '[fbergman-14568-tn7t8-ext]' not found
			log.Info("Updating controlplane machines")
			err = removeLoadBalancerMachineSet(ctx, kclient, lbName, masterList)
			if err != nil {
				log.Error(err, "failed to remove load balancer from machines")
				return err
			}
			go func() {
				maxRetries := 5
				for {
					time.Sleep(60 * time.Second)
					log.Info("Retrieve cpms again")
					cpms, err := baseutils.GetControlPlaneMachineSet(kclient)
					if err != nil {
						log.Error(err, "could not get updated CPMS")
					}
					log.Info("Removing LB from cpms")
					err = removeLoadBalancerCPMS(ctx, kclient, lbName, cpms)
					if err != nil {
						log.Error(err, "failed to update CPMS")
					}
					log.Info("Retrieve cpms again")
					cpms, err = baseutils.GetControlPlaneMachineSet(kclient)
					if err != nil {
						log.Error(err, "Could not retrieve CPMS")
					}
					err = baseutils.SetCPMSActive(context.Background(), kclient, cpms)
					if err != nil {
						log.Error(err, "Could not set CPMS active")
					} else {
						break
					}
					if maxRetries == 0 {
						log.Info("Could not set CPMS back to active after 5 attempts")
						break
					}
					maxRetries = maxRetries - 1
				}
			}()
			// Don't fail the following steps - setting CPMS back to active will be tried again
			return nil
		}
	} else {
		return func(lbName string) error {
			return removeLoadBalancerMachineSet(ctx, kclient, lbName, masterList)
		}
	}
}

// getAllTagsFromLoadBalancer Gets all the tags from a load balancer and returns them on a map
func (ac *Client) getAllTagsFromLoadBalancer(loadBalancerArn string) (map[string]string, error) {

	// Input parameters for DescribeTags API call
	ElbInput := &elbv2.DescribeTagsInput{
		ResourceArns: []*string{aws.String(loadBalancerArn)},
	}

	// Make DescribeTags API call to get tags for the specified load balancer
	resp, err := ac.elbv2Client.DescribeTags(ElbInput)
	if err != nil {
		return nil, err
	}

	// Extract tags from the response
	tags := make(map[string]string)
	for _, tagDescription := range resp.TagDescriptions {
		for _, tag := range tagDescription.Tags {
			tags[*tag.Key] = *tag.Value
		}
	}

	return tags, nil
}

// canDeleteNlb is a function that checks if and nlb can be deleted by checking certain conditions (if certain tags are present)
func (ac *Client) canDeleteNlb(networkLoadBalancer loadBalancerV2, clusterName string) (bool, error) {

	var belongstoCluster, notServiceAttached, isExternal bool

	nlbTagsMap, err := ac.getAllTagsFromLoadBalancer(networkLoadBalancer.loadBalancerArn)
	if err != nil {
		log.Error(err, "Problem getting tags", "NLB", networkLoadBalancer.loadBalancerArn)
		return false, err
	}

	//check if the elb belongs to the right cluster (this is in case there are more than one cluster in the same region)
	if _, ok := nlbTagsMap["kubernetes.io/cluster/"+clusterName]; ok {
		belongstoCluster = true
	}
	//check the elb is not attached to a service
	if _, ok := nlbTagsMap["kubernetes.io/service-name"]; !ok {
		notServiceAttached = true
	}
	//check the elb has the external suffix
	if strings.HasSuffix(nlbTagsMap["Name"], "-ext") {
		isExternal = true
	}

	return (belongstoCluster && notServiceAttached && isExternal), nil
}
