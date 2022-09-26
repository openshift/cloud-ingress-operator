package apischeme

import (
	"fmt"
	"testing"

	"github.com/openshift/cloud-ingress-operator/pkg/testutils"

	baseutils "github.com/openshift/cloud-ingress-operator/pkg/utils"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestClusterBaseDomain(t *testing.T) {
	aObj := testutils.CreateAPISchemeObject("rh-api", true, []string{"0.0.0.0/0"})
	masterNames := make([]string, 3)
	for i := 0; i < 3; i++ {
		masterNames[i] = fmt.Sprintf("master-%d", i)
	}
	machineList, _ := testutils.CreateMachineObjectList(masterNames, "basename", "master", testutils.DefaultRegionName, testutils.DefaultAzName)
	infraObj := testutils.CreateInfraObject("basename", testutils.DefaultAPIEndpoint, testutils.DefaultAPIEndpoint, testutils.DefaultRegionName)
	objs := []runtime.Object{aObj, infraObj, machineList}
	mocks := testutils.NewTestMock(t, objs)

	base, err := baseutils.GetClusterBaseDomain(mocks.FakeKubeClient)
	if err != nil {
		t.Fatalf("Could not get cluster base domain name: %v", err)
	}
	if base != "unit.test" {
		t.Fatalf("Base domain mismatch. Expected %s, got %s", "unit.test", base)
	}
}
