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

package workflow

import (
	"strings"

	"github.com/carv-ics-forth/frisbee/api/v1alpha1"
	"github.com/carv-ics-forth/frisbee/controllers/telemetry/grafana"
	"github.com/carv-ics-forth/frisbee/controllers/utils/expressions"
	"github.com/carv-ics-forth/frisbee/controllers/utils/lifecycle"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/validation"
)

// ValidateDAG validates the execution workflow.
// 1. Ensures that action names are qualified (since they are used as generators to jobs)
// 2. Ensures that there are no two actions with the same name.
// 3. Ensure that dependencies point to a valid action.
// 4. Ensure that macros point to a valid action.
func ValidateDAG(list []v1alpha1.Action, state lifecycle.ClassifierReader) error {
	index := make(map[string]*v1alpha1.Action)

	// prepare a dependency graph
	for i, action := range list {
		if errs := validation.IsQualifiedName(action.Name); len(errs) != 0 {
			err := errors.New(strings.Join(errs, "; "))

			return errors.Wrapf(err, "invalid actioname %s", action.Name)
		}

		index[action.Name] = &list[i]
	}

	successOK := func(deps *v1alpha1.WaitSpec) bool {
		for _, dep := range deps.Success {
			_, ok := index[dep]
			if !ok {
				return false
			}
		}

		return true
	}

	runningOK := func(deps *v1alpha1.WaitSpec) bool {
		for _, dep := range deps.Running {
			_, ok := index[dep]
			if !ok {
				return false
			}
		}

		return true
	}

	// validate dependencies and assertions
	for _, action := range list {
		if deps := action.DependsOn; deps != nil {
			if !successOK(deps) || !runningOK(deps) {
				return errors.Errorf("invalid dependency. action [%s] depends on [%s]", action.Name, deps)
			}
		}

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

		if action.ActionType == "Delete" {
			for _, job := range action.Delete.Jobs {
				target, exists := index[job]
				if !exists {
					return errors.Errorf("job [%s] of action [%s] does not exist", job, action.Name)
				}

				if target.ActionType == "Delete" {
					return errors.Errorf("cycle deletion. job [%s] of action [%s] is a deletion job", job, action.Name)
				}
			}
		}

	}

	// TODO:
	// 1) add validation for templateRef
	// 2) make validation as webhook so to validate the experiment before it begins.

	return nil
}

/*
func GetPotentialFaults(list v1alpha1.ActionList) {
	for _, action := range list {
		action.Service
	}

}

*/
