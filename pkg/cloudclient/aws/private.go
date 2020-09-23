package aws

// "Private" or non-interface conforming methods

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	utils "github.com/openshift/cloud-ingress-operator/pkg/controller/utils"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/cloud-ingress-operator/pkg/config"
	"github.com/openshift/cloud-ingress-operator/pkg/errors"

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

// ensureAdminAPIDNS ensure the DNS record for the rh-api "admin API" for
// APIScheme is present and mapped to the corresponding Service's AWS
// LoadBalancer
func (c *Client) ensureAdminAPIDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.APIScheme, svc *corev1.Service) error {
	return c.ensureDNSForService(ctx, kclient, svc, instance.Spec.ManagementAPIServerIngress.DNSName, "RH API Endpoint")
}

// deleteAdminAPIDNS removes the DNS record for the rh-api "admin API" for
// APIScheme
func (c *Client) deleteAdminAPIDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.APIScheme, svc *corev1.Service) error {
	return c.removeDNSForService(ctx, kclient, svc, instance.Spec.ManagementAPIServerIngress.DNSName, "RH API Endpoint")
}

// ensureSSHDNS ensures the DNS record for the SSH Service LoadBalancer is set
func (c *Client) ensureSSHDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.SSHD, svc *corev1.Service) error {
	return c.ensureDNSForService(ctx, kclient, svc, instance.Spec.DNSName, "RH SSH Endpoint")
}

// deleteSSHDNS ensures the DNS record for the SSH Service AWS LoadBalancer is unset
func (c *Client) deleteSSHDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.SSHD, svc *corev1.Service) error {
	return c.removeDNSForService(ctx, kclient, svc, instance.Spec.DNSName, "RH SSH Endpoint")
}

// setDefaultAPIPrivate sets the default api (api.<cluster-domain>) to private
// scope
func (c *Client) setDefaultAPIPrivate(ctx context.Context, kclient client.Client, _ *cloudingressv1alpha1.PublishingStrategy) error {
	// Delete the NLB and remove the NLB from the master Machine objects in
	// cluster. At the same time, get the name of the DNS zone and base domain for
	// the internal load balancer
	intDNSName, intHostedZoneID, err := c.removeLoadBalancerFromMasterNodes(ctx, kclient)
	if err != nil {
		return err
	}

	baseDomain, err := utils.GetClusterBaseDomain(kclient)
	if err != nil {
		return err
	}

	pubDomainName := baseDomain[strings.Index(baseDomain, ".")+1:]
	apiDNSName := fmt.Sprintf("api.%s.", baseDomain)
	comment := "Update api.<clusterName> alias to internal NLB"
	err = c.upsertARecord(pubDomainName+".", intDNSName, intHostedZoneID, apiDNSName, comment, false)
	if err != nil {
		return err
	}
	return nil
}

// setDefaultAPIPublic sets the default API (api.<cluster-domain>) to public
// scope
func (c *Client) setDefaultAPIPublic(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.PublishingStrategy) error {
	nlbs, err := c.listAllNLBs()
	if err != nil {
		return err
	}
	// TODO: Check for the expected name?
	for _, networkLoadBalancer := range nlbs {
		if networkLoadBalancer.scheme == "internet-facing" {
			// nothing to do
			return nil
		}
	}
	// create new ext nlb
	infrastructureName, err := utils.GetClusterName(kclient)
	if err != nil {
		return err
	}
	extNLBName := infrastructureName + "-ext"
	subnets, err := utils.GetMasterNodeSubnets(kclient)
	if err != nil {
		return err
	}
	subnetIDs, err := c.subnetNameToSubnetIDLookup([]string{subnets["public"]})
	if err != nil {
		return err
	}
	newNLBs, err := c.createNetworkLoadBalancer(extNLBName, "internet-facing", subnetIDs[0])
	if err != nil {
		return err
	}
	if len(newNLBs) != 1 {
		return fmt.Errorf("more than one NLB, or no new NLB detected (expected 1, got %d)", len(newNLBs))
	}
	err = c.addTagsForNLB(newNLBs[0].loadBalancerArn, infrastructureName)
	if err != nil {
		return err
	}
	// attempt to use existing TargetGroup
	targetGroupName := fmt.Sprintf("%s-aext", infrastructureName)
	targetGroupARN, err := c.getTargetGroupArn(targetGroupName)
	if err != nil {
		return err
	}
	err = c.createListenerForNLB(targetGroupARN, newNLBs[0].loadBalancerArn)
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
	baseDomain, err := utils.GetClusterBaseDomain(kclient)
	if err != nil {
		return err
	}
	pubDomainName := baseDomain[strings.Index(baseDomain, ".")+1:]
	apiDNSName := fmt.Sprintf("api.%s.", baseDomain)
	// not tested yet
	comment := "Update api.<clusterName> alias to external NLB"
	err = c.upsertARecord(pubDomainName+".",
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

/* Helper functions below, sorted by AWS API type */

// ELB (v1)
func (c *Client) doesELBExist(elbName string) (*awsLoadBalancer, error) {
	input := &elb.DescribeLoadBalancersInput{
		LoadBalancerNames: []*string{aws.String(elbName)},
	}
	output, err := c.elbClient.DescribeLoadBalancers(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case elb.ErrCodeAccessPointNotFoundException:
				return &awsLoadBalancer{}, errors.NewLoadBalancerNotFoundError(elbName)
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

func (c *Client) ensureDNSForService(ctx context.Context, kclient client.Client, svc *corev1.Service, dnsName, dnsComment string) error {
	// Get the ELB name from the Service's UID. Truncate to 32 characters for AWS
	elbName := strings.ReplaceAll("a"+string(svc.ObjectMeta.UID), "-", "")[0:32]
	awsELB, err := c.doesELBExist(elbName)
	// Primarily checking to see if this exists. It is an error if it does not,
	// likely because AWS is still creating it and the Reconcile should be retried
	if err != nil {
		return err
	}
	// ELB exists, now let's set the DNS
	clusterBaseDomain, err := utils.GetClusterBaseDomain(kclient)
	if err != nil {
		return err
	}
	lb := &loadBalancer{
		endpointName: dnsName,
		baseDomain:   clusterBaseDomain,
	}
	return c.ensureDNSRecord(lb, awsELB, dnsComment)
}

// removeDNSForService will remove a DNS entry for a particular Service
func (c *Client) removeDNSForService(ctx context.Context, kclient client.Client, svc *corev1.Service, dnsName, dnsComment string) error {
	// Get the ELB name from the Service's UID. Truncate to 32 characters for AWS
	elbName := strings.ReplaceAll("a"+string(svc.ObjectMeta.UID), "-", "")[0:32]
	awsELB, err := c.doesELBExist(elbName)
	// Primarily checking to see if this exists. It is an error if it does not,
	// likely because AWS is still creating it and the Reconcile should be retried
	if err != nil {
		return err
	}
	// ELB exists, now let's set the DNS
	clusterBaseDomain, err := utils.GetClusterBaseDomain(kclient)
	if err != nil {
		return err
	}
	return c.ensureDNSRecordsRemoved(
		clusterBaseDomain,
		awsELB.dnsName,
		awsELB.dnsZoneID,
		dnsName+"."+clusterBaseDomain,
		dnsComment,
		false)
}

func (c *Client) deleteARecord(clusterDomain, DNSName, aliasDNSZoneID, resourceRecordSetName string, targetHealth bool) error {
	publicHostedZoneID, err := c.getPublicHostedZoneID(clusterDomain)
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
	_, err = c.route53Client.ChangeResourceRecordSets(change)
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

func (c *Client) upsertARecord(clusterDomain, DNSName, aliasDNSZoneID, resourceRecordSetName, comment string, targetHealth bool) error {
	publicHostedZoneID, err := c.getPublicHostedZoneID(clusterDomain)
	if err != nil {
		return err
	}
	change := &route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action: aws.String("UPSERT"),
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
			Comment: aws.String(comment),
		},
		HostedZoneId: aws.String(publicHostedZoneID),
	}
	_, err = c.route53Client.ChangeResourceRecordSets(change)
	return err
}

func (c *Client) getPublicHostedZoneID(clusterDomain string) (string, error) {
	input := &route53.ListHostedZonesByNameInput{
		DNSName: aws.String(clusterDomain),
	}
	output, err := c.route53Client.ListHostedZonesByName(input)
	if err != nil {
		return "", err
	}
	for _, zone := range output.HostedZones {
		if *zone.Name == clusterDomain {
			return path.Base(aws.StringValue(zone.Id)), nil
		}
	}

	return "", fmt.Errorf("Route53 Zone not found for %s", clusterDomain)

}

func (c *Client) ensureDNSRecord(lb *loadBalancer, awsObj *awsLoadBalancer, comment string) error {
	// private zone

	for i := 1; i <= config.MaxAPIRetries; i++ {
		err := c.upsertARecord(
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
		err := c.upsertARecord(
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
func (c *Client) ensureDNSRecordsRemoved(clusterDomain, DNSName, aliasDNSZoneID, resourceRecordSetName, comment string, targetHealth bool) error {
	for i := 1; i <= config.MaxAPIRetries; i++ {
		err := c.deleteARecord(
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
		err := c.deleteARecord(
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

// EC2

func (c *Client) subnetNameToSubnetIDLookup(subnetNames []string) ([]string, error) {
	r := make([]string, len(subnetNames))
	for i, name := range subnetNames {
		filter := []*ec2.Filter{{Name: aws.String("tag:Name"), Values: aws.StringSlice([]string{name})}}
		res, err := c.ec2Client.DescribeSubnets(&ec2.DescribeSubnetsInput{
			Filters: filter,
		})
		if err != nil {
			return []string{}, err
		}
		r[i] = *res.Subnets[0].SubnetId
	}
	return r, nil
}

// ELBv2

// removeLoadBalancerFromMasterNodes
func (c *Client) removeLoadBalancerFromMasterNodes(ctx context.Context, kclient client.Client) (string, string, error) {
	nlbs, err := c.listAllNLBs()
	if err != nil {
		return "", "", err
	}
	masterList, err := utils.GetMasterMachines(kclient)
	if err != nil {
		return "", "", err
	}
	var intDNSName, intHostedZoneID, lbName string
	for _, networkLoadBalancer := range nlbs {
		if networkLoadBalancer.scheme == "internet-facing" {
			lbName = networkLoadBalancer.loadBalancerName
			err := c.deleteExternalLoadBalancer(networkLoadBalancer.loadBalancerArn)
			if err != nil {
				return "", "", err
			}
			err = utils.RemoveAWSLBFromMasterMachines(kclient, lbName, masterList)
			if err != nil {
				return "", "", err
			}
		}
		// we need this to update DNS
		if networkLoadBalancer.scheme == "internal" {
			intDNSName = networkLoadBalancer.dnsName
			intHostedZoneID = networkLoadBalancer.canonicalHostedZoneNameID
		}
	}
	return intDNSName, intHostedZoneID, nil
}

// listAllNLBs uses the DescribeLoadBalancersV2 to get back a list of all
// Network Load Balancers
func (c *Client) listAllNLBs() ([]loadBalancerV2, error) {
	i := &elbv2.DescribeLoadBalancersInput{}
	output, err := c.elbv2Client.DescribeLoadBalancers(i)
	if err != nil {
		return []loadBalancerV2{}, err
	}
	loadBalancers := make([]loadBalancerV2, 0)
	for _, loadBalancer := range output.LoadBalancers {
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
func (c *Client) deleteExternalLoadBalancer(extLoadBalancerArn string) error {
	i := elbv2.DeleteLoadBalancerInput{
		LoadBalancerArn: aws.String(extLoadBalancerArn),
	}
	_, err := c.elbv2Client.DeleteLoadBalancer(&i)
	return err
}

// createNetworkLoadBalancer should only return one new NLB at a time
func (c *Client) createNetworkLoadBalancer(lbName, scheme, subnet string) ([]loadBalancerV2, error) {
	i := &elbv2.CreateLoadBalancerInput{
		Name:   aws.String(lbName),
		Scheme: aws.String(scheme),
		Subnets: []*string{
			aws.String(subnet),
		},
		Type: aws.String("network"),
	}

	result, err := c.elbv2Client.CreateLoadBalancer(i)
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
func (c *Client) createListenerForNLB(targetGroupArn, loadBalancerArn string) error {
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

	_, err := c.elbv2Client.CreateListener(i)
	if err != nil {
		return err
	}
	return nil
}

// addTagsForNLB creates needed tags for an NLB
func (c *Client) addTagsForNLB(resourceARN string, clusterName string) error {
	i := &elbv2.AddTagsInput{
		ResourceArns: []*string{
			aws.String(resourceARN), // ext nlb resources arn
		},
		Tags: []*elbv2.Tag{
			{
				Key:   aws.String("kubernetes.io/cluster/" + clusterName),
				Value: aws.String("owned"),
			},
			{
				Key:   aws.String("Name"),
				Value: aws.String(clusterName + "-ext"), //in form of samn-test-qb58m-ext
			},
		},
	}

	_, err := c.elbv2Client.AddTags(i)
	if err != nil {
		return err
	}
	return nil
}

// getTargetGroupArn by passing in targetGroup Name
func (c *Client) getTargetGroupArn(targetGroupName string) (string, error) {
	i := &elbv2.DescribeTargetGroupsInput{
		Names: []*string{
			aws.String(targetGroupName),
		},
	}

	result, err := c.elbv2Client.DescribeTargetGroups(i)
	if err != nil {
		return "", err
	}
	return aws.StringValue(result.TargetGroups[0].TargetGroupArn), nil
}
