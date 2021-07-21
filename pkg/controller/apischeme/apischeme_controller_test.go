package apischeme

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	configv1 "github.com/openshift/api/config/v1"
	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	"github.com/openshift/cloud-ingress-operator/pkg/cloudclient"
	"github.com/openshift/cloud-ingress-operator/pkg/cloudclient/aws"
	mockcc "github.com/openshift/cloud-ingress-operator/pkg/cloudclient/mock_cloudclient"
	cioerrors "github.com/openshift/cloud-ingress-operator/pkg/errors"
	"github.com/openshift/cloud-ingress-operator/pkg/testutils"
	baseutils "github.com/openshift/cloud-ingress-operator/pkg/utils"
	v1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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

func TestReconcile(t *testing.T) {
	var (
		name      = "rh-api"
		namespace = "openshift-cloud-ingress-operator"
	)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// initialize the cloudclient
	cloud := mockcc.NewMockCloudClient(ctrl)
	cloudclient.Register(aws.ClientIdentifier, func(kclient client.Client) cloudclient.CloudClient { return cloud })

	//unmanaged apischeme
	defaultApiScheme := testutils.ClientObj{
		Obj: &cloudingressv1alpha1.APIScheme{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		},
		GroupVersion: cloudingressv1alpha1.SchemeGroupVersion,
	}
	//managed apisheme
	managedApiScheme := testutils.ClientObj{
		Obj: &cloudingressv1alpha1.APIScheme{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: cloudingressv1alpha1.APISchemeSpec{
				ManagementAPIServerIngress: cloudingressv1alpha1.ManagementAPIServerIngress{
					Enabled: true,
				},
			},
		},
		GroupVersion: cloudingressv1alpha1.SchemeGroupVersion,
	}

	metatime := metav1.Now()
	apischemeWithDeleteTimestampNoFinalizer := testutils.ClientObj{
		Obj: &cloudingressv1alpha1.APIScheme{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         namespace,
				DeletionTimestamp: &metatime,
			},
			Spec: cloudingressv1alpha1.APISchemeSpec{
				ManagementAPIServerIngress: cloudingressv1alpha1.ManagementAPIServerIngress{
					Enabled: true,
				},
			},
		},
		GroupVersion: cloudingressv1alpha1.SchemeGroupVersion,
	}
	apischemeWithDeleteTimestampWithFinalizer := testutils.ClientObj{
		Obj: &cloudingressv1alpha1.APIScheme{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         namespace,
				Finalizers:        []string{"dns.cloudingress.managed.openshift.io"},
				DeletionTimestamp: &metatime,
			},
			Spec: cloudingressv1alpha1.APISchemeSpec{
				ManagementAPIServerIngress: cloudingressv1alpha1.ManagementAPIServerIngress{
					Enabled: true,
					DNSName: "rh-api",
				},
			},
		},
		GroupVersion: cloudingressv1alpha1.SchemeGroupVersion,
	}

	infraObj := testutils.RuntimeObj{
		Obj:          testutils.CreateInfraObject("AWS", testutils.DefaultAPIEndpoint, testutils.DefaultAPIEndpoint, testutils.DefaultRegionName),
		GroupVersion: configv1.SchemeGroupVersion,
	}
	//NLB service
	nlbService := testutils.RuntimeObj{
		Obj: &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rh-api",
				Namespace: "openshift-kube-apiserver",
			},
		},
		GroupVersion: v1.SchemeGroupVersion,
	}

	tests := []struct {
		Name          string
		Resp          reconcile.Result
		ClientObj     []testutils.ClientObj
		RuntimeObj    []testutils.RuntimeObj
		ClientErr     map[string]string // used to instruct the client to generate an error on k8sclient Update, Delete or Create
		ErrorExpected bool
		ErrorReason   string
		CloudMockFunc func(client.Client, *mockcc.MockCloudClient, client.Object, runtime.Object, error)
		CloudMockArgs cloudmockargs
	}{
		{
			Name:          "Should complete without error when apischeme is NotFound",
			Resp:          reconcile.Result{},
			ErrorExpected: false,
			ClientObj:     []testutils.ClientObj{defaultApiScheme},
			RuntimeObj:    []testutils.RuntimeObj{infraObj},
			ClientErr:     map[string]string{"on": "Get", "type": "IsNotFound"},
		},
		{
			Name:          "Should error when failing to retrieve the apischeme",
			Resp:          reconcile.Result{},
			ErrorExpected: true,
			ErrorReason:   "InternalError",
			ClientObj:     []testutils.ClientObj{defaultApiScheme},
			RuntimeObj:    []testutils.RuntimeObj{infraObj},
			ClientErr:     map[string]string{"on": "Get", "type": "InternalError"},
		},
		{
			Name:          "Should complete without error when apischeme is not managed",
			Resp:          reconcile.Result{},
			ErrorExpected: false,
			ClientObj:     []testutils.ClientObj{defaultApiScheme},
			RuntimeObj:    []testutils.RuntimeObj{infraObj},
		},
		{
			Name:          "Should error when unable to retrieve cloudclient",
			Resp:          reconcile.Result{},
			ErrorExpected: true,
			ErrorReason:   "NotFound",
			ClientObj:     []testutils.ClientObj{managedApiScheme},
			RuntimeObj:    []testutils.RuntimeObj{},
		},
		{
			Name:          "Should error when failing to update finalizer on APIScheme",
			Resp:          reconcile.Result{},
			ErrorExpected: true,
			ErrorReason:   "InternalError",
			ClientObj:     []testutils.ClientObj{managedApiScheme},
			RuntimeObj:    []testutils.RuntimeObj{infraObj},
			ClientErr:     map[string]string{"on": "Update", "type": "InternalError"},
		},
		{
			Name:          "Should complete without error when apischeme is being deleted and has no finalizer",
			Resp:          reconcile.Result{},
			ErrorExpected: false,
			ClientObj:     []testutils.ClientObj{apischemeWithDeleteTimestampNoFinalizer},
			RuntimeObj:    []testutils.RuntimeObj{infraObj},
		},
		{
			Name:          "Should requeue without error when rh-api NLB in namespace openshift-kube-apiserver is not found",
			Resp:          reconcile.Result{Requeue: true},
			ErrorExpected: false,
			ClientObj:     []testutils.ClientObj{apischemeWithDeleteTimestampWithFinalizer},
			RuntimeObj:    []testutils.RuntimeObj{infraObj},
		},
		{
			Name:          "Should error when failing to retrieve rh-api NLB in namespace openshift-kube-apiserver",
			Resp:          reconcile.Result{},
			ErrorExpected: true,
			ErrorReason:   "InternalError",
			ClientObj:     []testutils.ClientObj{apischemeWithDeleteTimestampWithFinalizer},
			RuntimeObj:    []testutils.RuntimeObj{infraObj},
			ClientErr:     map[string]string{"on": "Get", "type": "InternalError", "target": "*v1.Service"},
		},
		{
			Name:          "Should requeue without error when successfully removing admin API A (DNS) record",
			Resp:          reconcile.Result{Requeue: true},
			ErrorExpected: false,
			ClientObj:     []testutils.ClientObj{apischemeWithDeleteTimestampWithFinalizer},
			RuntimeObj:    []testutils.RuntimeObj{infraObj, nlbService},
			CloudMockFunc: mockCloudExpectDeleteAdminAPIDNS,
			CloudMockArgs: cloudmockargs{
				cloud: cloud,
				cobj:  apischemeWithDeleteTimestampWithFinalizer.Obj,
				robj:  nlbService.Obj,
			},
		},
		{
			Name:          "Should requeue with delay and without error when LB is not yet ready",
			Resp:          reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second},
			ErrorExpected: false,
			ClientObj:     []testutils.ClientObj{apischemeWithDeleteTimestampWithFinalizer},
			RuntimeObj:    []testutils.RuntimeObj{infraObj, nlbService},
			CloudMockFunc: mockCloudExpectDeleteAdminAPIDNS,
			CloudMockArgs: cloudmockargs{
				cloud: cloud,
				cobj:  apischemeWithDeleteTimestampWithFinalizer.Obj,
				robj:  nlbService.Obj,
				err:   cioerrors.NewLoadBalancerNotReadyError(),
			},
		},
		{
			Name:          "Should error when failing to update the DNS Record",
			Resp:          reconcile.Result{},
			ErrorExpected: true,
			ErrorReason:   "",
			ClientObj:     []testutils.ClientObj{apischemeWithDeleteTimestampWithFinalizer},
			RuntimeObj:    []testutils.RuntimeObj{infraObj, nlbService},
			CloudMockFunc: mockCloudExpectDeleteAdminAPIDNS,
			CloudMockArgs: cloudmockargs{
				cloud: cloud,
				cobj:  apischemeWithDeleteTimestampWithFinalizer.Obj,
				robj:  nlbService.Obj,
				err:   cioerrors.NewDNSUpdateError("bad"),
			},
		},
	}

	for _, test := range tests {
		testClient, testScheme := testutils.SetUpTestClient(test.ClientObj, test.RuntimeObj, test.ClientErr["on"], test.ClientErr["type"], test.ClientErr["target"])

		if test.CloudMockFunc != nil {
			test.CloudMockFunc(testClient, test.CloudMockArgs.cloud, test.CloudMockArgs.cobj, test.CloudMockArgs.robj, test.CloudMockArgs.err)
		}

		r := &ReconcileAPIScheme{client: testClient, scheme: testScheme}
		result, err := r.Reconcile(context.TODO(), reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      name,
				Namespace: namespace,
			},
		})

		if err == nil && test.ErrorExpected || err != nil && !test.ErrorExpected {
			t.Fatalf("Test [%v] return mismatch. Expect error? %t: Return %+v", test.Name, test.ErrorExpected, err)
		}
		if err != nil && test.ErrorExpected && test.ErrorReason != fmt.Sprint(k8serr.ReasonForError(err)) {
			t.Fatalf("Test [%v] FAILED. Excepted Error %v. Got %v", test.Name, test.ErrorReason, k8serr.ReasonForError(err))
		}
		if result != test.Resp {
			t.Fatalf("Test [%v] FAILED. Excepted Response %v. Got %v", test.Name, test.Resp, result)
		}
	}
}

// set up a gomock matcher to test if something is the right type
type ofType struct{ t string }

func (o *ofType) Matches(x interface{}) bool {
	return reflect.TypeOf(x).String() == o.t
}

func (o *ofType) String() string {
	return "is of type " + o.t
}

func OfType(t string) gomock.Matcher {
	return &ofType{t}
}

type cloudmockargs struct {
	cloud *mockcc.MockCloudClient
	cobj  client.Object
	robj  runtime.Object
	err   error
}

func mockCloudExpectDeleteAdminAPIDNS(client client.Client, cloud *mockcc.MockCloudClient, apischeme client.Object, lbservice runtime.Object, err error) {
	cloud.EXPECT().DeleteAdminAPIDNS(context.TODO(), client, OfType(reflect.TypeOf(apischeme).String()), OfType(reflect.TypeOf(lbservice).String())).Return(err)
}
