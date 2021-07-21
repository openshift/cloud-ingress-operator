package cloudclient

import (
	"testing"

	"github.com/openshift/cloud-ingress-operator/pkg/testutils"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestProduce(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("testing panic: should have been failed")
		}
	}()
	objs := []runtime.Object{}
	mocks := testutils.NewTestMock(t, objs)
	_ = produce(mocks.FakeKubeClient)
}
