package awsclient

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/route53"

	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/elb/elbiface"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"
)

// Get load balancers
// delete targets from load balancers
// add targets to load balancers
// get target groups
// manipulate DNS zones

// Client wraps for AWS SDK (for easier testing)
type Client interface {
	// ELB - to make the api endpoint, and toggle the customer ones
	CreateLoadBalancer(*elb.CreateLoadBalancerInput) (*elb.CreateLoadBalancerOutput, error)
	// for making api. public, and creation of rh-api.
	CreateLoadBalancerListeners(*elb.CreateLoadBalancerListenersInput) (*elb.CreateLoadBalancerListenersOutput, error)
	// list all (or 1) load balancer to see if we need to create rh-api, and to identify api. AWS identifier
	DescribeLoadBalancers(*elb.DescribeLoadBalancersInput) (*elb.DescribeLoadBalancersOutput, error)
	// to check if it's been annotated with a k8s ownership tag
	DescribeTags(*elb.DescribeTagsInput) (*elb.DescribeTagsOutput, error)
	// for making the api. endpoint private (just delete the listeners so it doesn't need to be recreated)
	DeleteLoadBalancerListeners(*elb.DeleteLoadBalancerListenersInput) (*elb.DeleteLoadBalancerListenersOutput, error)

	// ELBv2 - to figure out which to assign back to the nlb
	DescribeTargetGroups(*elbv2.DescribeTargetGroupsInput) (*elbv2.DescribeTargetGroupsOutput, error)

	// Route 53 - to update DNS for internal/external swap and to add rh-api
	// for actually upserting the record
	ChangeResourceRecordSets(*route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error)
	// to turn baseDomain into a Route53 zone ID
	ListHostedZones(*route53.ListHostedZonesInput) (*route53.ListHostedZonesOutput, error)
	// does rh-api already exist? maybe a DNS lookup checking for NXDOMAIN is better/cheaper?
	ListResourceRecordSets(*route53.ListResourceRecordSetsInput) (*route53.ListResourceRecordSetsOutput, error)

	// EC2 - to create the security group for the new admin api
	// we can get the instance IDs from Node objects.
	AuthorizeSecurityGroupIngress(*ec2.AuthorizeSecurityGroupIngressInput) (*ec2.AuthorizeSecurityGroupIngressOutput, error)
	// for rh-api.
	CreateSecurityGroup(*ec2.CreateSecurityGroupInput) (*ec2.CreateSecurityGroupOutput, error)
	// for removing a formerly approved CIDR block from the rh-api. security group
	DeleteSecurityGroup(*ec2.DeleteSecurityGroupInput) (*ec2.DeleteSecurityGroupOutput, error)
	// to determine if we need to create the rh-api. security group
	DescribeSecurityGroups(*ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error)
	// for removing a formerly approved CIDR block from the rh-api. security group
	RevokeSecurityGroupIngress(*ec2.RevokeSecurityGroupIngressInput) (*ec2.RevokeSecurityGroupIngressOutput, error)
}

type awsClient struct {
	ec2Client     ec2iface.EC2API
	route53Client route53iface.Route53API
	elbClient     elbiface.ELBAPI
	elbv2Client   elbv2iface.ELBV2API
}

func NewClient(accessID, accessSecret, token, region string) (Client, error) {
	awsConfig := aws.Config{Region: aws.String(region)}
	awsConfig.Credentials = credentials.NewStaticCredentials(accessID, accessSecret, token)
	s, err := session.NewSession(&awsConfig)
	if err != nil {
		return nil, err
	}
	return &awsClient{
		ec2Client:     ec2.New(s),
		elbClient:     elb.New(s),
		elbv2Client:   elbv2.New(s),
		route53Client: route53.New(s),
	}, nil
}

func (c *awsClient) CreateLoadBalancer(i *elb.CreateLoadBalancerInput) (*elb.CreateLoadBalancerOutput, error) {
	return c.elbClient.CreateLoadBalancer(i)
}

func (c *awsClient) CreateLoadBalancerListeners(i *elb.CreateLoadBalancerListenersInput) (*elb.CreateLoadBalancerListenersOutput, error) {
	return c.elbClient.CreateLoadBalancerListeners(i)
}

func (c *awsClient) DescribeLoadBalancers(i *elb.DescribeLoadBalancersInput) (*elb.DescribeLoadBalancersOutput, error) {
	return c.elbClient.DescribeLoadBalancers(i)
}

func (c *awsClient) DescribeTags(i *elb.DescribeTagsInput) (*elb.DescribeTagsOutput, error) {
	return c.elbClient.DescribeTags(i)
}

func (c *awsClient) DeleteLoadBalancerListeners(i *elb.DeleteLoadBalancerListenersInput) (*elb.DeleteLoadBalancerListenersOutput, error) {
	return c.elbClient.DeleteLoadBalancerListeners(i)
}

func (c *awsClient) DescribeTargetGroups(i *elbv2.DescribeTargetGroupsInput) (*elbv2.DescribeTargetGroupsOutput, error) {
	return c.elbv2Client.DescribeTargetGroups(i)
}

func (c *awsClient) ChangeResourceRecordSets(i *route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error) {
	return c.route53Client.ChangeResourceRecordSets(i)
}

func (c *awsClient) ListHostedZones(i *route53.ListHostedZonesInput) (*route53.ListHostedZonesOutput, error) {
	return c.route53Client.ListHostedZones(i)
}

func (c *awsClient) ListResourceRecordSets(i *route53.ListResourceRecordSetsInput) (*route53.ListResourceRecordSetsOutput, error) {
	return c.route53Client.ListResourceRecordSets(i)
}

func (c *awsClient) AuthorizeSecurityGroupIngress(i *ec2.AuthorizeSecurityGroupIngressInput) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
	return c.ec2Client.AuthorizeSecurityGroupIngress(i)
}
func (c *awsClient) CreateSecurityGroup(i *ec2.CreateSecurityGroupInput) (*ec2.CreateSecurityGroupOutput, error) {
	return c.ec2Client.CreateSecurityGroup(i)
}
func (c *awsClient) DeleteSecurityGroup(i *ec2.DeleteSecurityGroupInput) (*ec2.DeleteSecurityGroupOutput, error) {
	return c.ec2Client.DeleteSecurityGroup(i)
}
func (c *awsClient) DescribeSecurityGroups(i *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
	return c.ec2Client.DescribeSecurityGroups(i)
}
func (c *awsClient) RevokeSecurityGroupIngress(i *ec2.RevokeSecurityGroupIngressInput) (*ec2.RevokeSecurityGroupIngressOutput, error) {
	return c.ec2Client.RevokeSecurityGroupIngress(i)
}
