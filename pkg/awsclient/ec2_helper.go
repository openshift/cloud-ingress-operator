package awsclient

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elb"
)

// FIXME: Add tags

// EnsureCIDRAccess ensures that for the given load balancer, the specified CIDR
// blocks, and only those blocks may access it.
// cidrBlocks always goes from 6443/TCP to 6443/TCP and is IPv4 only
// TODO: Expand to IPv6. This could be done by regular expression
func (c *awsClient) EnsureCIDRAccess(loadBalancerName, securityGroupName, vpcId string, cidrBlocks []string) error {
	// first need to see if the SecurityGroup exists, and if it does not, create it and populate its ingressCIDR permissions
	// If the SecurityGroup DOES exist, then make sure it only has the permissions we are receiving here.
	securityGroup, err := c.findSecurityGroupByName(securityGroupName)
	if err != nil {
		return err
	}
	if securityGroup == nil {
		// group does not exist, create it
		securityGroup, err = c.createSecurityGroup(securityGroupName, vpcId)
		if err != nil {
			return err
		}
	}
	// At this point, securityGroup is unified, no matter how we got it:
	// finding it, or creating it and so now we can reconcile the rules

	var rulesToRemove, rulesToAdd []*ec2.IpPermission

	// When processing all this SecurityGroup's ingress rules we compare
	// to cidrBlocks, but that doesn't always hit the expected set, so
	// this is a map to see if we have done just that. Any which are
	// false were not processed.
	seenExpectedRules := make(map[string]bool)
	// init map
	for _, cidrBlock := range cidrBlocks {
		seenExpectedRules[cidrBlock] = false
	}
	// For each ingress rule for the security group,
Outer:
	for _, ingressRule := range securityGroup.IpPermissions {
		// Only care about 6443/TCP -> 6443/TCP
		if *ingressRule.FromPort != 6443 &&
			*ingressRule.ToPort != 6443 &&
			*ingressRule.IpProtocol != "tcp" {
			continue
		}
		for _, cidrBlock := range cidrBlocks {
			// Note: For now, we assume that ingressRule.IpRange is length 1 as that
			// appears to be the usage inside AWS.
			if *ingressRule.IpRanges[0].CidrIp == cidrBlock {
				seenExpectedRules[cidrBlock] = true
				// No need to continue on this ingressRule, because we seen it
				continue Outer
			}
		}
		// If we didn't encounter our rule in the expected list of CIDR blocks, then
		// we should remove it
		// Note: This isn't the end of the story since it's still possible that we
		// have a rule that should have been added and wasn't in the permissions
		// for this security group.
		rulesToRemove = append(rulesToRemove, ingressRule)
	}
	for cidrBlock, seen := range seenExpectedRules {
		if !seen {
			rulesToAdd = append(rulesToAdd, &ec2.IpPermission{
				FromPort:   aws.Int64(6443),
				ToPort:     aws.Int64(6443),
				IpProtocol: aws.String("tcp"),
				IpRanges: []*ec2.IpRange{
					{
						CidrIp:      aws.String(cidrBlock),
						Description: aws.String("Approved CIDR Block from cloud-ingress-operator configuration"),
					},
				},
			})
		}
	}
	if err := c.addIngressRulesToSecurityGroup(securityGroup, rulesToAdd); err != nil {
		return err
	}
	if err := c.removeIngressRulesFromSecurityGroup(securityGroup, rulesToRemove); err != nil {
		return err
	}
	// Once the ingress rules are updated, attach the SecurityGroup to the load balancer
	return c.setLoadBalancerSecurityGroup(loadBalancerName, securityGroup)
}

// Add rules to the security group
func (c *awsClient) addIngressRulesToSecurityGroup(securityGroup *ec2.SecurityGroup, ipPermissions []*ec2.IpPermission) error {
	if len(ipPermissions) == 0 {
		// nothing to do
		return nil
	}
	i := &ec2.AuthorizeSecurityGroupIngressInput{
		IpPermissions: ipPermissions,
		GroupId:       securityGroup.GroupId,
	}
	_, err := c.AuthorizeSecurityGroupIngress(i)
	return err
}

// Remove rules from the security group
func (c *awsClient) removeIngressRulesFromSecurityGroup(securityGroup *ec2.SecurityGroup, ipPermissions []*ec2.IpPermission) error {
	if len(ipPermissions) == 0 {
		// nothing   to do
		return nil
	}
	i := &ec2.RevokeSecurityGroupIngressInput{
		FromPort:      aws.Int64(6443),
		ToPort:        aws.Int64(6443),
		IpProtocol:    aws.String("tcp"),
		IpPermissions: ipPermissions,
		GroupId:       securityGroup.GroupId,
	}
	_, err := c.RevokeSecurityGroupIngress(i)
	return err
}

// createSecurityGroup creates a SecurityGroup with the given name, and returns the EC2 object and/or any error
func (c *awsClient) createSecurityGroup(securityGroupName, vpcId string) (*ec2.SecurityGroup, error) {
	createInput := &ec2.CreateSecurityGroupInput{
		Description: aws.String("Admin API Security group"),
		GroupName:   aws.String(securityGroupName),
		VpcId:       aws.String(vpcId),
	}
	createResult, err := c.CreateSecurityGroup(createInput)
	if err != nil {
		return nil, err
	}

	// Caller of this method wants a *ec2.SecurityGroup, and since the create
	// method doesn't give us nought but the group-id, we have to do a search
	// to find it.
	return c.findSecurityGroupByID(*createResult.GroupId)
}

func (c *awsClient) findSecurityGroupByID(id string) (*ec2.SecurityGroup, error) {
	i := &ec2.DescribeSecurityGroupsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("group-id"),
				Values: aws.StringSlice([]string{id}),
			},
		},
	}
	o, err := c.DescribeSecurityGroups(i)
	if err != nil {
		return nil, err
	}
	if len(o.SecurityGroups) == 0 {
		return nil, nil
	}
	return o.SecurityGroups[0], nil
}

func (c *awsClient) findSecurityGroupByName(name string) (*ec2.SecurityGroup, error) {
	i := &ec2.DescribeSecurityGroupsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("group-name"),
				Values: aws.StringSlice([]string{name}),
			},
		},
	}
	o, err := c.DescribeSecurityGroups(i)
	if err != nil {
		return nil, err
	}
	if len(o.SecurityGroups) == 0 {
		return nil, nil
	}
	return o.SecurityGroups[0], nil
}

// Add a SecurityGroup to a load balancer. This is an idempotent operation
func (c *awsClient) setLoadBalancerSecurityGroup(loadBalancerName string, securityGroup *ec2.SecurityGroup) error {
	i := &elb.ApplySecurityGroupsToLoadBalancerInput{
		LoadBalancerName: aws.String(loadBalancerName),
		SecurityGroups:   aws.StringSlice([]string{*securityGroup.GroupId}),
	}
	_, err := c.ApplySecurityGroupsToLoadBalancer(i)
	return err
}
