package utils

import (
	"context"
	"fmt"
	"os"

	compare "github.com/hashicorp/go-version"
	configv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetClusterVersionObject returns the canonical ClusterVersion object
// To check current version: `output.Status.History[0].Version`
//
// `history contains a list of the most recent versions applied to the cluster.
// This value may be empty during cluster startup, and then will be updated when a new update is being applied.
// The newest update is first in the list and it is ordered by recency`
//
// Note:
// This can be queried inside the controllers, caching doesn't apply if the scope is global rather than namespaced.
func GetClusterVersionObject(kclient client.Client) (*configv1.ClusterVersion, error) {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "config.openshift.io/v1",
		Kind:    "ClusterVersion",
	})
	ns := types.NamespacedName{
		Namespace: "",
		Name:      "version",
	}
	err := kclient.Get(context.TODO(), ns, u)
	if err != nil {
		return nil, err
	}

	uContent := u.UnstructuredContent()
	var cv *configv1.ClusterVersion
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(uContent, &cv)
	if err != nil {
		return nil, err
	}

	return cv, nil
}

// SetClusterVersion sets the cluster version globally(to ENV as CLUSTER_VERSION)
func SetClusterVersion(kclient client.Client) error {
	versionObject, err := GetClusterVersionObject(kclient)
	if err != nil {
		return err
	}

	// handle when there's no object defined || no version found on history
	if len(versionObject.Status.History) == 0 || versionObject == nil {
		return fmt.Errorf("version couldn't be grabbed from clusterversion: %+v", versionObject) // (%+v) adds field names
	}

	return os.Setenv("CLUSTER_VERSION", versionObject.Status.History[0].Version)
}

// IsVersionHigherThan checks whether the given version is higher than the cluster version
// input is required to be a version such as: 4.10 or 4.10.1
// Returns false(no action) if there's an exception.
func IsVersionHigherThan(input string) bool {
	version, ok := os.LookupEnv("CLUSTER_VERSION")
	if !ok {
		return false
	}
	// Handle the clusternames that have more than 4 chars(such as 4.10.0-rc.4)
	shortVersion := version[0:4]

	EnvVersion, err := compare.NewVersion(shortVersion)
	if err != nil {
		return false
	}

	inputVersion, err := compare.NewVersion(input)
	if err != nil {
		return false
	}

	if EnvVersion.LessThan(inputVersion) {
		return false
	}

	return true // input greater than env so action
}
