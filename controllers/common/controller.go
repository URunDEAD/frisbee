/*
Copyright 2021 ICS-FORTH.

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

package common

import (
	"context"
	"reflect"
	"time"

	"github.com/carv-ics-forth/frisbee/api/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	k8errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func Stop() (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

// RequeueAfter will place the request in a queue, but it will be dequeue after the specified period.
func RequeueAfter(delay time.Duration) (ctrl.Result, error) {
	return ctrl.Result{RequeueAfter: delay, Requeue: true}, nil
}

// Requeue will place the request in a queue, and will be immediately dequeued.
func Requeue() (ctrl.Result, error) {
	return ctrl.Result{Requeue: true}, nil
}

// RequeueWithError will place the request in a queue, and will be immediately dequeued.
// State dequeuing the request, the controller will report the error.
func RequeueWithError(err error) (ctrl.Result, error) {
	return ctrl.Result{}, err
}

// Reconciler implements basic functionality that is common to every solid reconciler (e.g, finalizers)
type Reconciler interface {
	GetClient() client.Client
	GetCache() cache.Cache

	GetEventRecorderFor(name string) record.EventRecorder

	// Logging
	Error(err error, msg string, keysAndValues ...interface{})
	Info(msg string, keysAndValues ...interface{})
	V(level int) logr.Logger

	Finalizer() string

	// Finalize deletes any external resources associated with the service
	// Examples finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a EphemeralVolume.
	//
	// Ensure that delete implementation is idempotent and safe to invoke
	// multiple times for same object
	Finalize(object client.Object) error
}

// Reconcile provides the most common functions for all the Reconcilers. That includes acquisition of the CR object
//
//	and management of the CR (Custom Resource) finalizers.
//
// Bool indicate whether the caller should return immediately (true) or continue (false).
// The reconciliation cycle is where the framework gives us back control after a watch has passed up an event.
func Reconcile(ctx context.Context, r Reconciler, req ctrl.Request, obj client.Object, requeue *bool) (ctrl.Result, error) {

	// make the calling controller to return
	*requeue = true

	/*
		### 1: Retrieve the CR by name
	*/
	if err := r.GetClient().Get(ctx, req.NamespacedName, obj); err != nil {
		// Request object not found, could have been deleted after reconcile request.
		// We'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on added / deleted requests.
		if k8errors.IsNotFound(err) {
			return Stop()
		}

		return RequeueWithError(err)
	}

	/*
		### 2: Manage Resource initialization
		Finalizers provide a mechanism to inform the Kubernetes control plane that an action needs to take place
		before the standard Kubernetes garbage collection logic can be performed.
	*/

	if obj.GetDeletionTimestamp().IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and Update the object. This is equivalent
		// registering our finalizer.
		if controllerutil.AddFinalizer(obj, r.Finalizer()) {
			r.Info("AddFinalizer",
				"obj", client.ObjectKeyFromObject(obj),
				"finalizer", r.Finalizer())

			if err := Update(ctx, r, obj); err != nil {
				return RequeueWithError(err)
			}
			return Stop()
		}

	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(obj, r.Finalizer()) {
			// our finalizer is present, so lets handle any external dependency.
			if err := r.Finalize(obj); err != nil {
				// Run finalization logic to remove external dependencies.
				// If the finalization logic fails, don't remove the finalizer
				// so that we can retry during the next reconciliation.
				r.Error(err, "Finalize error",
					"obj", client.ObjectKeyFromObject(obj),
					"finalizer", r.Finalizer())

				return RequeueWithError(err)
			}

			// Once all finalizers have been removed, the object will be deleted.
			if controllerutil.RemoveFinalizer(obj, r.Finalizer()) {
				r.Info("RemoveFinalizer",
					"obj", client.ObjectKeyFromObject(obj),
					"finalizer", r.Finalizer())

				if err := Update(ctx, r, obj); err != nil {
					return RequeueAfter(time.Second)
				}

				return Stop()
			}
		}
	}

	*requeue = false

	return Stop()
}

// Update will update the metadata and the spec of the Object. If there is a conflict, it will retry again.
func Update(ctx context.Context, r Reconciler, obj client.Object) error {

	r.Info("OO UpdtMeta",
		"obj", client.ObjectKeyFromObject(obj),
		"version", obj.GetResourceVersion(),
	)

	return r.GetClient().Update(ctx, obj)
}

// UpdateStatus will update the status of the Object. If there is a conflict, it will retry again.
func UpdateStatus(ctx context.Context, r Reconciler, obj client.Object) error {
	r.Info("OO UpdtStatus",
		"obj", client.ObjectKeyFromObject(obj),
		"version", obj.GetResourceVersion(),
		"become", obj.(v1alpha1.ReconcileStatusAware).GetReconcileStatus().Phase,
	)

	return r.GetClient().Status().Update(ctx, obj)
}

// Create ignores existing objects.
// if the next reconciliation cycle happens faster than the API update, it is possible to
// reschedule the creation of a Job. To avoid that, get if the Job is already submitted.
func Create(ctx context.Context, r Reconciler, parent, child client.Object) error {
	// owner labels are used by the selectors.
	// workflow labels are used to select only objects that belong to this experiment.
	// used to narrow down the scope of fault injection in a common namespace
	v1alpha1.SetCreatedByLabel(child, parent)
	v1alpha1.SetInstanceLabel(child)

	child.SetNamespace(parent.GetNamespace())

	// SetControllerReference sets owner as a Controller OwnerReference on controlled.
	// This is used for garbage collection of the controlled object and for
	// reconciling the owner object on changes to controlled (with a Logs + EnqueueRequestForOwner).
	// Since only one OwnerReference can be a controller, it returns an error if
	// there is another OwnerReference with Controller flag set.
	if err := controllerutil.SetControllerReference(parent, child, r.GetClient().Scheme()); err != nil {
		return errors.Wrapf(err, "set controller reference")
	}

	r.Info("++ Create",
		"kind", reflect.TypeOf(child),
		"obj", client.ObjectKeyFromObject(child),
	)

	err := r.GetClient().Create(ctx, child)
	switch {
	case k8errors.IsAlreadyExists(err):
		panic(err) // This should never happen under normal conditions
	case err != nil:
		return errors.Wrapf(err, "creation error")
	default:
		return nil
	}
}

func ListChildren(ctx context.Context, r Reconciler, childJobs client.ObjectList, req types.NamespacedName) error {
	filters := []client.ListOption{
		client.InNamespace(req.Namespace),
		client.MatchingLabels{v1alpha1.LabelCreatedBy: req.Name},
	}

	if err := r.GetClient().List(ctx, childJobs, filters...); err != nil {
		return errors.Wrapf(err, "cannot list children")
	}

	return nil
}

// Delete removes a Kubernetes object, ignoring the NotFound error. If any error exists,
// it is recorded in the reconciler's logger.
func Delete(ctx context.Context, r Reconciler, obj client.Object) {
	r.Info("-- Delete",
		"kind", reflect.TypeOf(obj),
		"obj", client.ObjectKeyFromObject(obj),
		"version", obj.GetResourceVersion(),
	)

	// propagation := metav1.DeletePropagationForeground
	propagation := metav1.DeletePropagationBackground
	options := client.DeleteOptions{
		PropagationPolicy: &propagation,
	}

	err := r.GetClient().Delete(ctx, obj, &options)
	switch {
	case k8errors.IsNotFound(err):
	// Ignore
	case err != nil:
		r.Error(err, "deletion error", "obj", client.ObjectKeyFromObject(obj))
	default:
		return
	}
}

// IsManagedByThisController returns true if the object is managed by the specified controller.
// If it is managed by another controller, or no controller is being resolved, it returns false.
func IsManagedByThisController(obj metav1.Object, controller schema.GroupVersionKind) bool {
	owner := metav1.GetControllerOf(obj)
	if owner == nil {
		return false
	}

	if owner.APIVersion != controller.GroupVersion().String() ||
		owner.Kind != controller.Kind {
		return false
	}

	return true
}
