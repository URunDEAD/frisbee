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
	"github.com/fnikolai/frisbee/controllers/template/helpers"
	"github.com/fnikolai/frisbee/controllers/utils"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
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
		### 1: Load CR by name.
	*/
	var t v1alpha1.Template

	var requeue bool
	result, err := utils.Reconcile(ctx, r, req, &t, &requeue)

	if requeue {
		return result, errors.Wrapf(err, "initialization error")
	}

	if t.Status.Lifecycle.Phase == v1alpha1.PhaseRunning {
		return utils.Stop()
	}

	/*
		### 2:  Initialize the CR
	*/
	if !t.Status.IsRegistered {
		// validate services
		for name, spec := range t.Spec.Services {
			if _, err := thelpers.GenerateSpecFromScheme(spec.DeepCopy()); err != nil {
				return utils.Failed(ctx, r, &t, errors.Wrapf(err, "service template %s error", name))
			}
		}

		// validate monitors
		for name, spec := range t.Spec.Monitors {
			if _, err := thelpers.GenerateMonitorSpec(spec.DeepCopy()); err != nil {
				return utils.Failed(ctx, r, &t, errors.Wrapf(err, "monitor template %s error", name))
			}
		}

		r.Logger.Info("Import Template",
			"name", req.NamespacedName,
			"services", GetServiceNames(t.Spec),
			"monitor", GetMonitorNames(t.Spec),
		)
	}

	t.Status.IsRegistered = true

	return utils.Running(ctx, r, &t, "all templates are loaded")
}

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

func GetServiceNames(t v1alpha1.TemplateSpec) []string {
	names := make([]string, 0, len(t.Services))

	for name := range t.Services {
		names = append(names, name)
	}

	return names
}

func GetMonitorNames(t v1alpha1.TemplateSpec) []string {
	names := make([]string, 0, len(t.Monitors))

	for name := range t.Monitors {
		names = append(names, name)
	}

	return names
}

func NewController(mgr ctrl.Manager, logger logr.Logger) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Template{}).
		Named("template").
		Complete(&Controller{
			Manager: mgr,
			Logger:  logger.WithName("template"),
		})
}
