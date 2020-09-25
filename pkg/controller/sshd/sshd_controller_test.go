package sshd

import (
	"crypto/rsa"
	"errors"
	"reflect"
	"testing"

	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	mockAwsClient "github.com/openshift/cloud-ingress-operator/pkg/awsclient/mock"

	"golang.org/x/crypto/ssh"

	"github.com/golang/mock/gomock"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	// TODO: Use a fake client and cloud-service interface
	//       mocking to test ReconcileSSHD.Reconcile()
	//"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

func TestEnsureDNSRecords(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	awsClient := mockAwsClient.NewMockClient(ctrl)
	awsClient.EXPECT().UpsertARecord("privateHostedZoneName", "loadBalancerDNSName", "loadBalancerHostedZoneId", "resourceRecordSetName", "RH SSH Endpoint", false)
	awsClient.EXPECT().UpsertARecord("publicHostedZoneName", "loadBalancerDNSName", "loadBalancerHostedZoneId", "resourceRecordSetName", "RH SSH Endpoint", false)

	testClient, testScheme := setUpTestClient(t)
	r := &ReconcileSSHD{
		client:    testClient,
		scheme:    testScheme,
		awsClient: awsClient,
		route53: &Route53Data{
			loadBalancerDNSName:      "loadBalancerDNSName",
			loadBalancerHostedZoneId: "loadBalancerHostedZoneId",
			resourceRecordSetName:    "resourceRecordSetName",
			privateHostedZoneName:    "privateHostedZoneName",
			publicHostedZoneName:     "publicHostedZoneName",
		},
	}

	err := r.ensureDNSRecords()
	if err != nil {
		t.Fatalf("got an unexpected error: %s", err)
	}
}

func TestDeleteDNSRecords(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	awsClient := mockAwsClient.NewMockClient(ctrl)
	awsClient.EXPECT().DeleteARecord("privateHostedZoneName", "loadBalancerDNSName", "loadBalancerHostedZoneId", "resourceRecordSetName", false)
	awsClient.EXPECT().DeleteARecord("publicHostedZoneName", "loadBalancerDNSName", "loadBalancerHostedZoneId", "resourceRecordSetName", false)

	testClient, testScheme := setUpTestClient(t)
	r := &ReconcileSSHD{
		client:    testClient,
		scheme:    testScheme,
		awsClient: awsClient,
		route53: &Route53Data{
			loadBalancerDNSName:      "loadBalancerDNSName",
			loadBalancerHostedZoneId: "loadBalancerHostedZoneId",
			resourceRecordSetName:    "resourceRecordSetName",
			privateHostedZoneName:    "privateHostedZoneName",
			publicHostedZoneName:     "publicHostedZoneName",
		},
	}

	err := r.deleteDNSRecords()
	if err != nil {
		t.Fatalf("got an unexpected error: %s", err)
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

func TestNewSSHSecret(t *testing.T) {
	hostKeysSecret, err := newSSHDSecret(placeholderNamespace, "host-keys")
	if err != nil {
		t.Fatal("Failed to generate host keys:", err)
	}

	for _, pemBytes := range hostKeysSecret.Data {
		privateKey, err := ssh.ParseRawPrivateKey(pemBytes)
		if err != nil {
			t.Fatal("Failed to parse private key:", err)
		}
		switch privateKey := privateKey.(type) {
		case *rsa.PrivateKey:
			if err := privateKey.Validate(); err != nil {
				t.Fatal("RSA key is invalid:", err)
			}
			if privateKey.Size() != rsaKeyModulusSize {
				t.Errorf("RSA key has wrong modulus size %d bits; expected %d bits",
					privateKey.Size()*8, rsaKeyModulusSize*8)
			}
		// XXX Handle other host key types if/when the controller adds them.
		default:
			t.Fatalf("Unexpected private key type: %T", privateKey)
		}
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
	},
	Spec: cloudingressv1alpha1.SSHDSpec{
		AllowedCIDRBlocks: []string{"1.1.1.1", "2.2.2.2"},
		Image:             placeholderImage,
	},
}

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

	objects := []runtime.Object{cr}

	testClient = fake.NewFakeClientWithScheme(s, objects...)
	return
}
