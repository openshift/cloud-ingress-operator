package aws

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/elb/elbiface"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"

	configv1 "github.com/openshift/api/config/v1"
	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	"github.com/openshift/cloud-ingress-operator/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// ClientIdentifier is what kind of cloud this implement supports
const ClientIdentifier configv1.PlatformType = configv1.AWSPlatformType

var (
	log = logf.Log.WithName("aws_cloudclient")
)

// Client represents an AWS Client
type Client struct {
	ec2Client     ec2iface.EC2API
	route53Client route53iface.Route53API
	elbClient     elbiface.ELBAPI
	elbv2Client   elbv2iface.ELBV2API
}

// EnsureAdminAPIDNS implements cloudclient.CloudClient
func (c *Client) EnsureAdminAPIDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.APIScheme, svc *corev1.Service) error {
	return c.ensureAdminAPIDNS(ctx, kclient, instance, svc)
}

// DeleteAdminAPIDNS implements cloudclient.CloudClient
func (c *Client) DeleteAdminAPIDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.APIScheme, svc *corev1.Service) error {
	return c.deleteAdminAPIDNS(ctx, kclient, instance, svc)
}

// EnsureSSHDNS implements cloudclient.CloudClient
func (c *Client) EnsureSSHDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.SSHD, svc *corev1.Service) error {
	return c.ensureSSHDNS(ctx, kclient, instance, svc)
}

// DeleteSSHDNS implements cloudclient.CloudClient
func (c *Client) DeleteSSHDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.SSHD, svc *corev1.Service) error {
	return c.deleteSSHDNS(ctx, kclient, instance, svc)
}

// SetDefaultAPIPrivate implements cloudclient.CloudClient
func (c *Client) SetDefaultAPIPrivate(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.PublishingStrategy) error {
	return c.setDefaultAPIPrivate(ctx, kclient, instance)
}

// SetDefaultAPIPublic implements cloudclient.CloudClient
func (c *Client) SetDefaultAPIPublic(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.PublishingStrategy) error {
	return c.setDefaultAPIPublic(ctx, kclient, instance)
}

func newClient(region string, kclient client.Client) (*Client, error) {
	//awsConfig := &aws.Config{Region: aws.String(region)}
	sessionOptions := session.Options{
		Config: aws.Config{
			Region: aws.String(region),
		},
	}

	creds := &corev1.Secret{}
	err := kclient.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      config.AWSSecretName,
			Namespace: config.OperatorNamespace,
		},
		creds)
	if err != nil {
		panic(fmt.Sprintf("Couldn't get secret with credentials %s", err.Error()))
	}

	// get sharedCredsFile from secret
	sharedCredsFile, err := SharedCredentialsFileFromSecret(creds)
	if err != nil {
		return nil, err
	}

	sessionOptions.SharedConfigState = session.SharedConfigEnable // Force enable Shared Config support
	sessionOptions.SharedConfigFiles = []string{sharedCredsFile}  // Ordered list of files the session will load configuration from.

	s, err := session.NewSessionWithOptions(sessionOptions)
	if err != nil {
		return nil, err
	}

	// Remove temporary shared credentials token at end of func after creating session
	defer os.Remove(sharedCredsFile)

	return &Client{
		ec2Client:     ec2.New(s),
		elbClient:     elb.New(s),
		elbv2Client:   elbv2.New(s),
		route53Client: route53.New(s),
	}, nil
}

// NewClient creates a new CloudClient for use with AWS.
func NewClient(kclient client.Client) *Client {
	region, err := getClusterRegion(kclient)
	if err != nil {
		panic(fmt.Sprintf("Couldn't get cluster region %s", err.Error()))
	}

	c, err := newClient(
		region,
		kclient)

	if err != nil {
		panic(fmt.Sprintf("Couldn't create AWS client %s", err.Error()))
	}

	return c
}
