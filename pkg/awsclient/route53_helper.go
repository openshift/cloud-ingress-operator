package awsclient

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
)

// AddManagementDNSRecord will add the admin API resource record
// +adminAPIName+ (eg rh-api) as a CNAME alias to +elbFQDN+ in the
// +clusterDomain+ Route 53 zone.
func (c *awsClient) AddManagementDNSRecord(elbFQDN, clusterDomain, adminAPIName string, ttl int) error {
	// Look up the clusterDomain to get ID
	lookup := &route53.ListHostedZonesByNameInput{
		DNSName: aws.String(clusterDomain),
	}
	results, err := c.ListHostedZonesByName(lookup)
	if err != nil {
		return err
	}

	change := &route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action: aws.String("UPSERT"),
					ResourceRecordSet: &route53.ResourceRecordSet{
						Name: aws.String(adminAPIName + "." + clusterDomain),
						ResourceRecords: []*route53.ResourceRecord{
							{
								Value: aws.String(elbFQDN),
							},
						},
						TTL:  aws.Int64(60),
						Type: aws.String("CNAME"),
					},
				},
			},
			Comment: aws.String("RH API Endpoint"),
		},
		HostedZoneId: results.HostedZones[0].Id,
	}
	_, err = c.ChangeResourceRecordSets(change)
	if err != nil {
		return err
	}
	return nil
}
