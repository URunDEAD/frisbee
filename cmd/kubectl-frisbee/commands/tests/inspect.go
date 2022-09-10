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
	"os"

	"github.com/carv-ics-forth/frisbee/cmd/kubectl-frisbee/commands/common"
	"github.com/carv-ics-forth/frisbee/cmd/kubectl-frisbee/env"
	"github.com/carv-ics-forth/frisbee/pkg/ui"
	"github.com/spf13/cobra"
)

type InspectOptions struct {
	Overview, Events, ExternalResources, Charts bool
	All                                         bool
	Shell                                       string

	Logs     []string
	Loglines int
}

func PopulateInspectFlags(cmd *cobra.Command, options *InspectOptions) {
	cmd.Flags().BoolVar(&options.Overview, "overview", true, "show test overview")
	cmd.Flags().BoolVar(&options.ExternalResources, "all-resources", false, "list Chaos and K8s resources")
	cmd.Flags().BoolVar(&options.Events, "events", false, "show events hinting what's happening")
	cmd.Flags().BoolVar(&options.Charts, "charts", false, "show installed templates from dependent Helm charts")

	cmd.Flags().BoolVar(&options.All, "all", false, "enable all features (except for shell)")
	cmd.Flags().StringVar(&options.Shell, "shell", "", "opens a shell to a running container")

	cmd.Flags().StringSliceVarP(&options.Logs, "logs", "l", nil, "show logs output from executor pod (if unsure, use 'all')")
	cmd.Flags().IntVar(&options.Loglines, "log-lines", 5, "Lines of recent log file to display.")
}

func NewInspectTestCmd() *cobra.Command {
	var options InspectOptions

	cmd := &cobra.Command{
		Use:     "test <testName> [--interactive podName [-- ShellArgs]]",
		Aliases: []string{"tests", "t"},
		Short:   "Get all available test information",
		Long:    "Gets test execution details, until it's in success/error state, blocks until gets complete state",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				ui.Failf("Please Pass Test name as argument")
			}

			if options.Logs != nil {
				options.Overview = false
			}

			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			client := env.Settings.GetFrisbeeClient()

			testName := args[0]

			// Interactive is exclusive
			if options.Shell != "" {
				ui.NL()

				err := common.OpenShell(testName, options.Shell, args[1:]...)
				ui.ExitOnError("Opening Shell", err)

				return
			}

			// Always-on functions

			if options.Overview || options.All {
				test, err := client.GetScenario(cmd.Context(), testName)
				ui.ExitOnError("Getting Test Information", err)

				if test != nil {
					ui.NL()
					err = common.RenderList(test, os.Stdout)
					ui.ExitOnError("== Scenario Overview ==", err)

					ui.NL()
					err = common.RenderList(&test.Status, os.Stdout)
					ui.ExitOnError("== Scenario Status ==", err)
				}

				ui.Success("== Scenario Information ==")

				{ // Action Information
					ui.NL()
					err = common.GetFrisbeeResources(testName, false)
					env.Settings.Hint("For more Frisbee Resource information use:",
						"kubectl describe <kind>.frisbee.dev [names...] -n", testName)

					ui.ExitOnError("== Scenario Actions ==", err)
				}

				{ // Virtual Objects
					ui.NL()
					vObjList, err := client.ListVirtualObjects(cmd.Context(), testName)
					ui.ExitOnError("Getting list of virtual objects", err)

					err = common.RenderList(&vObjList, os.Stdout)
					ui.ExitOnError("Rendering virtual object list", err)
				}

				ui.Success("== Action Information ==")

				{ // Visualization Tools
					ui.NL()
					err = common.Dashboards(testName)
					ui.ExitOnError("== Visualization Tools ==", err)

					ui.Success("== Visualization Tools ==")
				}
			}

			if options.ExternalResources || options.All {
				ui.NL()
				err := common.GetChaosResources(testName)

				env.Settings.Hint("For more Chaos Resource information use:",
					"kubectl describe <kind>.chaos-mesh.org [names...] -n", testName)
				ui.ExitOnError("== Active Chaos Resources ==", err)

				ui.Success("== Chaos Resources ==")

				ui.NL()
				err = common.GetK8sResources(testName)

				env.Settings.Hint("For more K8s Resource information use:",
					"kubectl describe <kind> [names...] -n", testName)
				ui.ExitOnError("== Active K8s Resources ==", err)

				ui.Success("== Kubernetes Resources ==")
			}

			if options.Charts || options.All {
				ui.NL()
				err := common.GetTemplateResources(testName)

				env.Settings.Hint("For more Template info use:",
					"kubectl describe templates -n", testName, "[template...]")
				ui.ExitOnError("== Frisbee Templates ==", err)

				ui.Success("== Scenario Templates ==")

				/*
					ui.NL()
					err = common.ListHelm(cmd, testName)
					ui.ExitOnError("== Helm Charts ==", err)
					ui.Success("For more Helm info use:", "helm list -a -n", testName)
				*/
			}

			if options.Events || options.All {
				ui.NL()
				err := common.GetK8sEvents(testName)

				env.Settings.Hint("For more events use:", "kubectl get events -n", testName)
				ui.ExitOnError("== Events ==", err)

				ui.Success("== Scenario Events ==")

			}

			if options.Logs != nil || options.All {

				ui.NL()
				err := common.GetPodLogs(testName, false, options.Loglines, options.Logs...)

				env.Settings.Hint("For more logs use:", "kubectl logs -n", testName, "<podnames>")
				ui.ExitOnError("== Logs From Pods ==", err)

				ui.Success("== Scenario Logs ==")
			}
		},
	}

	PopulateInspectFlags(cmd, &options)

	return cmd
}
