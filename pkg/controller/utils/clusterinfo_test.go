package utils

import (
	"fmt"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/cloud-ingress-operator/pkg/testutils"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestClusterBaseDomain(t *testing.T) {
	infraObj := testutils.CreateInfraObject("basename", testutils.DefaultAPIEndpoint, testutils.DefaultAPIEndpoint, testutils.DefaultRegionName)
	objs := []runtime.Object{infraObj}
	mocks := testutils.NewTestMock(t, objs)

	base, err := GetClusterBaseDomain(mocks.FakeKubeClient)
	if err != nil {
		t.Fatalf("Could not get cluster base domain name: %v", err)
	}
	if base != "unit.test" {
		t.Fatalf("Base domain mismatch. Expected %s, got %s", "unit.test", base)
	}
}

// BZ https://bugzilla.redhat.com/show_bug.cgi?id=1814332
func TestOldClusterNoInfrastructureBackfill(t *testing.T) {
	clustername := "oldtest"
	// legacy infra obj has this extra bits, but the configmap does not
	extrabits := "cmld7"
	oldInfraObj := testutils.CreatOldInfraObject("oldtest-cmld7", testutils.DefaultAPIEndpoint, testutils.DefaultAPIEndpoint, testutils.DefaultRegionName)
	oldCM := testutils.CreateLegacyClusterConfig(fmt.Sprintf("%s.%s", clustername, testutils.DefaultClusterDomain),
		fmt.Sprintf("%s-%s", clustername, extrabits), testutils.DefaultRegionName, 0, 0)
	objs := []runtime.Object{oldInfraObj, oldCM}
	mocks := testutils.NewTestMock(t, objs)
	region, err := GetClusterRegion(mocks.FakeKubeClient)
	if err != nil {
		t.Fatalf("Error: Couldn't get region. Expected to get %s: %v", testutils.DefaultRegionName, err)
	}
	if region != testutils.DefaultRegionName {
		t.Fatalf("Expected region to be %s, but got %s", testutils.DefaultRegionName, region)
	}
}

func TestGetClusterPlatform(t *testing.T) {
	infraObj := testutils.CreateInfraObject("basename", testutils.DefaultAPIEndpoint, testutils.DefaultAPIEndpoint, testutils.DefaultRegionName)
	objs := []runtime.Object{infraObj}
	mocks := testutils.NewTestMock(t, objs)

	platform, err := GetClusterPlatform(mocks.FakeKubeClient)
	if err != nil {
		t.Fatalf("Couldn't get cluster platform: %v", err)
	}
	if platform != string(configv1.AWSPlatformType) {
		t.Fatalf("Expected platform to be %v, got %v instead.", string(configv1.AWSPlatformType), platform)
	}
}

func TestGetClusterName(t *testing.T) {
	clustername := "cluster-test-name"
	infraObj := testutils.CreateInfraObject(clustername, testutils.DefaultAPIEndpoint, testutils.DefaultAPIEndpoint, testutils.DefaultRegionName)
	objs := []runtime.Object{infraObj}
	mocks := testutils.NewTestMock(t, objs)

	name, err := GetClusterName(mocks.FakeKubeClient)
	if err != nil {
		t.Fatalf("Couldn't get cluster name %v", err)
	}
	if name != clustername {
		t.Fatalf("Expected cluster name to be %s, got %s instead", clustername, name)
	}
}

func TestGetMasterSubnetNames(t *testing.T) {
	clustername := "subnet-test"
	masterNames := make([]string, 3)
	for i := 0; i < 3; i++ {
		masterNames[i] = fmt.Sprintf("master-%d", i)
	}

	machineList, _ := testutils.CreateMachineObjectList(masterNames, "ids", "master", testutils.DefaultRegionName, testutils.DefaultAzName)
	infraObj := testutils.CreateInfraObject(clustername, testutils.DefaultAPIEndpoint, testutils.DefaultAPIEndpoint, testutils.DefaultRegionName)
	objs := []runtime.Object{infraObj, machineList}
	mocks := testutils.NewTestMock(t, objs)
	subnetmap, err := GetMasterNodeSubnets(mocks.FakeKubeClient)

	if err != nil {
		t.Fatalf("Couldn't get subnet map: %v", err)
	}
	if len(subnetmap) != 2 {
		t.Fatalf("Expected Subnet map to have exactly two keys, but got %d keys back", len(subnetmap))
	}
	if _, ok := subnetmap["public"]; !ok {
		t.Fatalf("Expected Subnet map to have a 'public' key, and it did not")
	}
	if _, ok := subnetmap["private"]; !ok {
		t.Fatalf("Expected Subnet map to have a 'private' key, and it did not")
	}
	expected := fmt.Sprintf("%s-public-%s", clustername, testutils.DefaultAzName)
	if subnetmap["public"] != expected {
		t.Fatalf("Expected public subnet to be %s, but got %s instead", expected, subnetmap["public"])
	}
	expected = fmt.Sprintf("%s-private-%s", clustername, testutils.DefaultAzName)
	if subnetmap["private"] != expected {
		t.Fatalf("Expected private subnet to be %s, but got %s instead", expected, subnetmap["private"])
	}
}

func TestGetMasterSubnetNamesNoMasters(t *testing.T) {
	clustername := "subnet-test"
	masterNames := make([]string, 0)
	machineList, _ := testutils.CreateMachineObjectList(masterNames, "ids", "master", testutils.DefaultRegionName, testutils.DefaultAzName)
	infraObj := testutils.CreateInfraObject(clustername, testutils.DefaultAPIEndpoint, testutils.DefaultAPIEndpoint, testutils.DefaultRegionName)
	objs := []runtime.Object{infraObj, machineList}
	mocks := testutils.NewTestMock(t, objs)

	_, err := GetMasterNodeSubnets(mocks.FakeKubeClient)
	if err == nil {
		t.Fatalf("Expected to see an exception when trying to get subnets for 0 master nodes ")
	}
}

func TestGetClusterRegion(t *testing.T) {
	infraObj := testutils.CreateInfraObject("region-test", testutils.DefaultAPIEndpoint, testutils.DefaultAPIEndpoint, testutils.DefaultRegionName)
	objs := []runtime.Object{infraObj}
	mocks := testutils.NewTestMock(t, objs)

	region, err := GetClusterRegion(mocks.FakeKubeClient)
	if err != nil {
		t.Fatalf("Could not get cluster region: %v", err)
	}
	if region != testutils.DefaultRegionName {
		t.Fatalf("Region mismatch. Expected %s, got %s", region, testutils.DefaultRegionName)
	}
}

func TestMasterInstanceIDs(t *testing.T) {
	masterNames := make([]string, 3)
	for i := 0; i < 3; i++ {
		masterNames[i] = fmt.Sprintf("master-%d", i)
	}
	machineList, _ := testutils.CreateMachineObjectList(masterNames, "ids", "master", testutils.DefaultRegionName, testutils.DefaultAzName)
	objs := []runtime.Object{machineList}
	mocks := testutils.NewTestMock(t, objs)

	ids, err := GetClusterMasterInstancesIDs(mocks.FakeKubeClient)
	if err != nil {
		t.Fatalf("Couldn't get master instance IDs %v", err)
	}
	if len(ids) != len(masterNames) {
		t.Fatalf("Expected %d instance IDs, but got %d back", len(masterNames), len(ids))
	}
	// TODO(lseelye): It'd be nice if this matched up with the actual provider
	// scheme instead of the contrived i-name
	for _, id := range ids {
		if id[0:9] != "i-master-" {
			t.Fatalf("Expected master instance ID to begin with i-master-. Got %s", string(id[0:9]))
		}
	}
}

func TestAWSOwnerTag(t *testing.T) {
	clustername := "awstags-test"
	infraObj := testutils.CreateInfraObject(clustername, testutils.DefaultAPIEndpoint, testutils.DefaultAPIEndpoint, testutils.DefaultRegionName)
	objs := []runtime.Object{infraObj}
	mocks := testutils.NewTestMock(t, objs)
	tags, err := AWSOwnerTag(mocks.FakeKubeClient)
	if err != nil {
		t.Fatalf("Couldn't get AWS owner tags %v", err)
	}
	// might add more tags(?), so check for non-zero
	if len(tags) == 0 {
		t.Fatalf("Zero tags returned, expected at least one")
	}
	expected := "kubernetes.io/cluster/awstags-test"
	if v, ok := tags[expected]; ok {
		if v != "owned" {
			t.Fatalf("Expected owner tag %s to have value %s. Got %s", expected, "owned", v)
		}
	} else {
		t.Fatalf("Expected to have a tag with key %s, but there wasn't one", expected)
	}
}

// None of these should ever occur, but if they did, it'd be nice to know they return an error
func TestNoInfraObj(t *testing.T) {
	masterNames := make([]string, 3)
	for i := 0; i < 3; i++ {
		masterNames[i] = fmt.Sprintf("master-%d", i)
	}
	machineList, _ := testutils.CreateMachineObjectList(masterNames, "ids", "master", testutils.DefaultRegionName, testutils.DefaultAzName)
	objs := []runtime.Object{machineList}
	mocks := testutils.NewTestMock(t, objs)

	_, err := AWSOwnerTag(mocks.FakeKubeClient)
	if err == nil {
		t.Fatalf("Expected to get an error from not having an Infrastructure object")
	}
	_, err = GetClusterRegion(mocks.FakeKubeClient)
	if err == nil {
		t.Fatalf("Expected to get an error from not having an Infrastructure object")
	}
	_, err = GetMasterNodeSubnets(mocks.FakeKubeClient)
	if err == nil {
		t.Fatalf("Expected to get an error from not having an Infrastructure object")
	}
	_, err = GetClusterName(mocks.FakeKubeClient)
	if err == nil {
		t.Fatalf("Expected to get an error from not having an Infrastructure object")
	}
	_, err = GetClusterPlatform(mocks.FakeKubeClient)
	if err == nil {
		t.Fatalf("Expected to get an error from not having an Infrastructure object")
	}
	_, err = GetClusterBaseDomain(mocks.FakeKubeClient)
	if err == nil {
		t.Fatalf("Expected to get an error from not having an Infrastructure object")
	}

}
