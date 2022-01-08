/*
Copyright 2022 The KubeEdge Authors.

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

package cmd

import (
	"fmt"
	"strings"

	"github.com/blang/semver"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/kubeedge/kubeedge/common/constants"
	types "github.com/kubeedge/kubeedge/keadm/cmd/keadm/app/cmd/common"
	"github.com/kubeedge/kubeedge/keadm/cmd/keadm/app/cmd/util"
)

var (
	cloudManifestLongDescription = `
"keadm manifest" command renders charts by using a list of set flags like helm.
`

	cloudManifestGenerateLongDescription = `
"keadm manifest generate" command renders charts by using a list of set flags like helm, and generates kubernetes resources.
`

	cloudManifestExample = `
keadm manifest

- This command will render Kubernetes resources

keadm manifest generate --advertise-address=127.0.0.1 [--set cloudcore-tag=v1.9.0] --profile version=v1.9.0 -n kubeedge --kube-config=/root/.kube/config

  - kube-config is the absolute path of kubeconfig which used to secure connectivity between cloudcore and kube-apiserver
`

	cloudManifestGenerateExample = `
keadm manifest generate

- This command will render and generate Kubernetes resources

keadm manifest generate --advertise-address=127.0.0.1 [--set cloudcore-tag=v1.9.0] --profile version=v1.9.0 -n kubeedge --kube-config=/root/.kube/config

  - kube-config is the absolute path of kubeconfig which used to secure connectivity between cloudcore and kube-apiserver
`
)

// NewCloudInit represents the keadm init command for cloud component
func NewCloudManifestGenerate() *cobra.Command {
	initbeta := newInitBetaOptions()
	tools := make(map[string]types.ToolsInstaller)
	flagVals := make(map[string]types.FlagData)

	var generateCmd = &cobra.Command{
		Use:     "generate",
		Short:   "Checks and generate the manifests.",
		Long:    cloudManifestGenerateLongDescription,
		Example: cloudManifestGenerateExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			checkFlags := func(f *pflag.Flag) {
				util.AddToolVals(f, flagVals)
			}
			cmd.Flags().VisitAll(checkFlags)
			err := AddManifestsGenerate2ToolsList(tools, flagVals, initbeta)
			if err != nil {
				return err
			}
			return ExecuteManifestsGenerate(tools)
		},
	}

	addManifestsGenerateJoinOtherFlags(generateCmd, initbeta)

	var manifestCmd = &cobra.Command{
		Use:     "manifest",
		Short:   "Render the manifests by using a list of set flags like helm.",
		Long:    cloudManifestLongDescription,
		Example: cloudManifestExample,
	}
	manifestCmd.AddCommand(generateCmd)
	return manifestCmd
}

func addManifestsGenerateJoinOtherFlags(cmd *cobra.Command, initBetaOpts *types.InitBetaOptions) {
	addInitBetaJoinOtherFlags(cmd, initBetaOpts)
	addHelmValueOptionsFlags(cmd, initBetaOpts)

	cmd.Flags().BoolVar(&initBetaOpts.SkipCRDs, types.SkipCRDs, initBetaOpts.SkipCRDs,
		"Print the generated k8s resources on the stdout, not actual excute. Always use in debug mode")
}

//AddManifestsGenerate2ToolsList Reads the flagData (containing val and default val) and join options to fill the list of tools.
func AddManifestsGenerate2ToolsList(toolList map[string]types.ToolsInstaller, flagData map[string]types.FlagData, initBetaOptions *types.InitBetaOptions) error {
	var kubeVer string
	var latestVersion string
	for i := 0; i < util.RetryTimes; i++ {
		version, err := util.GetLatestVersion()
		if err != nil {
			fmt.Println("Failed to get the latest KubeEdge release version, error: ", err)
			continue
		}
		if len(version) > 0 {
			kubeVer = strings.TrimPrefix(version, "v")
			latestVersion = version
			break
		}
	}
	if len(latestVersion) == 0 {
		kubeVer = types.DefaultKubeEdgeVersion
		fmt.Println("Failed to get the latest KubeEdge release version, will use default version: ", kubeVer)
	}

	if initBetaOptions.Namespace == "" {
		initBetaOptions.Namespace = constants.SystemNamespace
	}

	common := util.Common{
		ToolVersion: semver.MustParse(kubeVer),
		KubeConfig:  initBetaOptions.KubeConfig,
	}
	toolList["helm"] = &util.KubeCloudHelmInstTool{
		Common:           common,
		AdvertiseAddress: initBetaOptions.AdvertiseAddress,
		Manifests:        initBetaOptions.Manifests,
		Namespace:        initBetaOptions.Namespace,
		DryRun:           initBetaOptions.DryRun,
		CloudcoreImage:   initBetaOptions.CloudcoreImage,
		CloudcoreTag:     initBetaOptions.CloudcoreTag,
		IptablesMgrImage: initBetaOptions.IptablesMgrImage,
		IptablesMgrTag:   initBetaOptions.IptablesMgrTag,
		Sets:             initBetaOptions.Sets,
		Profile:          initBetaOptions.Profile,
		SkipCRDs:         initBetaOptions.SkipCRDs,
		Action:           types.HelmManifestAction,
	}
	return nil
}

//ExecuteInitBeta the installation for each tool and start cloudcore
func ExecuteManifestsGenerate(toolList map[string]types.ToolsInstaller) error {
	return toolList["helm"].InstallTools()
}
