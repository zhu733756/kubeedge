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
	cloudInitBetaLongDescription = `
"keadm init-beta" command install KubeEdge's master node (on the cloud) component by using a list of set flags like helm.
It checks if the Kubernetes Master are installed already,
If not installed, please install the Kubernetes first.
`
	cloudInitBetaExample = `
keadm init-beta

- This command will render and install the Charts for Kubeedge cloud component

keadm init-beta --advertise-address=127.0.0.1 [--set cloudcore-tag=v1.9.0] --profile version=v1.9.0 -n kubeedge --kube-config=/root/.kube/config

  - kube-config is the absolute path of kubeconfig which used to secure connectivity between cloudcore and kube-apiserver
`
)

// NewCloudInit represents the keadm init command for cloud component
func NewCloudInitBeta() *cobra.Command {
	initbeta := newInitBetaOptions()

	tools := make(map[string]types.ToolsInstaller)
	flagVals := make(map[string]types.FlagData)

	var cmd = &cobra.Command{
		Use:     "init-beta",
		Short:   "Bootstraps cloud component. Checks and install (if required) the pre-requisites.",
		Long:    cloudInitBetaLongDescription,
		Example: cloudInitBetaExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			checkFlags := func(f *pflag.Flag) {
				util.AddToolVals(f, flagVals)
			}
			cmd.Flags().VisitAll(checkFlags)
			err := AddInitBeta2ToolsList(tools, flagVals, initbeta)
			if err != nil {
				return err
			}
			return ExecuteInitBeta(tools)
		},
	}

	addInitBetaJoinOtherFlags(cmd, initbeta)
	addHelmValueOptionsFlags(cmd, initbeta)
	addForceOptionsFlags(cmd, initbeta)
	return cmd
}

//newInitBetaOptions will initialise new instance of options everytime
func newInitBetaOptions() *types.InitBetaOptions {
	opts := &types.InitBetaOptions{}
	opts.KubeConfig = types.DefaultKubeConfig
	return opts
}

func addInitBetaJoinOtherFlags(cmd *cobra.Command, initBetaOpts *types.InitBetaOptions) {
	cmd.Flags().StringVar(&initBetaOpts.AdvertiseAddress, types.AdvertiseAddress, initBetaOpts.AdvertiseAddress,
		"Use this key to set IPs in cloudcore's certificate SubAltNames field. eg: 10.10.102.78,10.10.102.79")

	cmd.Flags().StringVar(&initBetaOpts.KubeConfig, types.KubeConfig, initBetaOpts.KubeConfig,
		"Use this key to set kube-config path, eg: $HOME/.kube/config")

	cmd.Flags().StringVar(&initBetaOpts.Manifests, types.Manifests, initBetaOpts.Manifests,
		"Allow appending file directories of k8s resources to keadm, separated by commas")

	cmd.Flags().StringVarP(&initBetaOpts.Manifests, types.Files, "f", initBetaOpts.Manifests,
		"Allow appending file directories of k8s resources to keadm, separated by commas")

	cmd.Flags().StringVarP(&initBetaOpts.Namespace, types.Namespace, "n", initBetaOpts.Namespace,
		"Namespace to install kubeedge cloud component, default is kubeedge")

	cmd.Flags().BoolVarP(&initBetaOpts.DryRun, types.DryRun, "d", initBetaOpts.DryRun,
		"Print the generated k8s resources on the stdout, not actual excute. Always use in debug mode")

	cmd.Flags().StringVar(&initBetaOpts.CloudcoreImage, types.CloudcoreImage, initBetaOpts.CloudcoreImage,
		"The whole image of the cloudcore, default is kubeedge/cloudcore:v1.9.0")

	cmd.Flags().StringVar(&initBetaOpts.CloudcoreTag, types.CloudcoreTag, initBetaOpts.CloudcoreTag,
		"The image tag of the cloudcore, default is v1.9.0")

	cmd.Flags().StringVar(&initBetaOpts.IptablesMgrImage, types.IptablesMgrImage, initBetaOpts.IptablesMgrImage,
		"The whole image of the iptables manager, default is kubeedge/cloudcore:v1.9.0")

	cmd.Flags().StringVar(&initBetaOpts.IptablesMgrTag, types.IptablesMgrTag, initBetaOpts.IptablesMgrTag,
		"The image tag of the iptables manager, default is v1.9.0")
}

func addHelmValueOptionsFlags(cmd *cobra.Command, initBetaOpts *types.InitBetaOptions) {
	cmd.Flags().StringArrayVar(&initBetaOpts.Sets, "set", []string{}, "set values on the command line (can specify multiple or separate values with commas: key1=val1,key2=val2)")
	cmd.Flags().StringVar(&initBetaOpts.Profile, "profile", initBetaOpts.Profile, "set profile on the command line (iptablesMgrMode=external or version=1.9.1)")
}

func addForceOptionsFlags(cmd *cobra.Command, initBetaOpts *types.InitBetaOptions) {
	cmd.Flags().BoolVar(&initBetaOpts.Force, types.Force, initBetaOpts.Force,
		"Forced installing the cloud components.")
}

//Add2ToolsList Reads the flagData (containing val and default val) and join options to fill the list of tools.
func AddInitBeta2ToolsList(toolList map[string]types.ToolsInstaller, flagData map[string]types.FlagData, initBetaOptions *types.InitBetaOptions) error {
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
		Force:            initBetaOptions.Force,
		Action:           types.HelmInstallAction,
	}
	return nil
}

//ExecuteInitBeta the installation for each tool and start cloudcore
func ExecuteInitBeta(toolList map[string]types.ToolsInstaller) error {
	return toolList["helm"].InstallTools()
}
