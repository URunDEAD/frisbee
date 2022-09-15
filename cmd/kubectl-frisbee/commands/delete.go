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

package commands

import (
	"github.com/carv-ics-forth/frisbee/cmd/kubectl-frisbee/commands/common"
	"github.com/carv-ics-forth/frisbee/cmd/kubectl-frisbee/commands/tests"
	"github.com/carv-ics-forth/frisbee/cmd/kubectl-frisbee/env"
	"github.com/carv-ics-forth/frisbee/pkg/ui"
	"github.com/spf13/cobra"
)

func NewDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <resourceName>",
		Aliases: []string{"remove"},
		Short:   "Delete resources",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			ui.Logo()
			ui.SetVerbose(env.Settings.Debug)

			env.Settings.CheckKubePerms()

			ui.Info("Using config:", env.Settings.KubeConfig)

			if !common.CRDsExist(common.Scenarios) {
				ui.Failf("Frisbee is not installed on the kubernetes cluster.")
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			ui.PrintOnError("Displaying help", cmd.Help())
		},
	}

	cmd.AddCommand(tests.NewDeleteTestsCmd())

	return cmd
}
