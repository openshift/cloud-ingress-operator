package sshd

import (
	"context"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	// For host key generation
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"

	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	"github.com/openshift/cloud-ingress-operator/pkg/cloudclient"
	cioerrors "github.com/openshift/cloud-ingress-operator/pkg/errors"

	utils "github.com/openshift/cloud-ingress-operator/pkg/controller/utils"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_sshd")

// Add creates a new SSHD Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileSSHD{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("sshd-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// NOTE: The primary resource and the child resources it owns
	//       will exist in a different namespace.  Normally these
	//       watches would not work across namespaces but the pod
	//       spec for this operator specifies WATCH_NAMESPACE="",
	//       which gets passed to the Manager object in main() to
	//       enable cluster-wide watches.

	// Watch for changes to primary resource SSHD
	err = c.Watch(&source.Kind{Type: &cloudingressv1alpha1.SSHD{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to Deployments
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &cloudingressv1alpha1.SSHD{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to Services
	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &cloudingressv1alpha1.SSHD{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileSSHD implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileSSHD{}

type Route53Data struct {
	loadBalancerDNSName      string
	loadBalancerHostedZoneId string
	resourceRecordSetName    string
	privateHostedZoneName    string
	publicHostedZoneName     string
}

// ReconcileSSHD reconciles a SSHD object
type ReconcileSSHD struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme

	cloudClient cloudclient.CloudClient
}

const (
	authorizedKeysMountPath   = "/var/run/authorized_keys.d"
	hostKeysMountPath         = "/var/run/ssh"
	nodeMasterLabel           = "node-role.kubernetes.io/master"
	reconcileSSHDFinalizerDNS = "dns.cloudingress.managed.openshift.io"
	ELBAnnotationKey          = "service.beta.kubernetes.io/aws-load-balancer-connection-idle-timeout"
	ELBAnnotationValue        = "600"
)

// Reconcile reads that state of the cluster for a SSHD object and makes changes based on the state read
// and what is in the SSHD.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileSSHD) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling SSHD")

	// Fetch the SSHD instance
	instance := &cloudingressv1alpha1.SSHD{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Ensure we have a cloudClient instance.
	if r.cloudClient == nil {
		platform, err := utils.GetPlatformType(r.client)
		if err != nil {
			r.SetSSHDStatusError(instance, "Failed to get cluster's platform", err)
			return reconcile.Result{}, err
		}

		r.cloudClient = cloudclient.GetClientFor(r.client, *platform)
	}

	// Check for a deletion timestamp.
	if instance.DeletionTimestamp.IsZero() {
		// Request object is alive, so ensure it has the DNS finalizer.
		if !controllerutil.ContainsFinalizer(instance, reconcileSSHDFinalizerDNS) {
			controllerutil.AddFinalizer(instance, reconcileSSHDFinalizerDNS)
			if err = r.client.Update(context.TODO(), instance); err != nil {
				return reconcile.Result{}, err
			}
		}
	} else {
		// Request object is being deleted.
		if controllerutil.ContainsFinalizer(instance, reconcileSSHDFinalizerDNS) {
			r.SetSSHDStatus(instance, "Deleting DNS aliases", cloudingressv1alpha1.SSHDStateFinalizing)

			// fetch the sshd service
			svc := &corev1.Service{}
			err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name}, svc)
			if err != nil {
				return reconcile.Result{}, err
			}

			err = r.cloudClient.DeleteSSHDNS(context.TODO(), r.client, instance, svc)
			switch err {
			case nil:
				// all good
			case err.(*cioerrors.LoadBalancerNotFoundError):
				// couldn't find the load balancer - it's likely still queued for creation
				r.SetSSHDStatus(instance, "Couldn't reconcile", "Load balancer isn't ready.")
				r.client.Status().Update(context.TODO(), instance)
				return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, nil
			default:
				r.SetSSHDStatusError(instance, "Failed to delete the DNS record", err)
				return reconcile.Result{}, err
			}

			// Remove the DNS finalizer and update the request object.
			controllerutil.RemoveFinalizer(instance, reconcileSSHDFinalizerDNS)
			if err = r.client.Update(context.TODO(), instance); err != nil {
				return reconcile.Result{}, err
			}
		}

		// Halt the reconciliation.
		return reconcile.Result{}, nil
	}

	// List ConfigMaps with SSH keys
	//
	// Each internal team that should have SSH access to OSDv4 clusters has a unique
	// ConfigMap object with all team members SSH keys in a single "authorized_keys"
	// file, as well as a SelectorSyncSet object on Hive that syncs the ConfigMap to
	// appropriate clusters for the team.
	//
	// The Deployment object will be configured to mount each available ConfigMap in
	// the SSHD pod as a volume under a common directory.  The SSH server within the
	// pod will use an "AuthorizedKeysCommand" to combine all the mounted authorized
	// keys files under that common directory.
	//
	// Updates to ConfigMaps for new or departing members, as well as new ConfigMaps
	// for new teams, may incur up to a 60 second delay before being reconciled into
	// the deployed SSHD pod.
	configMapList := &corev1.ConfigMapList{}
	selector, err := metav1.LabelSelectorAsSelector(&instance.Spec.ConfigMapSelector)
	if err != nil {
		return reconcile.Result{}, err
	}
	if err = r.client.List(context.TODO(), configMapList,
		client.InNamespace(instance.Namespace),
		&client.MatchingLabelsSelector{Selector: selector}); err != nil {
		r.SetSSHDStatusError(instance, "Failed to list config maps with SSH keys", err)
		return reconcile.Result{}, err
	}

	// Install "host-keys" Secret
	//
	// Since host key generation has a random component and is therefore
	// different each time, only call newSSHDSecret if an existing secret
	// cannot be found.
	hostKeysSecret := &corev1.Secret{}
	secretName := types.NamespacedName{
		Namespace: instance.Namespace,
		Name:      instance.Name + "-host-keys",
	}
	if err = r.client.Get(context.TODO(), secretName, hostKeysSecret); err != nil {
		if errors.IsNotFound(err) {
			// Create a new "host-keys" Secret.
			r.SetSSHDStatusPending(instance, "Generating host keys")
			secret, err := newSSHDSecret(secretName.Namespace, secretName.Name)
			if err != nil {
				r.SetSSHDStatusError(instance, "Failed to generate host keys", err)
				return reconcile.Result{}, err
			}
			if err := controllerutil.SetControllerReference(instance, secret, r.scheme); err != nil {
				r.SetSSHDStatusError(instance, "Failed to set secret controller reference", err)
				return reconcile.Result{}, err
			}
			if err = r.client.Create(context.TODO(), secret); err != nil {
				if errors.IsAlreadyExists(err) {
					return reconcile.Result{Requeue: true}, nil
				}
				r.SetSSHDStatusError(instance, "Failed to create secret", err)
				return reconcile.Result{}, err
			}
			// Get the created secret on the next pass.
			return reconcile.Result{Requeue: true}, nil
		} else {
			return reconcile.Result{}, err
		}
	}

	// Install Deployment
	foundDeployment := &appsv1.Deployment{}
	deployment := newSSHDDeployment(instance, configMapList, hostKeysSecret)
	deploymentName, err := client.ObjectKeyFromObject(deployment)
	if err != nil {
		return reconcile.Result{}, err
	}
	if err = r.client.Get(context.TODO(), deploymentName, foundDeployment); err != nil {
		if errors.IsNotFound(err) {
			// Create a new Deployment.
			r.SetSSHDStatusPending(instance, "Creating deployment")
			if err := controllerutil.SetControllerReference(instance, deployment, r.scheme); err != nil {
				r.SetSSHDStatusError(instance, "Failed to set deployment controller reference", err)
				return reconcile.Result{}, err
			}
			if err = r.client.Create(context.TODO(), deployment); err != nil {
				if errors.IsAlreadyExists(err) {
					return reconcile.Result{Requeue: true}, nil
				}
				r.SetSSHDStatusError(instance, "Failed to create deployment", err)
				return reconcile.Result{}, err
			}
		} else {
			return reconcile.Result{}, err
		}
	} else {
		// Deployment exists, check if it's updated.
		if !reflect.DeepEqual(foundDeployment.Spec, deployment.Spec) {
			// Specs aren't equal, update and fix.
			r.SetSSHDStatusPending(instance, "Updating deployment", "from", foundDeployment.Spec, "to", deployment.Spec)
			foundDeployment.Spec = *deployment.Spec.DeepCopy()
			if err = r.client.Update(context.TODO(), foundDeployment); err != nil {
				r.SetSSHDStatusError(instance, "Failed to update deployment", err)
				return reconcile.Result{}, err
			}
		}
	}

	// Install Service
	foundService := &corev1.Service{}
	service := newSSHDService(instance)
	serviceName, err := client.ObjectKeyFromObject(service)
	if err != nil {
		return reconcile.Result{}, err
	}
	if err = r.client.Get(context.TODO(), serviceName, foundService); err != nil {
		if errors.IsNotFound(err) {
			// Create a new Service.
			r.SetSSHDStatusPending(instance, "Creating service")
			if err = controllerutil.SetControllerReference(instance, service, r.scheme); err != nil {
				r.SetSSHDStatusError(instance, "Failed to set service controller reference", err)
				return reconcile.Result{}, err
			}
			if err = r.client.Create(context.TODO(), service); err != nil {
				if errors.IsAlreadyExists(err) {
					return reconcile.Result{Requeue: true}, nil
				}
				r.SetSSHDStatusError(instance, "Failed to create service", err)
				return reconcile.Result{}, err
			}
			// Reconcile again to get the new Service and give AWS time to create the ELB.
			reqLogger.Info("Service was just created, so let's try to requeue to set it up")
			return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, nil
		} else {
			return reconcile.Result{}, err
		}
	} else {
		var serviceNeedsUpdate bool

		// Service exists, check if annotations or spec need updated.

		if !metav1.HasAnnotation(foundService.ObjectMeta, ELBAnnotationKey) ||
			foundService.ObjectMeta.Annotations[ELBAnnotationKey] != ELBAnnotationValue {
			r.SetSSHDStatusPending(instance, "Updating service annotations")
			metav1.SetMetaDataAnnotation(&foundService.ObjectMeta, ELBAnnotationKey, ELBAnnotationValue)
			serviceNeedsUpdate = true
		}

		// XXX Copy system-assigned fields to satisfy reflect.DeepEqual.
		service.Spec.Ports[0].NodePort = foundService.Spec.Ports[0].NodePort
		service.Spec.ClusterIP = foundService.Spec.ClusterIP
		service.Spec.HealthCheckNodePort = foundService.Spec.HealthCheckNodePort
		if !reflect.DeepEqual(foundService.Spec, service.Spec) {
			r.SetSSHDStatusPending(instance, "Updating service", "from", foundService.Spec, "to", service.Spec)
			foundService.Spec = *service.Spec.DeepCopy()
			serviceNeedsUpdate = true
		}

		if serviceNeedsUpdate {
			if err = r.client.Update(context.TODO(), foundService); err != nil {
				r.SetSSHDStatusError(instance, "Failed to update service", err)
				return reconcile.Result{}, err
			}
			// Requeue to give AWS time to apply the changes.
			reqLogger.Info("Requeuing after service update")
			return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, nil
		}
	}

	err = r.cloudClient.EnsureSSHDNS(context.TODO(), r.client, instance, service)
	switch err {
	case nil:
		// all good
	case err.(*cioerrors.LoadBalancerNotFoundError):
		// couldn't find the new load balancer yet
		r.SetSSHDStatus(instance, "Couldn't reconcile", "Load balancer isn't ready yet.")
		r.client.Status().Update(context.TODO(), instance)
		return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, nil
	default:
		r.SetSSHDStatusError(instance, "Failed to ensure the DNS record", err)
		return reconcile.Result{}, err
	}

	r.SetSSHDStatus(instance, "SSHD is ready", cloudingressv1alpha1.SSHDStateReady)

	return reconcile.Result{}, nil
}

func getMatchLabels(cr *cloudingressv1alpha1.SSHD) map[string]string {
	return map[string]string{"deployment": cr.Name}
}

func newSSHDSecret(namespace, name string) (*corev1.Secret, error) {
	// Generate 4096-bit RSA key
	rsaKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, err
	}
	ssh_host_rsa_key := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(rsaKey),
	})

	// XXX Generate other key types?
	//     ECDSA:   Easy, fully supported in standard library.
	//     ED25519: Standard library can generate a private key, but requires an
	//              external module to create an "OPENSSH PRIVATE KEY" PEM block:
	//              https://github.com/mikesmitty/edkey

	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			// Key also serves as an environment variable name
			// (uppercased) and must be a valid C_IDENTIFIER.
			"ssh_host_rsa_key": ssh_host_rsa_key,
		},
	}, nil
}

func newSSHDDeployment(cr *cloudingressv1alpha1.SSHD, configMapList *corev1.ConfigMapList, hostKeysSecret *corev1.Secret) *appsv1.Deployment {
	// Prefer nil over empty slices to satisfy reflect.DeepEqual.
	var volumes []corev1.Volume
	var volumeMounts []corev1.VolumeMount

	for _, configMap := range configMapList.Items {
		volumeName := configMap.ObjectMeta.Name
		volumes = append(volumes, corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configMap.ObjectMeta.Name,
					},
					DefaultMode: pointer.Int32Ptr(0600),
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      volumeName,
			MountPath: filepath.Join(authorizedKeysMountPath, volumeName),
		})
	}
	// Sort volume slices by name to keep the sequence stable.
	sort.Slice(volumes, func(i, j int) bool {
		return volumes[i].Name < volumes[j].Name
	})
	sort.Slice(volumeMounts, func(i, j int) bool {
		return volumeMounts[i].Name < volumeMounts[j].Name
	})

	// Because the OpenSSH server runs as an arbitrary user instead of root,
	// and the mounted host keys are owned by root, we rely on the container
	// user always being a member of the root group*, and set the permission
	// to be both owner and group-readable (0440).
	//
	// Normally group-readable private keys are forbidden by OpenSSH, but it
	// turns out the server does not apply its strict permission checks when
	// a private key is owned by a different user than itself.
	//
	// * See "Support arbitrary user ids" section in:
	//   https://docs.openshift.com/container-platform/4.5/openshift_images/create-images.html#images-create-guide-openshift_create-images
	volumeName := hostKeysSecret.ObjectMeta.Name
	volumes = append(volumes, corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  hostKeysSecret.ObjectMeta.Name,
				DefaultMode: pointer.Int32Ptr(0440),
				Optional:    pointer.BoolPtr(false),
			},
		},
	})
	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      volumeName,
		MountPath: hostKeysMountPath,
	})

	// Add environment variables to help the container configure itself.
	var env []corev1.EnvVar
	env = append(env, corev1.EnvVar{
		Name:  "AUTHORIZED_KEYS_DIR",
		Value: authorizedKeysMountPath,
	})
	for key := range hostKeysSecret.Data {
		env = append(env, corev1.EnvVar{
			Name:  strings.ToUpper(key),
			Value: filepath.Join(hostKeysMountPath, key),
		})
	}
	sort.Slice(env, func(i, j int) bool {
		return env[i].Name < env[j].Name
	})

	sshdContainer := corev1.Container{
		Name:    "sshd",
		Image:   cr.Spec.Image,
		Command: []string{"/opt/start-sshd.sh"},
		Ports: []corev1.ContainerPort{
			{
				Name:          "ssh",
				ContainerPort: int32(2222),
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env:                      env,
		VolumeMounts:             volumeMounts,
		TerminationMessagePath:   "/dev/termination-log",
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
		ImagePullPolicy:          corev1.PullIfNotPresent,
	}

	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: appsv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: getMatchLabels(cr),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cr.Name,
					Namespace: cr.Namespace,
					Labels:    getMatchLabels(cr),
				},
				Spec: corev1.PodSpec{
					Volumes:                       volumes,
					Containers:                    []corev1.Container{sshdContainer},
					RestartPolicy:                 corev1.RestartPolicyAlways,
					TerminationGracePeriodSeconds: pointer.Int64Ptr(30),
					DNSPolicy:                     corev1.DNSClusterFirst,
					SecurityContext:               &corev1.PodSecurityContext{},
					SchedulerName:                 "default-scheduler",
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
								{
									Weight: int32(100),
									Preference: corev1.NodeSelectorTerm{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      nodeMasterLabel,
												Operator: corev1.NodeSelectorOpExists,
											},
										},
									},
								},
							},
						},
					},
					Tolerations: []corev1.Toleration{
						{
							Key:      nodeMasterLabel,
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
			RevisionHistoryLimit:    pointer.Int32Ptr(10),
			ProgressDeadlineSeconds: pointer.Int32Ptr(600),
		},
	}
}

func newSSHDService(cr *cloudingressv1alpha1.SSHD) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Annotations: map[string]string{
				ELBAnnotationKey: ELBAnnotationValue,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "ssh",
					Protocol:   corev1.ProtocolTCP,
					Port:       int32(22),
					TargetPort: intstr.FromInt(2222),
				},
			},
			Selector:                 getMatchLabels(cr),
			Type:                     corev1.ServiceTypeLoadBalancer,
			SessionAffinity:          corev1.ServiceAffinityNone,
			LoadBalancerSourceRanges: cr.Spec.AllowedCIDRBlocks,
			ExternalTrafficPolicy:    corev1.ServiceExternalTrafficPolicyTypeCluster,
		},
	}
}

// SetSSHDStatusPending calls SetSSHDStatus with a Pending condition
func (r *ReconcileSSHD) SetSSHDStatusPending(cr *cloudingressv1alpha1.SSHD, message string, keysAndValues ...interface{}) {
	log.Info(message, keysAndValues...)
	r.SetSSHDStatus(cr, message, cloudingressv1alpha1.SSHDStatePending)
}

// SetSSHDStatusError calls SetSSHDStatus with an Error condition
func (r *ReconcileSSHD) SetSSHDStatusError(cr *cloudingressv1alpha1.SSHD, message string, err error) {
	log.Error(err, message)
	r.SetSSHDStatus(cr, message, cloudingressv1alpha1.SSHDStateError)
}

// SetSSHDStatus updates the status of the SSHD cluster resource
func (r *ReconcileSSHD) SetSSHDStatus(cr *cloudingressv1alpha1.SSHD, message string, state cloudingressv1alpha1.SSHDStateType) {
	cr.Status.State = state
	cr.Status.Message = message

	r.client.Status().Update(context.TODO(), cr)
}
