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

package telemetry

import (
	"fmt"

	"github.com/carv-ics-forth/frisbee/api/v1alpha1"
	"github.com/carv-ics-forth/frisbee/controllers/utils/lifecycle"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var telemetryServices = []string{"prometheus", "grafana"}

type test struct {
	expression bool
	lifecycle  v1alpha1.Lifecycle
	condition  metav1.Condition
}

func calculateLifecycle(t *v1alpha1.Telemetry, gs lifecycle.ClassifierReader) v1alpha1.Lifecycle {
	cycle := t.Status.Lifecycle

	// Skip any CR which are already completed, or uninitialized.
	if cycle.Phase.Is(v1alpha1.PhaseUninitialized, v1alpha1.PhaseSuccess, v1alpha1.PhaseFailed) {
		return cycle
	}

	expectedJobs := len(telemetryServices)

	autotests := []test{
		{ // A job has failed during execution.
			expression: gs.NumFailedJobs() > 0,
			lifecycle: v1alpha1.Lifecycle{
				Phase:   v1alpha1.PhaseFailed,
				Reason:  "JobHasFailed",
				Message: fmt.Sprintf("failed jobs: %s", gs.FailedList()),
			},
			condition: metav1.Condition{
				Type:    v1alpha1.ConditionJobFailed.String(),
				Status:  metav1.ConditionTrue,
				Reason:  "JobHasFailed",
				Message: fmt.Sprintf("failed jobs: %s", gs.FailedList()),
			},
		},
		{ // All jobs are running
			expression: gs.NumRunningJobs() == expectedJobs,
			lifecycle: v1alpha1.Lifecycle{
				Phase:   v1alpha1.PhaseRunning,
				Reason:  "JobIsRunning",
				Message: fmt.Sprintf("running jobs: %s", gs.RunningList()),
			},
			condition: metav1.Condition{
				Type:    v1alpha1.ConditionAllJobsScheduled.String(),
				Status:  metav1.ConditionTrue,
				Reason:  "AllJobsRunning",
				Message: fmt.Sprintf("active jobs: %s", gs.PendingList()),
			},
		},
		{ // Not all Jobs are yet created
			expression: cycle.Phase == v1alpha1.PhasePending,
			lifecycle: v1alpha1.Lifecycle{
				Phase:   v1alpha1.PhasePending,
				Reason:  "JobIsPending",
				Message: "at least one jobs has not yet created",
			},
		},
	}

	for _, testcase := range autotests {
		if testcase.expression {
			cycle = testcase.lifecycle

			return cycle
		}
	}

	logrus.Warn("TestPlan Debug info \n",
		" current ", cycle.Phase,
		" expected: ", expectedJobs,
		" activeJobs: ", gs.PendingList(),
		" runningJobs: ", gs.RunningList(),
		" successfulJobs: ", gs.SuccessfulList(),
		" failedJobs: ", gs.FailedList(),
		" cur status: ", cycle,
	)

	panic("unhandled lifecycle conditions")

}
