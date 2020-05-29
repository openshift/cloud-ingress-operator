package awsclient

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/elbv2"
)

// AWSLoadBalancer a handy way to return information about an ELB
type AWSLoadBalancer struct {
	ELBName   string // Name of the ELB
	DNSName   string // DNS Name of the ELB
	DNSZoneId string // Zone ID
}

// DoesELBExist checks for the existence of an ELB by name. If there's an AWS
// error it is returned.
func (c *AwsClient) DoesELBExist(elbName string) (bool, *AWSLoadBalancer, error) {

	i := &elb.DescribeLoadBalancersInput{
		LoadBalancerNames: []*string{aws.String(elbName)},
	}
	res, err := c.DescribeLoadBalancers(i)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case elb.ErrCodeAccessPointNotFoundException:
				return false, &AWSLoadBalancer{}, nil
			default:
				return false, &AWSLoadBalancer{}, err
			}
		}
	}
	return true, &AWSLoadBalancer{ELBName: elbName, DNSName: *res.LoadBalancerDescriptions[0].DNSName, DNSZoneId: *res.LoadBalancerDescriptions[0].CanonicalHostedZoneNameID}, nil
}

// LoadBalancerV2 is a list of all non-classic ELBs
type LoadBalancerV2 struct {
	CanonicalHostedZoneNameID string
	DNSName                   string
	LoadBalancerArn           string
	LoadBalancerName          string
	Scheme                    string
	VpcID                     string
}

// ListAllNLBs uses the DescribeLoadBalancersV2 to get back a list of all Network Load Balancers
func (c *AwsClient) ListAllNLBs() ([]LoadBalancerV2, error) {

	i := &elbv2.DescribeLoadBalancersInput{}
	output, err := c.DescribeLoadBalancersV2(i)
	if err != nil {
		return []LoadBalancerV2{}, err
	}
	loadBalancers := make([]LoadBalancerV2, 0)
	for _, loadBalancer := range output.LoadBalancers {
		loadBalancers = append(loadBalancers, LoadBalancerV2{
			CanonicalHostedZoneNameID: aws.StringValue(loadBalancer.CanonicalHostedZoneId),
			DNSName:                   aws.StringValue(loadBalancer.DNSName),
			LoadBalancerArn:           aws.StringValue(loadBalancer.LoadBalancerArn),
			LoadBalancerName:          aws.StringValue(loadBalancer.LoadBalancerName),
			Scheme:                    aws.StringValue(loadBalancer.Scheme),
			VpcID:                     aws.StringValue(loadBalancer.VpcId),
		})
	}
	return loadBalancers, nil
}

// DeleteExternalLoadBalancer takes in the external LB arn and deletes the entire LB
func (c *AwsClient) DeleteExternalLoadBalancer(extLoadBalancerArn string) error {
	i := elbv2.DeleteLoadBalancerInput{
		LoadBalancerArn: aws.String(extLoadBalancerArn),
	}
	_, err := c.DeleteLoadBalancerV2(&i)
	return err
}

// CreateNetworkLoadBalancer should only return one new NLB at a time
func (c *AwsClient) CreateNetworkLoadBalancer(lbName, scheme, subnet string) ([]LoadBalancerV2, error) {
	i := &elbv2.CreateLoadBalancerInput{
		Name:   aws.String(lbName),
		Scheme: aws.String(scheme),
		Subnets: []*string{
			aws.String(subnet),
		},
		Type: aws.String("network"),
	}

	result, err := c.CreateLoadBalancerV2(i)
	if err != nil {
		return []LoadBalancerV2{}, err
	}

	// there should only be 1 NLB made, but since CreateLoadBalancerOutput takes in slice
	// we return it as slice
	loadBalancers := make([]LoadBalancerV2, 0)
	for _, loadBalancer := range result.LoadBalancers {
		loadBalancers = append(loadBalancers, LoadBalancerV2{
			CanonicalHostedZoneNameID: aws.StringValue(loadBalancer.CanonicalHostedZoneId),
			DNSName:                   aws.StringValue(loadBalancer.DNSName),
			LoadBalancerArn:           aws.StringValue(loadBalancer.LoadBalancerArn),
			LoadBalancerName:          aws.StringValue(loadBalancer.LoadBalancerName),
			Scheme:                    aws.StringValue(loadBalancer.Scheme),
			VpcID:                     aws.StringValue(loadBalancer.VpcId),
		})
	}
	return loadBalancers, nil
}

// CreateListenerForNLB creates a listener between target group and nlb given their arn
func (c *AwsClient) CreateListenerForNLB(targetGroupArn, loadBalancerArn string) error {
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

	_, err := c.CreateListenerV2(i)
	if err != nil {
		return err
	}
	return nil
}

// AddTagsForNLB creates needed tags for an NLB
func (c *AwsClient) AddTagsForNLB(resourceARN string, clusterName string) error {
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

	_, err := c.AddTagsV2(i)
	if err != nil {
		return err
	}
	return nil
}

// GetTargetGroupArn by passing in targetGroup Name
func (c *AwsClient) GetTargetGroupArn(targetGroupName string) (string, error) {
	i := &elbv2.DescribeTargetGroupsInput{
		Names: []*string{
			aws.String(targetGroupName),
		},
	}

	result, err := c.DescribeTargetGroupsV2(i)
	if err != nil {
		return "", err
	}
	return aws.StringValue(result.TargetGroups[0].TargetGroupArn), nil
}
