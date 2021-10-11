// Licensed to FORTH/ICS under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. FORTH/ICS licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package service

import (
	"context"
	"reflect"
	"time"

	"github.com/fnikolai/frisbee/api/v1alpha1"
	"github.com/fnikolai/frisbee/controllers/utils"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:rbac:groups=frisbee.io,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=frisbee.io,resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=frisbee.io,resources=services/finalizers,verbs=update

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;

// Controller reconciles a Service object.
type Controller struct {
	ctrl.Manager
	logr.Logger

	// annotator sends annotations to grafana
	annotator utils.Annotator
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	/*
		1: Load CR by name.
		------------------------------------------------------------------
	*/
	var cr v1alpha1.Service

	var requeue bool
	result, err := utils.Reconcile(ctx, r, req, &cr, &requeue)

	if requeue {
		return result, errors.Wrapf(err, "initialization error")
	}

	r.Logger.Info("-> Reconcile",
		"kind", reflect.TypeOf(cr),
		"name", cr.GetName(),
		"lifecycle", cr.Status.Phase,
		"version", cr.GetResourceVersion(),
	)

	defer func() {
		r.Logger.Info("<- Reconcile",
			"kind", reflect.TypeOf(cr),
			"name", cr.GetName(),
			"lifecycle", cr.Status.Phase,
			"version", cr.GetResourceVersion(),
		)
	}()

	/*
		2: Load CR's components.
		------------------------------------------------------------------

		The component is a pod with the same name as the cr.
	*/
	var pod corev1.Pod
	{
		key := client.ObjectKeyFromObject(&cr)

		if err := r.GetClient().Get(ctx, key, &pod); client.IgnoreNotFound(err) != nil {
			return utils.Failed(ctx, r, &cr, errors.Wrapf(err, "retrieve pod"))
		}
	}

	/*
		3: Update the CR status using the data we've gathered
		------------------------------------------------------------------

		The Update at this step serves two functions.
		First, it is like "journaling" for the upcoming operations.
		Second, it is a roadblock for stall (queued) requests.

		However, due to the multiple updates, it is possible for this function to
		be in conflict. We fix this issue by re-queueing the request.
		We also suppress verbose error reporting as to avoid polluting the output.
	*/
	newStatus := calculateLifecycle(&cr, &pod)
	cr.Status.Lifecycle = newStatus

	if err := utils.UpdateStatus(ctx, r, &cr); err != nil {
		runtime.HandleError(err)

		return utils.Requeue()
	}

	/*
		4: Clean up the controller from finished jobs
		------------------------------------------------------------------

		First, we'll try to clean up old jobs, so that we don't leave too many lying
		around.
	*/
	if newStatus.Phase == v1alpha1.PhaseSuccess {
		// r.GetEventRecorderFor("").Event(&cr, corev1.EventTypeNormal,
		//	newStatus.Reason, "service succeeded")
		// TODO: delete pod and service, but leave the service descriptor.
		return utils.Stop()
	}

	if newStatus.Phase == v1alpha1.PhaseFailed {
		r.GetEventRecorderFor("").Event(&cr, corev1.EventTypeWarning,
			newStatus.Reason, "service failed")

		return utils.Stop()
	}

	/*
		5: Make the world matching what we want in our spec
		------------------------------------------------------------------

		Once we've updated our status, we can move on to ensuring that the status of
		the world matches what we want in our spec.

		We may delete the service, add a pod, or wait for existing pod to change its status.
	*/
	if cr.Status.LastScheduleTime != nil {
		// next reconciliation cycle will be trigger by the watchers
		return utils.Stop()
	}

	if err := r.runJob(ctx, &cr); err != nil {
		return utils.Failed(ctx, r, &cr, errors.Wrapf(err, "cannot create pod"))
	}

	/*
		6: Avoid double actions
		------------------------------------------------------------------

		If this process restarts at this point (after posting a job, but
		before updating the status), then we might try to start the job on
		the next time.  Actually, if we re-list the Jobs on the next cycle
		we might not see our own status update, and then post one again.
		So, we need to use the job name as a lock to prevent us from making the job twice.
	*/

	// Add the just-started jobs to the status list.
	cr.Status.LastScheduleTime = &metav1.Time{Time: time.Now()}

	return utils.Pending(ctx, r, &cr, "create pod")
}

/*
### Finalizers
*/

func (r *Controller) Finalizer() string {
	return "services.frisbee.io/finalizer"
}

func (r *Controller) Finalize(obj client.Object) error {
	r.Logger.Info("XX Finalize",
		"kind", reflect.TypeOf(obj),
		"name", obj.GetName(),
		"version", obj.GetResourceVersion(),
	)

	return nil
}

/*
### Setup
	Finally, we'll update our setup.

	We'll inform the manager that this controller owns some resources, so that it
	will automatically call Reconcile on the underlying controller when a resource changes, is
	deleted, etc.
*/

var controllerKind = v1alpha1.GroupVersion.WithKind("Service")

func NewController(mgr ctrl.Manager, logger logr.Logger) error {
	r := &Controller{
		Manager:   mgr,
		Logger:    logger.WithName("service"),
		annotator: &utils.PointAnnotation{},
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named("service").
		For(&v1alpha1.Service{}).
		Owns(&corev1.Pod{}, builder.WithPredicates(r.Watchers())).
		Complete(r)
}