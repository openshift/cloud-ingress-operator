package utils

import (
	"fmt"
	"testing"

	"github.com/openshift/cloud-ingress-operator/pkg/testutils"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// None of these should ever occur, but if they did, it'd be nice to know they return an error
func TestNoInfraObj(t *testing.T) {
	masterNames := make([]string, 3)
	for i := 0; i < 3; i++ {
		masterNames[i] = fmt.Sprintf("master-%d", i)
	}
	machineList, _ := testutils.CreateMachineObjectList(masterNames, "ids", "master", testutils.DefaultRegionName, testutils.DefaultAzName)
	objs := []runtime.Object{machineList}
	mocks := testutils.NewTestMock(t, objs)

	_, err := GetClusterBaseDomain(mocks.FakeKubeClient)
	if err == nil {
		t.Fatalf("Expected to get an error from not having an Infrastructure object")
	}
	_, err = GetClusterName(mocks.FakeKubeClient)
	if err == nil {
		t.Fatalf("Expected to get an error from not having an Infrastructure object")
	}
	_, err = GetPlatformType(mocks.FakeKubeClient)
	if err == nil {
		t.Fatalf("Expected to get an error from not having an Infrastructure object")
	}
}

func TestGetCliSecret(t *testing.T) {
	fakeSecret := &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "fake-name",
			Namespace: "sut-ns",
		},
		Data: make(map[string][]byte),
	}
	fakeSecret.Data["fake"] = []byte("dummy")
	objs := []runtime.Object{fakeSecret}
	mocks := testutils.NewTestMock(t, objs)

	s, e := GetCliSecret(mocks.FakeKubeClient, "fake-name", "sut-ns")
	if e != nil {
		t.Errorf("secret should have been retrieved: %w", e)
	}
	value := string(s.Data["fake"])
	if value != "dummy" {
		t.Errorf("secret doesn't have the data")
	}
}
