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

package testplan

import (
	"context"
	"strings"

	"github.com/carv-ics-forth/frisbee/api/v1alpha1"
	chaosutils "github.com/carv-ics-forth/frisbee/controllers/chaos/utils"
	"github.com/carv-ics-forth/frisbee/controllers/common/expressions"
	"github.com/carv-ics-forth/frisbee/controllers/common/grafana"
	"github.com/carv-ics-forth/frisbee/controllers/common/lifecycle"
	serviceutils "github.com/carv-ics-forth/frisbee/controllers/service/utils"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

// Validate validates the execution workflow.
// 1. Ensures that action names are qualified (since they are used as generators to jobs)
// 2. Ensures that there are no two actions with the same name.
// 3. Ensure that dependencies point to a valid action.
// 4. Ensure that macros point to a valid action.
func (r *Controller) Validate(ctx context.Context, plan *v1alpha1.TestPlan, clusterView lifecycle.ClassifierReader) error {
	callIndex, err := PrepareDependencyGraph(plan.Spec.Actions)
	if err != nil {
		return errors.Wrapf(err, "invalid plan [%s]", plan.GetName())
	}

	for actionName, action := range callIndex {
		if err := CheckDependencies(action, callIndex); err != nil {
			return errors.Wrapf(err, "dependency error for action [%s]", actionName)
		}

		if err := CheckAssertions(action, clusterView); err != nil {
			return errors.Wrapf(err, "assertion error for action [%s]", actionName)
		}

		if err := r.CheckTemplateRef(ctx, plan, action); err != nil {
			return errors.Wrapf(err, "template reference error for action [%s]", actionName)
		}

		if err := CheckJobRef(action, callIndex); err != nil {
			return errors.Wrapf(err, "job reference error for action [%s]", actionName)
		}
	}

	// TODO:
	// 2) make validation as webhook so to validate the experiment before it begins.

	return nil
}

type index map[string]*v1alpha1.Action

func PrepareDependencyGraph(actionList []v1alpha1.Action) (index, error) {
	callIndex := make(map[string]*v1alpha1.Action)

	isSupported := func(act *v1alpha1.Action) bool {
		if act == nil || act.EmbedActions == nil {
			return false
		}

		switch act.ActionType {
		case v1alpha1.ActionService:
			return act.EmbedActions.Service != nil
		case v1alpha1.ActionCluster:
			return act.EmbedActions.Cluster != nil
		case v1alpha1.ActionChaos:
			return act.EmbedActions.Chaos != nil
		case v1alpha1.ActionCascade:
			return act.EmbedActions.Cascade != nil
		case v1alpha1.ActionDelete:
			return act.EmbedActions.Delete != nil
		case v1alpha1.ActionCall:
			return act.EmbedActions.Call != nil
		}

		return false
	}

	// prepare a dependency graph
	for i, action := range actionList {
		// Ensure that the type of action is supported and is correctly set
		if !isSupported(&action) {
			return nil, errors.Errorf("incorrent spec for type [%s] of action [%s]", action.ActionType, action.Name)
		}

		// Because the action name will be the "matrix" for generating addressable jobs,
		// it must adhere to certain properties.
		if errs := validation.IsQualifiedName(action.Name); len(errs) != 0 {
			err := errors.New(strings.Join(errs, "; "))

			return nil, errors.Wrapf(err, "invalid actioname %s", action.Name)
		}

		_, ok := callIndex[action.Name]
		if ok {
			return nil, errors.Errorf("Duplicate action '%s'", action.Name)
		}

		callIndex[action.Name] = &actionList[i]
	}

	return callIndex, nil
}

func CheckDependencies(action *v1alpha1.Action, callIndex index) error {
	successOK := func(deps *v1alpha1.WaitSpec) bool {
		for _, dep := range deps.Success {
			_, ok := callIndex[dep]
			if !ok {
				return false
			}
		}

		return true
	}

	runningOK := func(deps *v1alpha1.WaitSpec) bool {
		for _, dep := range deps.Running {
			_, ok := callIndex[dep]
			if !ok {
				return false
			}
		}

		return true
	}

	if deps := action.DependsOn; deps != nil {
		if !successOK(deps) || !runningOK(deps) {
			return errors.Errorf("invalid dependency. action [%s] depends on [%s]", action.Name, deps)
		}
	}

	return nil
}

func CheckAssertions(action *v1alpha1.Action, state lifecycle.ClassifierReader) error {
	if assert := action.Assert; !assert.IsZero() {
		if action.Delete != nil {
			return errors.Errorf("Delete job cannot have assertion")
		}

		if assert.HasStateExpr() {
			_, _, err := expressions.FiredState(assert.State, state)
			if err != nil {
				return errors.Wrapf(err, "Invalid state expr for action %s", action.Name)
			}
		}

		if assert.HasMetricsExpr() {
			_, err := grafana.ParseAlertExpr(assert.Metrics)
			if err != nil {
				return errors.Wrapf(err, "Invalid metrics expr for action %s", action.Name)
			}
		}
	}

	return nil
}

func (r *Controller) CheckTemplateRef(ctx context.Context, who metav1.Object, action *v1alpha1.Action) error {
	switch action.ActionType {
	case v1alpha1.ActionService:
		if _, err := serviceutils.GetServiceSpec(ctx, r, who, *action.Service); err != nil {
			return errors.Wrapf(err, "cannot retrieve service spec")
		}
	case v1alpha1.ActionCluster:
		if _, err := serviceutils.GetServiceSpec(ctx, r, who, action.Cluster.GenerateFromTemplate); err != nil {
			return errors.Wrapf(err, "cannot retrieve cluster spec")
		}

	case v1alpha1.ActionChaos:
		if _, err := chaosutils.GetChaosSpec(ctx, r, who, *action.Chaos); err != nil {
			return errors.Wrapf(err, "cannot retrieve chaos spec")
		}

	case v1alpha1.ActionCascade:
		if _, err := chaosutils.GetChaosSpec(ctx, r, who, action.Cascade.GenerateFromTemplate); err != nil {
			return errors.Wrapf(err, "cannot retrieve cascade spec")
		}
	default:
		return nil
	}

	return nil
}

func CheckJobRef(action *v1alpha1.Action, callIndex index) error {
	switch action.ActionType {
	case v1alpha1.ActionDelete:
		// Check that references jobs exist and there are no cycle deletions
		for _, job := range action.Delete.Jobs {
			target, exists := callIndex[job]
			if !exists {
				return errors.Errorf("job [%s] of action [%s] does not exist", job, action.Name)
			}

			if target.ActionType == v1alpha1.ActionDelete {
				return errors.Errorf("cycle deletion. job [%s] of action [%s] is a deletion job", job, action.Name)
			}
		}
	}

	/*
		if spec.Type == v1alpha1.FaultKill {
			if action.DependsOn.Success != nil {
				return nil, errors.Errorf("kill is a inject-only chaos. it does not have success. only running")
			}
		}

	*/

	return nil
}