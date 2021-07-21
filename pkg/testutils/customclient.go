package testutils

import (
	"context"
	"fmt"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type customClient struct {
	client.Client
	errorOn     string
	errorType   string
	errorTarget string // when specified, will only error if the action errorOn is done this target.
}

type ClientObj struct {
	Obj          client.Object
	GroupVersion schema.GroupVersion
}

type RuntimeObj struct {
	Obj          runtime.Object
	GroupVersion schema.GroupVersion
}

func SetUpTestClient(clientobj []ClientObj, runtimeobj []RuntimeObj, errorOn, errorType, errorTarget string) (*customClient, *runtime.Scheme) {
	s := scheme.Scheme
	ro := []runtime.Object{}
	co := []client.Object{}

	for _, v := range clientobj {
		s.AddKnownTypes(v.GroupVersion, v.Obj)
		co = append(co, v.Obj)
	}
	for _, v := range runtimeobj {
		s.AddKnownTypes(v.GroupVersion, v.Obj)
		ro = append(ro, v.Obj)
	}

	testClient := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(ro...).WithObjects(co...).Build()
	return &customClient{testClient, errorOn, errorType, errorTarget}, s
}

func getK8sError(errorType string, objType string) error {
	errorMap := map[string]error{
		"IsNotFound": k8serr.NewNotFound(schema.GroupResource{Group: "somegroup",
			Resource: "varies"}, objType),
	}
	if err, found := errorMap[errorType]; found {
		return err
	} else {
		// by default we return internal error, when the error type specified doesn't match something we preconfigured
		return k8serr.NewInternalError(fmt.Errorf("%v was raised", errorType))

	}
}
func (c *customClient) Equal() client.Client {
	return c.Client
}
func (c *customClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if c.errorOn == "Update" {
		return getK8sError(c.errorType, fmt.Sprintf("%T", obj))
	}
	return c.Client.Update(ctx, obj, opts...)
}
func (c *customClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object) error {
	if c.errorOn == "Get" {
		t := fmt.Sprintf("%T", obj)
		if c.errorTarget == "" || c.errorTarget == t {
			return getK8sError(c.errorType, fmt.Sprintf("%T", obj))
		}
	}

	return c.Client.Get(ctx, key, obj)
}
