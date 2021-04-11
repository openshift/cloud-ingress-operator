package routerservice

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_router_service")

const (
	RouterServiceNamespace = "openshift-ingress"
	ELBAnnotationKey       = "service.beta.kubernetes.io/aws-load-balancer-connection-idle-timeout"
	ELBAnnotationValue     = "1800"
)

// Add creates a new Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileRouterService{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("router-service-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Only filter on services in the openshift-ingress namespace and create/update events
	p := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return e.Meta.GetNamespace() == RouterServiceNamespace
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return e.MetaNew.GetNamespace() == RouterServiceNamespace
		},
	}

	// Watch for changes to primary resource Service
	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForObject{}, p)
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileRouterService implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileRouterService{}

// ReconcileRouterService reconciles a Service object
type ReconcileRouterService struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a RouterService object and makes changes based on the state read
// and what is in the Service.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileRouterService) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	// Fetch the Service
	svc := &corev1.Service{}
	err := r.client.Get(context.TODO(), request.NamespacedName, svc)
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

	// Only check LoadBalancer service types for annotations
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		if !metav1.HasAnnotation(svc.ObjectMeta, ELBAnnotationKey) ||
			svc.ObjectMeta.Annotations[ELBAnnotationKey] != ELBAnnotationValue {
			reqLogger.Info("Updating annotation for " + svc.Name)
			metav1.SetMetaDataAnnotation(&svc.ObjectMeta, ELBAnnotationKey, ELBAnnotationValue)
			err = r.client.Update(context.TODO(), svc)
			if err != nil {
				reqLogger.Error(err, "Error updating service annotation")
				return reconcile.Result{}, err
			}
		} else {
			reqLogger.Info("skipping service " + svc.Name + " w/ proper annotations")
		}
	}

	return reconcile.Result{}, nil
}
