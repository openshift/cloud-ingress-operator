package utils

import (
	"testing"

	"github.com/openshift/cloud-ingress-operator/pkg/ingresscontroller"
	"github.com/openshift/cloud-ingress-operator/pkg/testutils"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestSAhealthcheck(t *testing.T) {
	ingressCO := &ingresscontroller.IngressController{
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
