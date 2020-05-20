package sshd

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	"github.com/openshift/cloud-ingress-operator/pkg/awsclient"
	"github.com/openshift/cloud-ingress-operator/pkg/config"

	utils "github.com/openshift/cloud-ingress-operator/pkg/controller/utils"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
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

	awsClient awsclient.Client
	route53   *Route53Data
}

const (
	nodeMasterLabel           = "node-role.kubernetes.io/master"
	reconcileSSHDFinalizerDNS = "dns.cloudingress.managed.openshift.io"
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

	// Ensure we have an awsClient instance.
	if r.awsClient == nil {
		region, err := utils.GetClusterRegion(r.client)
		if err != nil {
			r.SetSSHDStatusError(instance, "Failed to get cluster's AWS region", err)
			return reconcile.Result{}, err
		}
		r.awsClient, err = awsclient.GetAWSClient(r.client, awsclient.NewAwsClientInput{
			SecretName: config.AWSSecretName,
			NameSpace:  config.OperatorNamespace,
			AwsRegion:  region,
		})
		if err != nil {
			r.SetSSHDStatusError(instance, "Failed to create an AWS client", err)
			return reconcile.Result{}, err
		}
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
		if controllerutil.ContainsFinalizer(instance, reconcileSSHDFinalizerDNS) && r.route53 != nil {
			r.SetSSHDStatus(instance, "Deleting DNS aliases", cloudingressv1alpha1.SSHDStateFinalizing)
			if err = r.deleteDNSRecords(); err != nil {
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

	// Install Deployment
	foundDeployment := &appsv1.Deployment{}
	deployment := newSSHDDeployment(instance)
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
		// Service exists, check if it's updated.
		// XXX Copy system-assigned fields to satisfy reflect.DeepEqual.
		service.Spec.Ports[0].NodePort = foundService.Spec.Ports[0].NodePort
		service.Spec.ClusterIP = foundService.Spec.ClusterIP
		service.Spec.HealthCheckNodePort = foundService.Spec.HealthCheckNodePort
		if !reflect.DeepEqual(foundService.Spec, service.Spec) {
			// Specs aren't equal, update and fix.
			r.SetSSHDStatusPending(instance, "Updating service", "from", foundService.Spec, "to", service.Spec)
			foundService.Spec = *service.Spec.DeepCopy()
			if err = r.client.Update(context.TODO(), foundService); err != nil {
				r.SetSSHDStatusError(instance, "Failed to update service", err)
				return reconcile.Result{}, err
			}
			// Requeue to give AWS time to apply the changes.
			reqLogger.Info("Requeuing after service update")
			return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, nil
		}
	}

	// Create a Route53 DNS entry for the service's load balancer.
	// TODO Consider using https://github.com/openshift/external-dns

	clusterBaseDomain, err := utils.GetClusterBaseDomain(r.client)
	if err != nil {
		r.SetSSHDStatusError(instance, "Failed to get cluster's base domain", err)
		return reconcile.Result{}, err
	}

	// Get the ELB-to-be's name from Service's UID.
	elbName := strings.ReplaceAll("a"+string(foundService.ObjectMeta.UID), "-", "")
	if len(elbName) > 32 {
		// truncate to 32 characters
		elbName = elbName[0:32]
	}

	exists, elb, err := r.awsClient.DoesELBExist(elbName)
	if err != nil {
		r.SetSSHDStatusError(instance, "Failed to get load balancer status", err)
		return reconcile.Result{}, err
	}
	if !exists {
		// It isn't bad that it doesn't exist if there's no error, so re-queue.
		r.SetSSHDStatusPending(instance, "Waiting on service load balancers")
		return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, nil
	}

	r.route53 = &Route53Data{
		loadBalancerDNSName:      elb.DNSName,
		loadBalancerHostedZoneId: elb.DNSZoneId,
		resourceRecordSetName:    instance.Spec.DNSName + "." + clusterBaseDomain,
		privateHostedZoneName:    clusterBaseDomain + ".",
		// The public zone name omits the cluster name.
		// e.g. mycluster.abcd.s1.openshift.com -> abcd.s1.openshift.com
		publicHostedZoneName: clusterBaseDomain[strings.Index(clusterBaseDomain, ".")+1:] + ".",
	}

	err = r.ensureDNSRecords()
	if err != nil {
		r.SetSSHDStatusError(instance, "Failed to ensure the DNS record", err)
		return reconcile.Result{}, err
	}

	r.SetSSHDStatus(instance, "SSHD is ready", cloudingressv1alpha1.SSHDStateReady)

	return reconcile.Result{}, nil
}

func getMatchLabels(cr *cloudingressv1alpha1.SSHD) map[string]string {
	return map[string]string{"deployment": cr.Name}
}

func newSSHDDeployment(cr *cloudingressv1alpha1.SSHD) *appsv1.Deployment {
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
		TerminationMessagePath:   "/dev/termination-log",
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
		ImagePullPolicy:          corev1.PullAlways,
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
					Containers:                    []corev1.Container{sshdContainer},
					RestartPolicy:                 corev1.RestartPolicyAlways,
					TerminationGracePeriodSeconds: pointer.Int64Ptr(30),
					DNSPolicy:                     corev1.DNSClusterFirst,
					NodeSelector:                  map[string]string{nodeMasterLabel: ""},
					SecurityContext:               &corev1.PodSecurityContext{},
					SchedulerName:                 "default-scheduler",
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

func (r *ReconcileSSHD) ensureDNSRecords() error {
	// Private zone
	for i := 1; i <= config.MaxAPIRetries; i++ {
		err := r.awsClient.UpsertARecord(
			r.route53.privateHostedZoneName,
			r.route53.loadBalancerDNSName,
			r.route53.loadBalancerHostedZoneId,
			r.route53.resourceRecordSetName,
			"RH SSH Endpoint", false)
		if err != nil {
			log.Info("Couldn't upsert a DNS record for private zone: " + err.Error())
			if i == config.MaxAPIRetries {
				log.Info("Out of retries for private zone")
				return err
			}
			log.Info(fmt.Sprintf("Sleeping %d seconds before retrying...", i))
			time.Sleep(time.Duration(i) * time.Second)
		} else {
			break
		}
	}

	// Public zone
	for i := 1; i <= config.MaxAPIRetries; i++ {
		// Append a dot to get the zone name.
		err := r.awsClient.UpsertARecord(
			r.route53.publicHostedZoneName,
			r.route53.loadBalancerDNSName,
			r.route53.loadBalancerHostedZoneId,
			r.route53.resourceRecordSetName,
			"RH SSH Endpoint", false)
		if err != nil {
			log.Info("Couldn't upsert a DNS record for public zone: " + err.Error())
			if i == config.MaxAPIRetries {
				log.Info("Out of retries for public zone")
				return err
			}
			log.Info(fmt.Sprintf("Sleeping %d seconds before retrying...", i))
			time.Sleep(time.Duration(i) * time.Second)
		} else {
			break
		}
	}

	return nil
}

func (r *ReconcileSSHD) deleteDNSRecords() error {
	// Private zone
	for i := 1; i <= config.MaxAPIRetries; i++ {
		err := r.awsClient.DeleteARecord(
			r.route53.privateHostedZoneName,
			r.route53.loadBalancerDNSName,
			r.route53.loadBalancerHostedZoneId,
			r.route53.resourceRecordSetName,
			false)
		if err != nil {
			log.Info("Couldn't delete a DNS record for private zone: " + err.Error())
			if i == config.MaxAPIRetries {
				log.Info("Out of retries for private zone")
				return err
			}
			log.Info(fmt.Sprintf("Sleeping %d seconds before retrying...", i))
			time.Sleep(time.Duration(i) * time.Second)
		} else {
			break
		}
	}

	// Public zone
	for i := 1; i <= config.MaxAPIRetries; i++ {
		// Append a dot to get the zone name.
		err := r.awsClient.DeleteARecord(
			r.route53.publicHostedZoneName,
			r.route53.loadBalancerDNSName,
			r.route53.loadBalancerHostedZoneId,
			r.route53.resourceRecordSetName,
			false)
		if err != nil {
			log.Info("Couldn't delete a DNS record for public zone: " + err.Error())
			if i == config.MaxAPIRetries {
				log.Info("Out of retries for public zone")
				return err
			}
			log.Info(fmt.Sprintf("Sleeping %d seconds before retrying...", i))
			time.Sleep(time.Duration(i) * time.Second)
		} else {
			break
		}
	}

	return nil
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
