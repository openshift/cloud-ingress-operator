package aws

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
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
	"github.com/openshift/cloud-ingress-operator/config"
	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	k8s "sigs.k8s.io/controller-runtime/pkg/client"

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
func (ac *Client) EnsureAdminAPIDNS(ctx context.Context, kclient k8s.Client, instance *cloudingressv1alpha1.APIScheme, svc *corev1.Service) error {
	return ac.ensureAdminAPIDNS(ctx, kclient, instance, svc)
}

// DeleteAdminAPIDNS implements cloudclient.CloudClient
func (ac *Client) DeleteAdminAPIDNS(ctx context.Context, kclient k8s.Client, instance *cloudingressv1alpha1.APIScheme, svc *corev1.Service) error {
	return ac.deleteAdminAPIDNS(ctx, kclient, instance, svc)
}

// EnsureSSHDNS implements cloudclient.CloudClient
func (ac *Client) EnsureSSHDNS(ctx context.Context, kclient k8s.Client, instance *cloudingressv1alpha1.SSHD, svc *corev1.Service) error {
	return ac.ensureSSHDNS(ctx, kclient, instance, svc)
}

// DeleteSSHDNS implements cloudclient.CloudClient
func (ac *Client) DeleteSSHDNS(ctx context.Context, kclient k8s.Client, instance *cloudingressv1alpha1.SSHD, svc *corev1.Service) error {
	return ac.deleteSSHDNS(ctx, kclient, instance, svc)
}

// SetDefaultAPIPrivate implements cloudclient.CloudClient
func (ac *Client) SetDefaultAPIPrivate(ctx context.Context, kclient k8s.Client, instance *cloudingressv1alpha1.PublishingStrategy) error {
	return ac.setDefaultAPIPrivate(ctx, kclient, instance)
}

// SetDefaultAPIPublic implements cloudclient.CloudClient
func (ac *Client) SetDefaultAPIPublic(ctx context.Context, kclient k8s.Client, instance *cloudingressv1alpha1.PublishingStrategy) error {
	return ac.setDefaultAPIPublic(ctx, kclient, instance)
}

// Healthcheck performs basic calls to make sure client is healthy
func (ac *Client) Healthcheck(ctx context.Context, kclient k8s.Client) error {
	input := &elb.DescribeLoadBalancersInput{}
	_, err := ac.elbClient.DescribeLoadBalancers(input)

	return err
}

func newClient(accessID, accessSecret, token, region string) (*Client, error) {
	awsConfig := &aws.Config{Region: aws.String(region)}
	if token == "" {
		os.Setenv("AWS_ACCESS_KEY_ID", accessID)
		os.Setenv("AWS_SECRET_ACCESS_KEY", accessSecret)
	} else {
		awsConfig.Credentials = credentials.NewStaticCredentials(accessID, accessSecret, token)
	}
	s, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, err
	}
	return &Client{
		ec2Client:     ec2.New(s),
		elbClient:     elb.New(s),
		elbv2Client:   elbv2.New(s),
		route53Client: route53.New(s),
	}, nil
}

// NewClient creates a new CloudClient for use with AWS.
func NewClient(kclient k8s.Client) (*Client, error) {
	region, err := getClusterRegion(kclient)
	if err != nil {
		return nil, fmt.Errorf("couldn't get cluster region %w", err)
	}
	secret := &corev1.Secret{}
	err = kclient.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      config.AWSSecretName,
			Namespace: config.OperatorNamespace,
		},
		secret)
	if err != nil {
		return nil, fmt.Errorf("couldn't get Secret with credentials %w", err)
	}
	accessKeyID, ok := secret.Data["aws_access_key_id"]
	if !ok {
		return nil, fmt.Errorf("access credentials missing key")
	}
	secretAccessKey, ok := secret.Data["aws_secret_access_key"]
	if !ok {
		return nil, fmt.Errorf("access credentials missing secret key")
	}

	c, err := newClient(
		string(accessKeyID),
		string(secretAccessKey),
		"",
		region)

	if err != nil {
		return nil, fmt.Errorf("couldn't create AWS client %w", err)
	}

	return c, nil
}
