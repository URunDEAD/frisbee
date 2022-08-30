/*
Copyright 2022 ICS-FORTH.

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

package tests

import (
	"fmt"

	"github.com/carv-ics-forth/frisbee/api/v1alpha1"
	"github.com/carv-ics-forth/frisbee/cmd/kubectl-frisbee/commands/common"
	"github.com/carv-ics-forth/frisbee/pkg/ui"
	"github.com/spf13/cobra"
)

type TestSaveOptions struct {
	Force bool
}

func PopulateSaveTestFlags(cmd *cobra.Command, options *TestSaveOptions) {
	cmd.Flags().BoolVar(&options.Force, "force", false, "Force save test data despite test phase.")
}

const (
	Prometheus     = "prometheus"
	PrometheusData = "/prometheus/data"
	Logviewer      = "logviewer"
	TestDataPath   = "/testdata"
)

func NewSaveTestsCmd() *cobra.Command {
	var options TestSaveOptions

	cmd := &cobra.Command{
		Use:     "test <testName> <destination>",
		Aliases: []string{"tests", "t"},
		Short:   "Store locally data generated throughout the test execution",
		Long:    `Getting all available tests from given namespace - if no namespace given "frisbee" namespace is used`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				ui.Failf("Pass Test name and destination to store the data.")
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			client := common.GetClient(cmd)

			testName := args[0]
			destination := args[1]

			scenario, err := client.GetScenario(testName)
			ui.ExitOnError("Getting test information", err)

			if scenario.Spec.TestData == nil {
				ui.Failf("TestData is not enabled. Enable the TestData parameter on the scenario definition")
			}

			// Abort getting data from a non-completed test, unless --force is used
			if !scenario.Status.Phase.Is(v1alpha1.PhaseSuccess, v1alpha1.PhaseFailed) {
				if !options.Force {
					ui.Failf("Unsafe operation. The test is not completed yet. Use --force")
				}
			}

			err = common.KubectlPrint(testName, "cp", fmt.Sprintf("%s:/%s", Logviewer, TestDataPath), destination)
			ui.ExitOnError("Save test data to "+destination, err)

			promDestination := destination + "/" + Prometheus
			err = common.KubectlPrint(testName, "cp", fmt.Sprintf("%s:/%s", Prometheus, PrometheusData), promDestination)
			ui.ExitOnError("Save Prometheus data to "+promDestination, err)

			common.Hint(cmd, "To store data from a specific location use 'kubectl cp pod:path destination -n", testName)
		},
	}

	PopulateSaveTestFlags(cmd, &options)

	return cmd
}