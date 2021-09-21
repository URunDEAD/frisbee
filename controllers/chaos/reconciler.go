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

	"github.com/fnikolai/frisbee/api/v1alpha1"
	"github.com/fnikolai/frisbee/controllers/common"
	"github.com/fnikolai/frisbee/controllers/common/lifecycle"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:rbac:groups=frisbee.io,resources=chaoss,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=frisbee.io,resources=chaoss/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=frisbee.io,resources=chaoss/finalizers,verbs=update

func NewController(mgr ctrl.Manager, logger logr.Logger) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Chaos{}).
		Named("chaos").
		Complete(&Reconciler{
			Manager: mgr,
			Logger:  logger.WithName("chaos"),
		})
}

// Reconciler reconciles a Reference object
type Reconciler struct {
	ctrl.Manager
	logr.Logger
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var obj v1alpha1.Chaos

	var ret bool
	result, err := common.Reconcile(ctx, r, req, &obj, &ret)
	if ret {
		return result, err
	}

	r.Logger.Info("-> Reconcile", "kind", reflect.TypeOf(obj), "name", obj.GetName(), "lifecycle", obj.Status.Phase)
	defer func() {
		r.Logger.Info("<- Reconcile", "kind", reflect.TypeOf(obj), "name", obj.GetName(), "lifecycle", obj.Status.Phase)
	}()

	handler := r.dispatch(obj.Spec.Type)

	// Here goes the actual reconcile logic
	switch obj.Status.Phase {
	case v1alpha1.PhaseUninitialized:
		return lifecycle.Pending(ctx, &obj, "received chaos request")

	case v1alpha1.PhasePending:
		if err := handler.Inject(ctx, &obj); err != nil {
			return lifecycle.Failed(ctx, &obj, errors.Wrapf(err, "injection failed"))
		}

		return common.Stop()

	case v1alpha1.PhaseRunning:
		if err := handler.WaitForDuration(ctx, &obj); err != nil {
			return lifecycle.Failed(ctx, &obj, errors.Wrapf(err, "chaos failed"))
		}

		return lifecycle.Success(ctx, &obj, "chaos revoked")

	case v1alpha1.PhaseSuccess:
		r.Logger.Info("Chaos completed", "name", obj.GetName())

		if err := handler.Revoke(ctx, &obj); err != nil {
			return lifecycle.Failed(ctx, &obj, errors.Wrapf(err, "unable to revoke chaos"))
		}

		return common.Stop()

	case v1alpha1.PhaseFailed:
		r.Logger.Info("Chaos failed", "name", obj.GetName())

		return common.Stop()

	case v1alpha1.PhaseChaos:
		// These phases should not happen in the workflow
		panic(errors.Errorf("invalid lifecycle phase %s", obj.Status.Phase))

	default:
		panic(errors.Errorf("unknown lifecycle phase: %s", obj.Status.Phase))
	}
}

func (r *Reconciler) Finalizer() string {
	return "chaoss.frisbee.io/finalizer"
}

func (r *Reconciler) Finalize(obj client.Object) error {
	r.Logger.Info("Finalize", "kind", reflect.TypeOf(obj), "name", obj.GetName())

	return nil
}

type chaoHandler interface {
	Inject(ctx context.Context, obj *v1alpha1.Chaos) error
	WaitForDuration(ctx context.Context, obj *v1alpha1.Chaos) error
	Revoke(ctx context.Context, obj *v1alpha1.Chaos) error
}

func (r *Reconciler) dispatch(faultType v1alpha1.FaultType) chaoHandler {
	switch faultType {
	case v1alpha1.FaultPartition:
		return &partition{r: r}

	default:
		panic("should never happen")
	}
}
