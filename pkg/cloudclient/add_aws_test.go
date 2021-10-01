package cloudclient

import (
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/cloud-ingress-operator/config"
	"github.com/openshift/cloud-ingress-operator/pkg/testutils"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestProducePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("testing panic: should have been failed")
		}
	}()
	objs := []runtime.Object{}
	mocks := testutils.NewTestMock(t, objs)
	_ = produce(mocks.FakeKubeClient)
}

func TestProduceSuccess(t *testing.T) {
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
	cli := produce(mocks.FakeKubeClient)
	if cli == nil {
		t.Error("cli couldn't initialize with given params")
	}
}
