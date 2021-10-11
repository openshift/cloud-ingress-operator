package utils

import (
	"testing"

	operv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/cloud-ingress-operator/pkg/testutils"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestSAhealthcheck(t *testing.T) {
	ingressCO := &operv1.IngressController{
		ObjectMeta: v1.ObjectMeta{
			Name:      "default",
			Namespace: "openshift-ingress-operator",
		},
	}
	objs := []runtime.Object{ingressCO}
	mocks := testutils.NewTestMock(t, objs)
	if err := SAhealthcheck(mocks.FakeKubeClient); err != nil {
		t.Error("checking ingresscontroller failed:", err)
	}
}
