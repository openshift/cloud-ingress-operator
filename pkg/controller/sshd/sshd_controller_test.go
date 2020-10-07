package sshd

import (
	"context"
	"errors"
	"reflect"
	"testing"

	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	mockcc "github.com/openshift/cloud-ingress-operator/pkg/cloudclient/mock_cloudclient"

	"github.com/golang/mock/gomock"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	placeholderName      string = "placeholderName"
	placeholderNamespace string = "placeholderNamespace"
	placeholderImage     string = "placeholderImage"

	rsaKeyModulusSize int = (4096 / 8)
)

// tests
func TestSetSSHDStatus(t *testing.T) {
	tests := []struct {
		Name        string
		Message     string
		State       cloudingressv1alpha1.SSHDStateType
		ExpectError bool
	}{
		{
			Name:    "set status",
			Message: "working as expected",
			State:   cloudingressv1alpha1.SSHDStateReady,
		},
	}

	for _, test := range tests {
		testClient, testScheme := setUpTestClient(t)
		r := &ReconcileSSHD{client: testClient, scheme: testScheme}
		r.SetSSHDStatus(cr, test.Message, test.State)

		if cr.Status.Message != test.Message {
			t.Errorf("test: %s; status was %s, expected %s\n", test.Name, cr.Status.Message, test.Message)
		}

		if cr.Status.State != test.State {
			t.Errorf("test: %s; state was %s, expected %s\n", test.Name, cr.Status.State, test.State)
		}
	}
}

func TestSetSSHDStatusPending(t *testing.T) {
	tests := []struct {
		Name        string
		Message     string
		ExpectError bool
	}{
		{
			Name:    "set status",
			Message: "working as expected",
		},
	}

	for _, test := range tests {
		testClient, testScheme := setUpTestClient(t)
		r := &ReconcileSSHD{client: testClient, scheme: testScheme}
		r.SetSSHDStatusPending(cr, test.Message)

		if cr.Status.Message != test.Message {
			t.Errorf("test: %s; status was %s, expected %s\n", test.Name, cr.Status.Message, test.Message)
		}

		if cr.Status.State != cloudingressv1alpha1.SSHDStatePending {
			t.Errorf("test: %s; state was %s, expected %s\n", test.Name, cr.Status.State, cloudingressv1alpha1.SSHDStatePending)
		}
	}
}

func TestSetSSHDStatusError(t *testing.T) {
	tests := []struct {
		Name        string
		Message     string
		ExpectError bool
	}{
		{
			Name:    "set status",
			Message: "working as expected",
		},
	}

	for _, test := range tests {
		testClient, testScheme := setUpTestClient(t)
		r := &ReconcileSSHD{client: testClient, scheme: testScheme}
		r.SetSSHDStatusError(cr, test.Message, errors.New("fake error"))

		if cr.Status.Message != test.Message {
			t.Errorf("test: %s; status was %s, expected %s\n", test.Name, cr.Status.Message, test.Message)
		}

		if cr.Status.State != cloudingressv1alpha1.SSHDStateError {
			t.Errorf("test: %s; state was %s, expected %s\n", test.Name, cr.Status.State, cloudingressv1alpha1.SSHDStateError)
		}
	}
}

func TestNewSSHDeployment(t *testing.T) {
	var configMapList *corev1.ConfigMapList
	var deployment *appsv1.Deployment
	const hostKeysName string = "host-keys"

	hostKeysSecret, err := newSSHDSecret(placeholderNamespace, hostKeysName)
	if err != nil {
		t.Fatal("Failed to generate host keys:", err)
	}

	// Verify SSHD parameters are honored
	configMapList = newConfigMapList()
	deployment = newSSHDDeployment(cr, configMapList, hostKeysSecret)
	if deployment.ObjectMeta.Name != cr.ObjectMeta.Name {
		t.Errorf("Deployment has wrong name %q, expected %q",
			deployment.ObjectMeta.Name, cr.ObjectMeta.Name)
	}
	if deployment.ObjectMeta.Namespace != cr.ObjectMeta.Namespace {
		t.Errorf("Deployment has wrong namespace %q, expected %q",
			deployment.ObjectMeta.Namespace, cr.ObjectMeta.Namespace)
	}
	if !reflect.DeepEqual(deployment.Spec.Selector.MatchLabels, getMatchLabels(cr)) {
		t.Errorf("Deployment has wrong selector %v, expected %v",
			deployment.Spec.Selector.MatchLabels, getMatchLabels(cr))
	}
	if deployment.Spec.Template.ObjectMeta.Name != cr.ObjectMeta.Name {
		t.Errorf("Deployment has wrong pod spec name %q, expected %q",
			deployment.Spec.Template.ObjectMeta.Name, cr.ObjectMeta.Name)
	}
	if deployment.Spec.Template.ObjectMeta.Namespace != cr.ObjectMeta.Namespace {
		t.Errorf("Deployment has wrong pod spec namespace %q, expected %q",
			deployment.Spec.Template.ObjectMeta.Namespace, cr.ObjectMeta.Namespace)
	}
	if !reflect.DeepEqual(deployment.Spec.Template.ObjectMeta.Labels, getMatchLabels(cr)) {
		t.Errorf("Deployment has wrong pod spec labels %v, expected %v",
			deployment.Spec.Template.ObjectMeta.Labels, getMatchLabels(cr))
	}
	if deployment.Spec.Template.Spec.Containers[0].Image != cr.Spec.Image {
		t.Errorf("Deployment has wrong container image %q, expected %q",
			deployment.Spec.Template.Spec.Containers[0].Image, cr.Spec.Image)
	}

	// Verify no config maps yields only the host keys volume
	if len(deployment.Spec.Template.Spec.Volumes) < 1 {
		t.Error("Deployment is missing a volume for host keys")
	} else if len(deployment.Spec.Template.Spec.Volumes) > 1 {
		t.Errorf("Deployment has unexpected volumes: %v",
			deployment.Spec.Template.Spec.Volumes)
	} else if deployment.Spec.Template.Spec.Volumes[0].Name != hostKeysName {
		t.Errorf("Volume in deployment does not appear to be for host keys: %v",
			deployment.Spec.Template.Spec.Volumes[0])
	}
	if len(deployment.Spec.Template.Spec.Containers[0].VolumeMounts) < 1 {
		t.Error("Deployment is missing a volume mount for host keys")
	} else if len(deployment.Spec.Template.Spec.Containers[0].VolumeMounts) > 1 {
		t.Errorf("Deployment has unexpected volume mounts in container: %v",
			deployment.Spec.Template.Spec.Containers[0].VolumeMounts)
	} else if deployment.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name != hostKeysName {
		t.Errorf("Volume mount in container does not appear to be for host keys: %v",
			deployment.Spec.Template.Spec.Containers[0].VolumeMounts[0])
	}

	// Verify config maps are handled properly
	configMapList = newConfigMapList("A", "B")
	deployment = newSSHDDeployment(cr, configMapList, hostKeysSecret)
	// Plus one volume for the host key secret.
	if len(deployment.Spec.Template.Spec.Volumes) != len(configMapList.Items)+1 {
		t.Errorf("Volumes are wrong in deployment, found %d, expected %d",
			len(deployment.Spec.Template.Spec.Volumes),
			len(configMapList.Items))
	}
	// Plus one volume mount for the host key secret.
	if len(deployment.Spec.Template.Spec.Containers[0].VolumeMounts) != len(configMapList.Items)+1 {
		t.Errorf("Container's volume mounts are wrong in deployment, found %d, expected %d",
			len(deployment.Spec.Template.Spec.Containers[0].VolumeMounts),
			len(configMapList.Items))
	}
	for index, configMap := range configMapList.Items {
		volume := &deployment.Spec.Template.Spec.Volumes[index]
		if volume.Name != configMap.ObjectMeta.Name {
			t.Errorf("Volume %d has wrong name %q, expected %q",
				index, volume.Name, configMap.ObjectMeta.Name)
		}
		if volume.VolumeSource.ConfigMap.LocalObjectReference.Name != configMap.ObjectMeta.Name {
			t.Errorf("Volume %d references wrong config map %q, expected %q",
				index, volume.VolumeSource.ConfigMap.LocalObjectReference.Name,
				configMap.ObjectMeta.Name)
		}

		volumeMount := &deployment.Spec.Template.Spec.Containers[0].VolumeMounts[index]
		if volumeMount.Name != configMap.ObjectMeta.Name {
			t.Errorf("Volume mount %d has wrong name %q, expected %q",
				index, volumeMount.Name, configMap.ObjectMeta.Name)
		}
	}
}

func TestNewSSHService(t *testing.T) {
	var service *corev1.Service

	// Verify SSHD parameters are honored
	service = newSSHDService(cr)
	if service.ObjectMeta.Name != cr.ObjectMeta.Name {
		t.Errorf("Service has wrong name %q, expected %q",
			service.ObjectMeta.Name, cr.ObjectMeta.Name)
	}
	if service.ObjectMeta.Namespace != cr.ObjectMeta.Namespace {
		t.Errorf("Service has wrong namespace %q, expected %q",
			service.ObjectMeta.Namespace, cr.ObjectMeta.Namespace)
	}
	if !reflect.DeepEqual(service.Spec.Selector, getMatchLabels(cr)) {
		t.Errorf("Service has wrong selector %v, expected %v",
			service.Spec.Selector, getMatchLabels(cr))
	}
	if !reflect.DeepEqual(service.Spec.LoadBalancerSourceRanges, cr.Spec.AllowedCIDRBlocks) {
		t.Errorf("Service has wrong source ranges %v, expected %v",
			service.Spec.LoadBalancerSourceRanges, cr.Spec.AllowedCIDRBlocks)
	}
}

func TestReconcile(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	testClient, testScheme := setUpTestClient(t)
	cloud := mockcc.NewMockCloudClient(ctrl)

	cloud.EXPECT().EnsureSSHDNS(context.TODO(), testClient, OfType(reflect.TypeOf(cr).String()), svc)

	r := &ReconcileSSHD{
		client:      testClient,
		scheme:      testScheme,
		cloudClient: cloud,
	}

	result, err := r.Reconcile(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      placeholderName,
			Namespace: placeholderNamespace,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if result.Requeue {
		t.Errorf("got requeue on %v", result)
	}
}

// utils
var cr = &cloudingressv1alpha1.SSHD{
	TypeMeta: metav1.TypeMeta{
		Kind:       "SSHD",
		APIVersion: cloudingressv1alpha1.SchemeGroupVersion.String(),
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:      placeholderName,
		Namespace: placeholderNamespace,
		Finalizers: []string{
			reconcileSSHDFinalizerDNS,
		},
	},
	Spec: cloudingressv1alpha1.SSHDSpec{
		AllowedCIDRBlocks: []string{"1.1.1.1", "2.2.2.2"},
		Image:             placeholderImage,
	},
}

var svc = newSSHDService(cr)

func newConfigMap(name string) corev1.ConfigMap {
	return corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: placeholderNamespace,
			Labels: map[string]string{
				"api.openshift.com/authorized-keys": name,
			},
		},
		Data: map[string]string{
			"authorized_keys": "ssh-rsa R0lCQkVSSVNIIQ==",
		},
	}
}

func newConfigMapList(names ...string) *corev1.ConfigMapList {
	items := []corev1.ConfigMap{}
	for _, name := range names {
		items = append(items, newConfigMap(name))
	}
	return &corev1.ConfigMapList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMapList",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		Items: items,
	}
}

func setUpTestClient(t *testing.T) (testClient client.Client, s *runtime.Scheme) {
	t.Helper()

	s = scheme.Scheme
	s.AddKnownTypes(cloudingressv1alpha1.SchemeGroupVersion, cr)

	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      placeholderName + "-host-keys",
			Namespace: placeholderNamespace,
		},
		Data: map[string][]byte{
			"ssh_host_rsa_key": []byte("somefakedata"),
		},
	}

	objects := []runtime.Object{cr, svc, secret}

	testClient = fake.NewFakeClientWithScheme(s, objects...)
	return
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
