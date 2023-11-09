package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/elb/elbiface"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cloud-ingress-operator/config"
	"github.com/openshift/cloud-ingress-operator/pkg/testutils"
	"github.com/openshift/cloud-ingress-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func TestCpmsBranching(t *testing.T) {
	fakeAwsMachine := machinev1beta1.AWSMachineProviderConfig{
		LoadBalancers: []machinev1beta1.LoadBalancerReference{
			{
				Name: "internal-lb",
				Type: "NLB",
			},
			{
				Name: "removal-lb",
				Type: "NLB",
			}},
	}
	bytes, _ := utils.ConvertToRawBytes(fakeAwsMachine)
	fakeCpms := machinev1.ControlPlaneMachineSet{
		TypeMeta:   v1.TypeMeta{},
		ObjectMeta: v1.ObjectMeta{Name: "cluster", Namespace: "openshift-machine-api"},
		Spec: machinev1.ControlPlaneMachineSetSpec{
			State:    machinev1.ControlPlaneMachineSetStateActive,
			Replicas: aws.Int32(3),
			Strategy: machinev1.ControlPlaneMachineSetStrategy{Type: ""},
			Selector: v1.LabelSelector{},
			Template: machinev1.ControlPlaneMachineSetTemplate{
				MachineType: machinev1.OpenShiftMachineV1Beta1MachineType,
				OpenShiftMachineV1Beta1Machine: &machinev1.OpenShiftMachineV1Beta1MachineTemplate{
					FailureDomains: machinev1.FailureDomains{
						Platform: "aws",
						AWS:      &[]machinev1.AWSFailureDomain{},
					},
					ObjectMeta: machinev1.ControlPlaneMachineSetTemplateObjectMeta{},
					Spec: machinev1beta1.MachineSpec{
						ObjectMeta: machinev1beta1.ObjectMeta{
							Name:      "awsmachine",
							Namespace: "openshift-machine-api",
						},
						ProviderSpec: machinev1beta1.ProviderSpec{
							Value: &runtime.RawExtension{
								Raw:    bytes,
								Object: nil,
							},
						},
						ProviderID: aws.String("aws"),
					},
				},
			},
		},
	}
	objs := []runtime.Object{&fakeCpms}
	mocks := testutils.NewTestMock(t, objs)
	f := getLoadBalancerRemovalFunc(context.TODO(), mocks.FakeKubeClient, nil, &fakeCpms)
	err := f("removal-lb")
	if err != nil {
		t.Errorf("Removing load balancer from cluster failed: %v", err)
	}
	updatedCpms := &machinev1.ControlPlaneMachineSet{}
	err = mocks.FakeKubeClient.Get(context.TODO(), client.ObjectKey{
		Namespace: "openshift-machine-api",
		Name:      "cluster",
	}, updatedCpms)
	if err != nil {
		t.Errorf("Could not get cpms again.")
	}
	spec, err := utils.ConvertFromRawExtension[machinev1beta1.AWSMachineProviderConfig](updatedCpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec.ProviderSpec.Value)
	if err != nil {
		t.Errorf("Could not convert cpms rawextension.")
	}
	if len(spec.LoadBalancers) != 1 {
		t.Errorf("Removing load balancer from cluster did not leave only 1 lb behind: %v", spec.LoadBalancers)
	}
}
