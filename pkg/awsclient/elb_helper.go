package awsclient

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elb"
)

// CreateClassicELB creates a classic ELB in Amazon, as in for management API endpoint.
// inputs are the name of the ELB, the availability zone(s) and subnet(s) the
// ELB should attend, as well as the listener port.
// The port is used for the instance port and load balancer port
// Return is the (FQDN) DNS name from Amazon, and error, if any.
func (c *awsClient) CreateClassicELB(elbName string, availabilityZones, subnets []string, listenerPort int64) (string, error) {
	i := &elb.CreateLoadBalancerInput{
		LoadBalancerName:  aws.String(elbName),
		Subnets:           aws.StringSlice(subnets),
		AvailabilityZones: aws.StringSlice(availabilityZones),
		Listeners: []*elb.Listener{
			{
				InstancePort:     aws.Int64(listenerPort),
				InstanceProtocol: aws.String("TCP"),
				Protocol:         aws.String("TCP"),
				LoadBalancerPort: aws.Int64(listenerPort),
			},
		},
	}
	o, err := c.CreateLoadBalancer(i)
	if err != nil {
		return "", err
	}
	return *o.DNSName, nil
}

// DoesELBExist checks for the existence of an ELB by name. If there's an AWS
// error it is returned.
func (c *awsClient) DoesELBExist(elbName string) (bool, error) {
	i := &elb.DescribeLoadBalancersInput{
		LoadBalancerNames: []*string{aws.String(elbName)},
	}
	res, err := c.DescribeLoadBalancers(i)
	if err != nil {
		return false, err
	}
	if len(res.LoadBalancerDescriptions) == 1 {
		return true, nil
	}
	return false, nil
}
