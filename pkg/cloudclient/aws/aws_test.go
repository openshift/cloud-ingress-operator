package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/elb/elbiface"
	"github.com/openshift/cloud-ingress-operator/pkg/config"
	"github.com/openshift/cloud-ingress-operator/pkg/testutils"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestNewClient(t *testing.T) {
	infra := testutils.CreateInfraObject("test-cluster", testutils.DefaultAPIEndpoint, testutils.DefaultAPIEndpoint, testutils.DefaultRegionName)

	fakeSecret := &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      config.AWSSecretName,
			Namespace: config.OperatorNamespace,
		},
		Data: make(map[string][]byte),
	}
	fakeSecret.Data["aws_access_key_id"] = []byte("dummyID")
	fakeSecret.Data["aws_secret_access_key"] = []byte("dummyPassKey")

	objs := []runtime.Object{infra, fakeSecret}
	mocks := testutils.NewTestMock(t, objs)
	cli, err := NewClient(mocks.FakeKubeClient)

	if err != nil {
		t.Error("err occured while creating cli:", err)
	}

	if cli == nil {
		t.Errorf("cli should have been initialized")
	}
}

type mockELBClient struct {
	elbiface.ELBAPI
}

func (m *mockELBClient) DescribeLoadBalancers(params *elb.DescribeLoadBalancersInput) (*elb.DescribeLoadBalancersOutput, error) {
	// mock response/functionality
	out := &elb.DescribeLoadBalancersOutput{
		LoadBalancerDescriptions: []*elb.LoadBalancerDescription{
			{
				LoadBalancerName: aws.String("lb-3"),
			},
		},
		NextMarker: aws.String(""),
	}

	return out, nil
}

func (m *mockELBClient) DescribeTags(*elb.DescribeTagsInput) (*elb.DescribeTagsOutput, error) {
	return &elb.DescribeTagsOutput{
		TagDescriptions: []*elb.TagDescription{
			{
				Tags: []*elb.Tag{
					{
						Key:   aws.String("kubernetes.io/service-name"),
						Value: aws.String("openshift-kube-apiserver/rh-api"),
					},
					{
						Key:   aws.String("kubernetes.io/cluster/dummy-cluster"),
						Value: aws.String("owned"),
					},
					{
						Key:   aws.String("ccs-tag"),
						Value: aws.String("project"),
					},
				},
			},
		},
	}, nil
}

func TestHealthcheck(t *testing.T) {
	infra := testutils.CreateInfraObject("dummy-cluster", testutils.DefaultAPIEndpoint, testutils.DefaultAPIEndpoint, testutils.DefaultRegionName)

	objs := []runtime.Object{infra}
	mocks := testutils.NewTestMock(t, objs)
	cli := Client{elbClient: &mockELBClient{}}
	err := cli.Healthcheck(context.TODO(), mocks.FakeKubeClient)
	if err != nil {
		t.Error("err occured while performing healthcheck:", err)
	}

}
