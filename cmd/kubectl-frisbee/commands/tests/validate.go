/*
Copyright 2022-2023 ICS-FORTH.

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
	"os"
	"path/filepath"

	"github.com/carv-ics-forth/frisbee/cmd/kubectl-frisbee/commands/common"
	"github.com/kubeshop/testkube/pkg/ui"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func NewValidateTestCmd() *cobra.Command {
	var options SubmitTestCmdOptions

	cmd := &cobra.Command{
		Use:     "test <Scenario>",
		Aliases: []string{"tests", "t"},
		Short:   "Validate a new test",
		Long:    `Validate run the scenario in a dry-run mode`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				ui.Failf("Pass Scenario File or Scenario Dir")
			}

			return nil
		},

		Run: func(cmd *cobra.Command, args []string) {
			testFile := args[0]

			file, err := os.Open(testFile)
			ui.ExitOnError("failed to open path", err)

			fileInfo, err := file.Stat()
			ui.ExitOnError("failed to extract scenarios info", err)

			if fileInfo.IsDir() {
				if filepath.Base(testFile) == "platform" {
					ui.Failf("due to Helm constraints Frisbee cannot be self-validated ")
				}

				if filepath.Base(testFile) == "examples" {
					// examples are validated by the admission controller of Frisbee
					err := validateExamples(testFile)

					ui.ExitOnError("Scenario Validation ...", err)

					ui.Success("All Scenario Files have been successfully validated.", testFile)

					return
				}

				// Helm charts (and therefore templates) are validated by Helm
				if _, err := os.Stat(testFile + "/Chart.yaml"); err == nil {
					err = validateChart(testFile)

					ui.ExitOnError("Chart Validation ...", err)

					ui.Success("Chart validated.", testFile)

					return
				}

				ui.Failf("Validation path should point to a Helm Chart or to an Examples directory.")
			} else {
				err := validateScenario(testFile)
				ui.ExitOnError("Validating ...", err)

				ui.Success("Scenario validated:", testFile)
			}
		},
	}

	SubmitTestCmdFlags(cmd, &options)

	return cmd
}

func validateScenario(filepath string) error {
	ui.Info("Validating Scenarios ... ", filepath)

	return common.RunTest("", filepath, common.ValidationServer)
}

func validateExamples(examplesDir string) error {
	return filepath.Walk(examplesDir, func(path string, info os.FileInfo, err error) error {
		fileExtension := filepath.Ext(path)

		// Kubernetes' files are expected to be either in .yml or .yaml format. Anything else is ignored.
		if fileExtension == ".yml" || fileExtension == ".yaml" {
			if err := validateScenario(path); err != nil {
				return errors.Wrap(err, path)
			}
		}

		ui.Debug("Ignore file", path)

		return nil
	})
}

func validateChart(chartDir string) error {
	// Helm by default will validate only the chart/templates directory
	ui.Info("Validating Templates ... ", chartDir)

	if _, err := common.Helm("", "install", "dummy", "--dry-run", chartDir); err != nil {
		return err
	}

	// Then, we also need to validate the chart/examples directory
	return validateExamples(chartDir + "/examples")
}
