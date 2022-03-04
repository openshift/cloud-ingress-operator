package utils

import (
	"context"

	configv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetClusterVersionObject returns the canonical ClusterVersion object
// To check current version: `output.Status.History[0].Version` or `output.Status.Desired.Version` depending on the use-case.
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
