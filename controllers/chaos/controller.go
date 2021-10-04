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

package chaos

import (
	"context"
	"reflect"
	"time"

	"github.com/fnikolai/frisbee/api/v1alpha1"
	"github.com/fnikolai/frisbee/controllers/utils"
	"github.com/go-logr/logr"
	cmap "github.com/orcaman/concurrent-map"
	"github.com/pkg/errors"
	k8errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:rbac:groups=frisbee.io,resources=chaos,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=frisbee.io,resources=chaos/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=frisbee.io,resources=chaos/finalizers,verbs=update

// +kubebuilder:rbac:groups=chaos-mesh.org,resources=networkchaos,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chaos-mesh.org,resources=networkchaos/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chaos-mesh.org,resources=networkchaos/finalizers,verbs=update

// Controller reconciles a Reference object
type Controller struct {
	ctrl.Manager
	logr.Logger

	// annotator sends annotations to grafana
	annotators cmap.ConcurrentMap
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	/*
		1: Load CR by name.
		------------------------------------------------------------------
	*/
	var cr v1alpha1.Chaos

	var requeue bool
	result, err := utils.Reconcile(ctx, r, req, &cr, &requeue)

	if requeue {
		return result, errors.Wrapf(err, "initialization error")
	}

	r.Logger.Info("-> Reconcile",
		"kind", reflect.TypeOf(cr),
		"name", cr.GetName(),
		"lifecycle", cr.Status.Phase,
		"epoch", cr.GetResourceVersion(),
	)

	defer func() {
		r.Logger.Info("<- Reconcile",
			"kind", reflect.TypeOf(cr),
			"name", cr.GetName(),
			"lifecycle", cr.Status.Phase,
			"epoch", cr.GetResourceVersion(),
		)
	}()

	/*
		2: Load CR's components.
		------------------------------------------------------------------

		Because we use the unstructured type,  Get will return an empty if there is no object. In turn, the
		client's parses will return the following error: "Object 'Kind' is missing in 'unstructured object has no kind'"
		To avoid that, we ignore errors if the map is empty -- yielding the same behavior as empty, but valid objects.
	*/
	handler := dispatch(&cr)

	fault := handler.GetFault(r)
	{
		key := client.ObjectKeyFromObject(&cr)

		err := r.GetClient().Get(ctx, key, fault)
		if err != nil && !k8errors.IsNotFound(err) {
			return utils.Failed(ctx, r, &cr, errors.Wrapf(err, "retrieve fault"))
		}
	}

	/*
		3: Update the CR status using the data we've gathered
		------------------------------------------------------------------

		Using the date we've gathered, we'll update the status of our CR.
	*/

	newStatus := calculateLifecycle(&cr, fault)
	cr.Status.Lifecycle = newStatus

	if err := utils.UpdateStatus(ctx, r, &cr); err != nil {
		// due to the multiple updates, it is possible for this function to
		// be in conflict. We fix this issue by re-queueing the request.
		// We also omit verbose error reporting as to avoid polluting the output.
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
		utils.Delete(ctx, r, fault)

		return utils.Stop()
	}

	if newStatus.Phase == v1alpha1.PhaseFailed {
		r.Logger.Error(errors.New(cr.Status.Lifecycle.Reason),
			"chaos failed",
			"chaos", cr.GetName())

		return utils.Stop()
	}

	/*
		5: Make the world matching what we want in our spec
		------------------------------------------------------------------

		Once we've updated our status, we can move on to ensuring that the status of
		the world matches what we want in our spec.

		We may delete the cr, add a pod, or wait for existing pod to change its status.
	*/
	if cr.Status.LastScheduleTime != nil {
		// next reconciliation cycle will be trigger by the watchers
		return utils.Stop()
	}

	nextFault := handler.ConstructJob(ctx, r)

	if err := utils.CreateUnlessExists(ctx, r, nextFault); err != nil {
		return utils.Failed(ctx, r, &cr, errors.Wrapf(err, "injection failed"))
	}

	// Add the just-started jobs to the status list.
	cr.Status.LastScheduleTime = &metav1.Time{Time: time.Now()}

	return utils.Pending(ctx, r, &cr, "injecting fault")
}

/*
### Finalizers
*/

func (r *Controller) Finalizer() string {
	return "chaos.frisbee.io/finalizer"
}

func (r *Controller) Finalize(obj client.Object) error {
	r.Logger.Info("XX Finalize",
		"kind", reflect.TypeOf(obj),
		"name", obj.GetName(),
		"epoch", obj.GetResourceVersion(),
	)
	return nil
}

/*
### Setup
	Finally, we'll update our setup.

	We'll inform the manager that this controller owns some Services, so that it
	will automatically call Reconcile on the underlying Service when a Pod changes, is
	deleted, etc.
*/

var controllerKind = v1alpha1.GroupVersion.WithKind("Chaos")

func NewController(mgr ctrl.Manager, logger logr.Logger) error {
	r := &Controller{
		Manager:    mgr,
		Logger:     logger.WithName("chaos"),
		annotators: cmap.New(),
	}

	var fault Fault

	AsPartition(&fault)

	return ctrl.NewControllerManagedBy(mgr).
		Named("chaos").
		For(&v1alpha1.Chaos{}).
		Owns(&fault, builder.WithPredicates(r.Watchers())).
		Complete(r)
}
