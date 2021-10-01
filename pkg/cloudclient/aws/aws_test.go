package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/elb/elbiface"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/cloud-ingress-operator/config"
	"github.com/openshift/cloud-ingress-operator/pkg/testutils"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestNewClient(t *testing.T) {
	infra := &configv1.Infrastructure{
		ObjectMeta: v1.ObjectMeta{
			Name:      "cluster",
			Namespace: "",
		},
		Status: configv1.InfrastructureStatus{
			PlatformStatus: &configv1.PlatformStatus{
				AWS: &configv1.AWSPlatformStatus{
					Region: "eu-west-1",
				},
			},
		}}

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
		t.Error("err occured while creating cli: %w", err)
	}

	if cli == nil {
		t.Errorf("cli should have been initialized")
	}
}

type mockELBClient struct {
	elbiface.ELBAPI
}

func (m *mockELBClient) DescribeLoadBalancers(i *elb.DescribeLoadBalancersInput) (*elb.DescribeLoadBalancersOutput, error) {
	// mock response/functionality
	testLB := "test"
	return &elb.DescribeLoadBalancersOutput{
		LoadBalancerDescriptions: []*elb.LoadBalancerDescription{
			{
				LoadBalancerName: &testLB,
			},
		},
	}, nil
}

func (m *mockELBClient) DescribeTags(*elb.DescribeTagsInput) (*elb.DescribeTagsOutput, error) {
	k := "sut"
	v := "openshift-kube-apiserver/rh-api"
	return &elb.DescribeTagsOutput{
		TagDescriptions: []*elb.TagDescription{
			{
				Tags: []*elb.Tag{
					{
						Key:   &k,
						Value: &v,
					},
				},
			},
		},
	}, nil
}

func TestHealthcheck(t *testing.T) {
	objs := []runtime.Object{}
	mocks := testutils.NewTestMock(t, objs)
	cli := Client{elbClient: &mockELBClient{}}
	err := cli.Healthcheck(context.TODO(), mocks.FakeKubeClient)
	if err != nil {
		t.Errorf("err occured while performing healthcheck: %s", err)
	}

}
