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

package template

import (
	"context"
	"reflect"

	"github.com/fnikolai/frisbee/api/v1alpha1"
	thelpers "github.com/fnikolai/frisbee/controllers/template/helpers"
	"github.com/fnikolai/frisbee/controllers/utils"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Controller reconciles a Templates object
type Controller struct {
	ctrl.Manager
	logr.Logger
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	/*
		1: Load CR by name.
		------------------------------------------------------------------
	*/
	var cr v1alpha1.Template

	var requeue bool
	result, err := utils.Reconcile(ctx, r, req, &cr, &requeue)

	if requeue {
		return result, errors.Wrapf(err, "initialization error")
	}

	/*
		2: Update the CR status using the data we've gathered
		------------------------------------------------------------------
	*/
	if err := utils.UpdateStatus(ctx, r, &cr); err != nil {
		runtime.HandleError(err)

		return utils.Requeue()
	}

	/*
		3: Clean up the controller from finished jobs
		------------------------------------------------------------------

		Not needed now.
	*/

	/*
		4: Make the world matching what we want in our spec
		------------------------------------------------------------------
	*/
	if cr.Status.Lifecycle.Phase == v1alpha1.PhaseRunning {
		return utils.Stop()
	}

	if cr.Status.Lifecycle.Phase == v1alpha1.PhaseUninitialized {
		// validate services
		for name, scheme := range cr.Spec.Entries {
			specStr, err := thelpers.GenerateSpecFromScheme(scheme.DeepCopy())
			if err != nil {
				return utils.Failed(ctx, r, &cr, errors.Wrapf(err, "template %s error", name))
			}

			sSpec := v1alpha1.ServiceSpec{}

			if err := yaml.Unmarshal([]byte(specStr), &sSpec); err != nil {
				// if it is not a service, it may be a monitor
				mSpec := v1alpha1.MonitorSpec{}
				if err := yaml.Unmarshal([]byte(specStr), &mSpec); err != nil {
					return utils.Failed(ctx, r, &cr, errors.Wrapf(err, "unparsable scheme for %s", name))
				}
			}
		}

		names := make([]string, 0, len(cr.Spec.Entries))

		for name := range cr.Spec.Entries {
			names = append(names, name)
		}

		r.Logger.Info("Import Template",
			"name", req.NamespacedName,
			"entries", names,
		)

		return utils.Running(ctx, r, &cr, "all templates are loaded")
	}

	return utils.Stop()
}

/*
### Finalizers
*/

func (r *Controller) Finalizer() string {
	return "templates.frisbee.io/finalizer"
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

func NewController(mgr ctrl.Manager, logger logr.Logger) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Template{}).
		Named("template").
		Complete(&Controller{
			Manager: mgr,
			Logger:  logger.WithName("template"),
		})
}