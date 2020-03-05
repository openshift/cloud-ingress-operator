package awsclient

import (
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
)

// UpsertARecord adds an A record alias named DNSName in the target zone aliasDNSZoneID, inside the clusterDomain's zone.
func (c *AwsClient) UpsertARecord(clusterDomain, DNSName, aliasDNSZoneID, resourceRecordSetName, comment string, targetHealth bool) error {
	// look up clusterDomain to get hostedzoneID
	lookup := &route53.ListHostedZonesByNameInput{
		DNSName: aws.String(clusterDomain),
	}

	listHostedZones, err := c.ListHostedZonesByName(lookup)
	if err != nil {
		return err
	}

	// get public hosted zone ID needed to changeResourceRecordSets
	var publicHostedZoneID string
	for _, zone := range listHostedZones.HostedZones {
		if *zone.Name == clusterDomain {
			// In order to get the publicHostedZoneID we need to get
			// the HostedZone.Id object which is in the form of "/hostedzone/Z1P3C0HZA40C0N"
			// Since we only care about the ID number, we take index of the last "/" char and parse right
			zoneID := aws.StringValue(zone.Id)
			slashIndex := strings.LastIndex(zoneID, "/")
			publicHostedZoneID = zoneID[slashIndex+1:]
			break
		}
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
