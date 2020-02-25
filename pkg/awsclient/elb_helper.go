package awsclient

import (
	"fmt"

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

// CreateClassicELB creates a classic ELB in Amazon, as in for management API endpoint.
// inputs are the name of the ELB, the availability zone(s) and subnet(s) the
// ELB should attend, as well as the listener port.
// The port is used for the instance port and load balancer port
// Return is the (FQDN) DNS name from Amazon, and error, if any.
func (c *AwsClient) CreateClassicELB(elbName string, subnets []string, listenerPort int64, tagList map[string]string) (*AWSLoadBalancer, error) {
	tags := make([]*elb.Tag, 0)
	for k, v := range tagList {
		tags = append(tags, &elb.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}

	i := &elb.CreateLoadBalancerInput{
		LoadBalancerName: aws.String(elbName),
		Subnets:          aws.StringSlice(subnets),
		//AvailabilityZones: aws.StringSlice(availabilityZones),
		Listeners: []*elb.Listener{
			{
				InstancePort:     aws.Int64(listenerPort),
				InstanceProtocol: aws.String("tcp"),
				Protocol:         aws.String("tcp"),
				LoadBalancerPort: aws.Int64(listenerPort),
			},
		},
		Tags: tags,
	}
	_, err := c.CreateLoadBalancer(i)
	if err != nil {
		return &AWSLoadBalancer{}, err
	}
	err = c.addHealthCheck(elbName, "HTTP", "/", 6443)
	if err != nil {
		return &AWSLoadBalancer{}, err
	}
	// Caller will need the DNS name and Zone ID for the ELB (for route53) so let's make a handy object to return, using the
	_, awsELBObj, err := c.DoesELBExist(elbName)
	if err != nil {
		return &AWSLoadBalancer{}, err
	}
	return awsELBObj, nil
}

// SetLoadBalancerPrivate sets a load balancer private by removing its
// listeners (port 6443/TCP)
func (c *AwsClient) SetLoadBalancerPrivate(elbName string) error {

	return c.removeListenersFromELB(elbName)
}

// SetLoadBalancerPublic will set the specified load balancer public by
// re-adding the 6443/TCP -> 6443/TCP listener. Any instances (still)
// attached to the load balancer will begin to receive traffic.
func (c *AwsClient) SetLoadBalancerPublic(elbName string, listenerPort int64) error {

	l := []*elb.Listener{
		{
			InstancePort:     aws.Int64(listenerPort),
			InstanceProtocol: aws.String("tcp"),
			Protocol:         aws.String("tcp"),
			LoadBalancerPort: aws.Int64(listenerPort),
		},
	}
	return c.addListenersToELB(elbName, l)
}

// removeListenersFromELB will remove the 6443/TCP -> 6443/TCP listener from
// the specified ELB. This is useful when the "ext" ELB is to be no longer
// publicly accessible
func (c *AwsClient) removeListenersFromELB(elbName string) error {

	i := &elb.DeleteLoadBalancerListenersInput{
		LoadBalancerName:  aws.String(elbName),
		LoadBalancerPorts: aws.Int64Slice([]int64{6443}),
	}
	_, err := c.DeleteLoadBalancerListeners(i)
	return err
}

// addListenersToELB will add the +listeners+ to the specified ELB. This is
// useful for when the "ext" ELB is to be publicly accessible. See also
// removeListenersFromELB.
// Note: This will likely always want to be given 6443/tcp -> 6443/tcp for
// the kube-api
func (c *AwsClient) addListenersToELB(elbName string, listeners []*elb.Listener) error {

	i := &elb.CreateLoadBalancerListenersInput{
		Listeners:        listeners,
		LoadBalancerName: aws.String(elbName),
	}
	_, err := c.CreateLoadBalancerListeners(i)
	return err
}

// AddLoadBalancerInstances will attach +instanceIds+ to +elbName+
// so that they begin to receive traffic. Note that this takes an amount of
// time to return. This is also additive (but idempotent - TODO: Validate this).
// Note that the recommended steps:
// 1. stop the instance,
// 2. deregister the instance,
// 3. start the instance,
// 4. and then register the instance.
func (c *AwsClient) AddLoadBalancerInstances(elbName string, instanceIds []string) error {

	instances := make([]*elb.Instance, 0)
	for _, instance := range instanceIds {
		instances = append(instances, &elb.Instance{InstanceId: aws.String(instance)})
	}
	i := &elb.RegisterInstancesWithLoadBalancerInput{
		Instances:        instances,
		LoadBalancerName: aws.String(elbName),
	}
	_, err := c.RegisterInstancesWithLoadBalancer(i)
	return err
}

// RemoveInstancesFromLoadBalancer removes +instanceIds+ from +elbName+, eg when an Node is deleted.
func (c *AwsClient) RemoveInstancesFromLoadBalancer(elbName string, instanceIds []string) error {

	instances := make([]*elb.Instance, 0)
	for _, instance := range instanceIds {
		instances = append(instances, &elb.Instance{InstanceId: aws.String(instance)})
	}
	i := &elb.DeregisterInstancesFromLoadBalancerInput{
		Instances:        instances,
		LoadBalancerName: aws.String(elbName),
	}
	_, err := c.DeregisterInstancesFromLoadBalancer(i)
	return err
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

func (c *AwsClient) addHealthCheck(loadBalancerName, protocol, path string, port int64) error {
	i := &elb.ConfigureHealthCheckInput{
		HealthCheck: &elb.HealthCheck{
			HealthyThreshold:   aws.Int64(2),
			Interval:           aws.Int64(30),
			Target:             aws.String(fmt.Sprintf("%s:%d%s", protocol, port, path)),
			Timeout:            aws.Int64(3),
			UnhealthyThreshold: aws.Int64(2),
		},
		LoadBalancerName: aws.String(loadBalancerName),
	}
	_, err := c.ConfigureHealthCheck(i)
	return err
}
