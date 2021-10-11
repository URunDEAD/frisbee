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

package workflow

import (
	"context"

	"github.com/fnikolai/frisbee/api/v1alpha1"
	"github.com/fnikolai/frisbee/controllers/utils"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func (r *Controller) runJob(ctx context.Context, w *v1alpha1.Workflow, action v1alpha1.Action) error {
	logrus.Warn("Handle job ", action.Name)

	switch action.ActionType {
	case "Service":
		return r.service(ctx, w, action)

	case "Cluster":
		return r.cluster(ctx, w, action)

	case "Chaos":
		return r.chaos(ctx, w, action)

	default:
		return errors.Errorf("unknown action %s", action.ActionType)
	}
}

func (r *Controller) service(ctx context.Context, w *v1alpha1.Workflow, action v1alpha1.Action) error {
	var service v1alpha1.Service

	utils.SetOwner(r, w, &service)
	service.SetName(action.Name)

	action.Service.DeepCopyInto(&service.Spec)

	if err := utils.CreateUnlessExists(ctx, r, &service); err != nil {
		return errors.Wrapf(err, "action %s execution failed", action.Name)
	}

	return nil
}

func (r *Controller) cluster(ctx context.Context, w *v1alpha1.Workflow, action v1alpha1.Action) error {
	var cluster v1alpha1.Cluster

	utils.SetOwner(r, w, &cluster)
	cluster.SetName(action.Name)

	action.Cluster.DeepCopyInto(&cluster.Spec)

	if err := utils.CreateUnlessExists(ctx, r, &cluster); err != nil {
		return errors.Wrapf(err, "action %s execution failed", action.Name)
	}

	return nil
}

func (r *Controller) chaos(ctx context.Context, w *v1alpha1.Workflow, action v1alpha1.Action) error {
	var chaos v1alpha1.Chaos

	switch chaos.Spec.Type {
	case v1alpha1.FaultKill:
		if action.DependsOn.Success != nil {
			return errors.Errorf("kill is a inject-only chaos. it does not have success. only running")
		}
	}

	utils.SetOwner(r, w, &chaos)
	chaos.SetName(action.Name)

	action.Chaos.DeepCopyInto(&chaos.Spec)

	if err := utils.CreateUnlessExists(ctx, r, &chaos); err != nil {
		return errors.Wrapf(err, "action %s execution failed", action.Name)
	}

	return nil
}