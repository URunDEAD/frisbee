/*
Copyright 2023 ICS-FORTH.

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

package completion

const completionDesc = `
Generate autocompletion scripts for Frisbee for the specified shell.
`

const bashCompDesc = `
Generate the autocompletion script for Frisbee for the bash shell.
To load completions in your current shell session:
    source <(frisbee completion bash)
To load completions for every new session, execute once:
- Linux:
      frisbee completion bash > /etc/bash_completion.d/frisbee.bash
- MacOS:
      frisbee completion bash > /usr/local/etc/bash_completion.d/helm.bash
`
