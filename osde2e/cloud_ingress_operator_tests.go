// DO NOT REMOVE TAGS BELOW. IF ANY NEW TEST FILES ARE CREATED UNDER /osde2e, PLEASE ADD THESE TAGS TO THEM IN ORDER TO BE EXCLUDED FROM UNIT TESTS. //go:build osde2e
//go:build osde2e
// +build osde2e

package osde2etests

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift/osde2e-common/pkg/clients/openshift"
	"golang.org/x/oauth2/google"
	computev1 "google.golang.org/api/compute/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	logger "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/aws/aws-sdk-go/aws/awserr"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elb"
	"google.golang.org/api/option"
)

var _ = ginkgo.Describe("cloud-ingress-operator", ginkgo.Ordered, func() {
	var k8s *openshift.Client
	var region string
	var provider string
	var sts bool

	ginkgo.BeforeAll(func(ctx context.Context) {
		logger.SetLogger(ginkgo.GinkgoLogr)
		var err error
		k8s, err = openshift.New(ginkgo.GinkgoLogr)
		Expect(err).ShouldNot(HaveOccurred(), "Unable to setup k8s client")

		sts, err = k8s.IsSTS(ctx)
		Expect(err).NotTo(HaveOccurred(), "Could not determine STS config")

		if sts {
			ginkgo.Skip("Skipping sts clusters")
		}

		provider, err = k8s.GetProvider(ctx)
		Expect(err).NotTo(HaveOccurred(), "Could not determine provider")

		region, err = k8s.GetRegion(ctx)
		Expect(err).NotTo(HaveOccurred(), "Could not determine region")
	})

	if provider == "aws" {
		ginkgo.It("manually deleted rh-api load balancer should be recreated in AWS", func(ctx context.Context) {
			awsAccessKey := os.Getenv("AWS_ACCESS_KEY_ID")
			awsSecretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
			Expect(awsAccessKey).NotTo(BeEmpty(), "awsAccessKey not found")
			Expect(awsSecretKey).NotTo(BeEmpty(), "awsSecretKey not found")

			ginkgo.By("Getting old rh-api load balancer name")
			oldLBName, err := getLBForService(ctx, k8s, "openshift-kube-apiserver", "rh-api")
			Expect(err).NotTo(HaveOccurred(), "No existing rh-api service found")
			log.Printf("Old load balancer name %s ", oldLBName)

			// delete the load balancer in aws
			awsSession, err := session.NewSession(aws.NewConfig().WithCredentials(credentials.NewStaticCredentials(awsAccessKey, awsSecretKey, "")).WithRegion(region))
			Expect(err).NotTo(HaveOccurred(), "Could not set up aws session")

			ginkgo.By("Initializing AWS ELB service")
			lb := elb.New(awsSession)
			input := &elb.DeleteLoadBalancerInput{
				LoadBalancerName: aws.String(oldLBName),
			}

			// must store security groups associated with LB, so we can delete them
			oldLBDesc, err := lb.DescribeLoadBalancersWithContext(ctx, &elb.DescribeLoadBalancersInput{
				LoadBalancerNames: []*string{aws.String(oldLBName)},
			})
			Expect(err).NotTo(HaveOccurred(), "Could not describe old load balancer")
			orphanSecGroupIds := oldLBDesc.LoadBalancerDescriptions[0].SecurityGroups

			ginkgo.By("Deleting old rh-api load balancer")
			_, err = lb.DeleteLoadBalancer(input)
			Expect(err).NotTo(HaveOccurred(), "Could not delete rh-api lb")
			log.Printf("Old rh-api load balancer delete initiated")

			ginkgo.By("Waiting for rh-api service reconcile")
			err = wait.PollUntilContextTimeout(ctx, 15*time.Second, 5*time.Minute, false, func(ctx2 context.Context) (bool, error) {
				newLBName, err := getLBForService(ctx2, k8s, "openshift-kube-apiserver", "rh-api")
				log.Printf("Looking for new load balancer")

				if err != nil || newLBName == "" {
					// either we couldn't retrieve the LB name, or it wasn't created yet
					log.Printf("New load balancer not found yet...")
					return false, nil
				}
				if newLBName != oldLBName {
					// the LB was successfully recreated
					log.Printf("Reconciliation succeeded. New load balancer name: %s", newLBName)
					return true, nil
				}
				// the rh-api svc hasn't been deleted yet
				log.Printf("Old rh-api service not deleted yet...")
				return false, nil
			})
			Expect(err).NotTo(HaveOccurred(), "rh-api service did not reconcile")

			// old LB's security groups ("orphans") will leak if not explicitly deleted
			// first, delete sec group rule references to the orphans
			ec2Svc := ec2.New(awsSession)
			ginkgo.By("Cleaning up references to security groups orphaned by old LB deletion")
			err = deleteSecGroupReferencesToOrphans(ec2Svc, orphanSecGroupIds)
			Expect(err).NotTo(HaveOccurred(), "Error cleaning up after test")

			// then delete the orphaned sec groups themselves
			for _, orphanSecGroupId := range orphanSecGroupIds {
				_, err := ec2Svc.DeleteSecurityGroupWithContext(ctx, &ec2.DeleteSecurityGroupInput{
					GroupId: aws.String(*orphanSecGroupId),
				})
				if err != nil {
					log.Printf("Failed to delete security group %s: %s", *orphanSecGroupId, err)
				} else {
					log.Printf("Deleted orphaned security group %s", *orphanSecGroupId)
				}
			}
		})
	}

	if provider == "gcp" {
		ginkgo.It("manually deleted rh-api forwarding rule should be recreated in GCP", func(ctx context.Context) {
			region := os.Getenv("CLOUD_PROVIDER_REGION")
			Expect(region).NotTo(BeEmpty(), "No CLOUD_PROVIDER_REGION set")

			ginkgo.By("Getting current rh-api ip")
			oldLBIP, err := getLBForService(ctx, k8s, "openshift-kube-apiserver", "rh-api")
			Expect(err).NotTo(HaveOccurred(), "No existing rh-api service found")
			log.Printf("Old forwarding rule IP:  %s ", oldLBIP)

			ginkgo.By("Getting GCP creds")
			gcpCreds, status := getGCPCreds(ctx, k8s)
			Expect(status).To(BeTrue(), "GCP creds not created")
			project := gcpCreds.ProjectID

			ginkgo.By("Initializing GCP compute service")
			computeService, err := computev1.NewService(ctx, option.WithCredentials(gcpCreds), option.WithScopes("https://www.googleapis.com/auth/compute"))
			Expect(err).NotTo(HaveOccurred(), "Could not initialize GCP compute service")

			ginkgo.By("Getting GCP forwarding rule for rh-api")
			oldLB, err := getGCPForwardingRuleForIP(computeService, oldLBIP, project, region)
			Expect(err).NotTo(HaveOccurred(), "Could not get forwarding rule for rh-api")

			// There's no single command to delete a load balancer in GCP
			// Deletion of any related cloud resources may delete in misconfiguration.
			// Delete all GCP resources related to rh-api LB setup
			ginkgo.By("Deleting GCP forwarding rule for rh-api")
			if oldLB == nil {
				log.Printf("GCP forwarding rule for rh-api does not exist; Skipping deletion ")
			} else {
				log.Printf("Old forwarding rule name:  %s ", oldLB.Name)
				_, err = computeService.ForwardingRules.Get(project, region, oldLB.Name).Do()
				if err != nil {
					log.Printf("GCP forwarding rule for rh-api not found")
				} else {
					ginkgo.By("Deleting GCP forwarding rule for rh-api")
					_, err = computeService.ForwardingRules.Delete(project, region, oldLB.Name).Do()
					if err != nil {
						log.Printf("Error deleting forwarding rule")
					}
				}

				ginkgo.By("Deleting GCP backend service rule for rh-api")
				_, err = computeService.BackendServices.Get(project, oldLB.Name).Do()
				if err != nil {
					log.Printf("GCP backend service already deleted. ")
				} else {
					_, err = computeService.BackendServices.Delete(project, oldLB.Name).Do()
					if err != nil {
						log.Printf("Error deleting backend service ")
					}
				}

				ginkgo.By("Deleting GCP health check for rh-api ")
				_, err = computeService.HealthChecks.Get(project, oldLB.Name).Do()
				if err != nil {
					log.Printf("GCP health check already deleted ")
				} else {
					_, err = computeService.HealthChecks.Delete(project, oldLB.Name).Do()
					if err != nil {
						log.Printf("Error deleting health check ")
					}
				}

				ginkgo.By("Deleting GCP target pool for rh-api")
				_, err = computeService.TargetPools.Get(project, region, oldLB.Name).Do()
				if err != nil {
					log.Printf("GCP target pool already deleted")
				} else {
					_, err = computeService.TargetPools.Delete(project, region, oldLB.Name).Do()
					if err != nil {
						log.Printf("Error deleting target pool")
					}
				}
			}

			ginkgo.By("Deleting GCP address for rh-api")
			_, err = computeService.Addresses.Get(project, region, oldLBIP).Do()
			if err != nil {
				log.Printf("GCP IP address already deleted")
			} else {
				_, err = computeService.Addresses.Delete(project, region, oldLBIP).Do()
				if err != nil {
					log.Printf("Error deleting address")
				}
			}

			newLBIP := ""
			ginkgo.By("Waiting for rh-api service reconcile")
			err = wait.PollUntilContextTimeout(ctx, 15*time.Second, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
				// Getting the newly created IP from rh-api service
				ginkgo.By("Getting new rh-api IP from rh-api service")
				newLBIP, err = getLBForService(ctx, k8s, "openshift-kube-apiserver", "rh-api")
				if (err != nil) || (newLBIP == "") {
					log.Printf("New rh-api svc not created yet...")
					return false, nil
				} else if newLBIP == oldLBIP {
					log.Printf("Old rh-api svc not deleted yet...")
					return false, nil
				} else {
					log.Printf("Found new rh-api svc!")
					log.Printf("Reconciliation succeeded. New loadbalancer IP: %s ", newLBIP)
					return true, nil
				}
			})
			Expect(err).NotTo(HaveOccurred(), "rh-api service did not reconcile")

			ginkgo.By("Waiting for new rh-api forwarding rule")
			err = wait.PollUntilContextTimeout(ctx, 5*time.Second, 1*time.Minute, false, func(ctx context.Context) (bool, error) {
				ginkgo.By("Polling GCP to get new forwarding rule for rh-api")
				newLB, err := getGCPForwardingRuleForIP(computeService, newLBIP, project, region)
				if err != nil || newLB == nil {
					// Either we couldn't retrieve the LB, or it wasn't created yet
					log.Printf("New forwarding rule not found yet...")
					return false, nil
				}
				log.Printf("New lb name: %s ", newLB.Name)

				if newLB.Name != oldLB.Name {
					// A new LB was successfully recreated in GCP
					return true, nil
				}
				// rh-api lb hasn't been deleted yet
				log.Printf("Old forwarding rule not deleted yet...")
				return false, nil
			})
			Expect(err).NotTo(HaveOccurred(), "New rh-api forwarding rule not created in GCP")
		})
	}
})

// getLBForService retrieves the load balancer name or IP associated with a service of type LoadBalancer
func getLBForService(ctx context.Context, k8s *openshift.Client, namespace string, service string) (string, error) {
	svc := new(corev1.Service)
	err := k8s.Get(ctx, service, namespace, svc)
	if err != nil {
		return "", err
	}
	if svc.Spec.Type != "LoadBalancer" {
		return "", fmt.Errorf("service type is not LoadBalancer")
	}

	ingressList := svc.Status.LoadBalancer.Ingress
	if len(ingressList) == 0 {
		// the LB wasn't created yet
		return "", nil
	}

	// for GCP
	if len(ingressList[0].IP) > 0 {
		return ingressList[0].IP, nil
	}

	// for aws
	return ingressList[0].Hostname[0:32], nil
}

// deleteSecGroupReferencesToOrphans deletes any security group rules referencing the provided
// security group IDs (assumed to be those of security groups "orphaned" by LB deletion)
func deleteSecGroupReferencesToOrphans(ec2Svc *ec2.EC2, orphanSecGroupIds []*string) error {
	for _, orphanSecGroupId := range orphanSecGroupIds {
		// list all sec groups
		secGroupsAll, err := ec2Svc.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{})
		if err != nil {
			return err
		}

		// now that we know which sec groups mention the orphan, we can modify them to remove
		// the referencing rules
		for _, secGroup := range secGroupsAll.SecurityGroups {
			// define an "IpPermissions" pattern that matches all rules referencing orphan
			orphanSecGroupIpPermissions := []*ec2.IpPermission{
				{
					IpProtocol:       aws.String("-1"), // Means "all protocols"
					UserIdGroupPairs: []*ec2.UserIdGroupPair{{GroupId: aws.String(*orphanSecGroupId)}},
				},
			}

			// delete all egress rules matching pattern
			_, err = ec2Svc.RevokeSecurityGroupEgress(&ec2.RevokeSecurityGroupEgressInput{
				GroupId:       aws.String(*secGroup.GroupId),
				IpPermissions: orphanSecGroupIpPermissions,
			})
			if err == nil {
				log.Printf("Removed egress rule referring to orphan from %s", *secGroup.GroupId)
			} else if err.(awserr.Error).Code() != "InvalidPermission.NotFound" {
				// since we're iterating over all security groups, RevokeSecurityGroup*gress
				// will often throw InvalidPermission; this is expected behavior. if a different
				// error arises, report it
				log.Printf("Encountered error while removing egress rule from %s: %s", *secGroup.GroupId, err)
			}

			// delete all ingress rules matching pattern
			_, err = ec2Svc.RevokeSecurityGroupIngress(&ec2.RevokeSecurityGroupIngressInput{
				GroupId:       aws.String(*secGroup.GroupId),
				IpPermissions: orphanSecGroupIpPermissions,
			})
			if err == nil {
				log.Printf("Removed ingress rule referring to orphan from %s", *secGroup.GroupId)
			} else if err.(awserr.Error).Code() != "InvalidPermission.NotFound" {
				log.Printf("Encountered error while removing ingress rule from %s: %s", *secGroup.GroupId, err)
			}
		}
	}
	return nil
}

// get credential object to use in service initialization
func getGCPCreds(ctx context.Context, k8s *openshift.Client) (*google.Credentials, bool) {
	serviceAccountJSON := []byte(os.Getenv("GCP_CREDS_JSON"))
	credentials, err := google.CredentialsFromJSON(
		ctx, serviceAccountJSON,
		computev1.ComputeScope)
	if err != nil {
		return nil, false
	}
	return credentials, true
}

// Get forwarding rule for rh-api load balancer in GCP
func getGCPForwardingRuleForIP(computeService *computev1.Service, oldLBIP string, project string, region string) (*computev1.ForwardingRule, error) {
	listCall := computeService.ForwardingRules.List(project, region)
	response, err := listCall.Do()
	var oldLB *computev1.ForwardingRule
	if err != nil {
		return nil, err
	}

	for _, lb := range response.Items {
		// This list of forwardingrules (LBs) includes any service LBs
		// for application routers so check the IP to identify
		// the rh-api LB.
		if lb.IPAddress == oldLBIP {
			oldLB = lb
		}
	}

	return oldLB, nil
}
