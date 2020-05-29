package awsclient

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
)

// SubnetNameToSubnetIDLookup takes a slice of names and turns them into IDs.
// The return is the same order as the names: name[0] -> return[0]
func (c *AwsClient) SubnetNameToSubnetIDLookup(subnetNames []string) ([]string, error) {
	r := make([]string, len(subnetNames))
	for i, name := range subnetNames {
		filter := []*ec2.Filter{{Name: aws.String("tag:Name"), Values: aws.StringSlice([]string{name})}}
		res, err := c.DescribeSubnets(&ec2.DescribeSubnetsInput{
			Filters: filter,
		})
		if err != nil {
			return []string{}, err
		}
		r[i] = *res.Subnets[0].SubnetId
	}

	return r, nil
}
