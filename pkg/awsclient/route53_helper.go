package awsclient

import (
	"path"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
)

// GetPublicHostedZoneID looks up the ID of the public hosted zone for clusterDomain.
func (c *AwsClient) GetPublicHostedZoneID(clusterDomain string) (string, error) {
	input := &route53.ListHostedZonesByNameInput{
		DNSName: aws.String(clusterDomain),
	}
	output, err := c.ListHostedZonesByName(input)
	if err != nil {
		return "", err
	}

	var publicHostedZoneID string
	for _, zone := range output.HostedZones {
		if *zone.Name == clusterDomain {
			// The zone ID is the last element of the string
			// HostedZone.Id, which takes the form of a path:
			// "/hostedzone/<ZONEID>"
			publicHostedZoneID = path.Base(aws.StringValue(zone.Id))
			break
		}
	}

	return publicHostedZoneID, nil
}

// UpsertARecord adds an A record alias named DNSName in the target zone aliasDNSZoneID, inside the clusterDomain's zone.
func (c *AwsClient) UpsertARecord(clusterDomain, DNSName, aliasDNSZoneID, resourceRecordSetName, comment string, targetHealth bool) error {
	publicHostedZoneID, err := c.GetPublicHostedZoneID(clusterDomain)
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

	_, err = c.ChangeResourceRecordSets(change)
	if err != nil {
		return err
	}
	return nil
}
