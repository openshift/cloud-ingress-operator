/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package routerservice

import (
	"context"

	baseutils "github.com/openshift/cloud-ingress-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logf.Log.WithName("controller_router_service")

const (
	RouterServiceNamespace = "openshift-ingress"
	ELBAnnotationKey       = "service.beta.kubernetes.io/aws-load-balancer-connection-idle-timeout"
	ELBAnnotationValue     = "1800"
)

// RouterServiceReconciler reconciles a RouterService object
type RouterServiceReconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the RouterService object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.2/pkg/reconcile

// Reconcile reads that state of the cluster for a RouterService object and makes changes based on the state read
// and what is in the Service.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *RouterServiceReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	// Fetch the Service
	svc := &corev1.Service{}
	err := r.Client.Get(context.TODO(), request.NamespacedName, svc)
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
	// Only set timeout annotations on services for < OCP 4.11. In 4.11+, the cluster-ingress-operator maintains this annotation
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer && !baseutils.IsVersionHigherThan("4.11") {
		if !metav1.HasAnnotation(svc.ObjectMeta, ELBAnnotationKey) ||
			svc.ObjectMeta.Annotations[ELBAnnotationKey] != ELBAnnotationValue {
			reqLogger.Info("Updating annotation for " + svc.Name)
			metav1.SetMetaDataAnnotation(&svc.ObjectMeta, ELBAnnotationKey, ELBAnnotationValue)
			err = r.Client.Update(context.TODO(), svc)
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

// Only filter on services in the openshift-ingress namespace and create/update events
func eventPredicates() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return e.Object.GetNamespace() == RouterServiceNamespace

		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return e.ObjectNew.GetNamespace() == RouterServiceNamespace
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *RouterServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		WithEventFilter(eventPredicates()).
		Complete(r)
}
