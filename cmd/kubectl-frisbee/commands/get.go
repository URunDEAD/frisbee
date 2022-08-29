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

package commands

import (
	"github.com/carv-ics-forth/frisbee/cmd/kubectl-frisbee/commands/tests"
	"github.com/carv-ics-forth/frisbee/pkg/ui"
	"github.com/spf13/cobra"
)

func NewGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <resourceName>",
		Aliases: []string{"g"},
		Short:   "Get resources",
		Long:    `Get available resources, get single item or list`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			ui.SetVerbose(verbose)
		},
		Run: func(cmd *cobra.Command, args []string) {
			ui.Logo()

			err := cmd.Help()
			ui.PrintOnError("Displaying help", err)
		},
	}

	cmd.AddCommand(tests.NewGetTestsCmd())
	// cmd.AddCommand(testsuites.NewRunTestSuiteCmd())

	cmd.PersistentFlags().StringP("output", "o", "pretty", "output type can be one of json|yaml|pretty|go-template")
	cmd.PersistentFlags().StringP("go-template", "", "{{.}}", "go template to render")

	return cmd
}
