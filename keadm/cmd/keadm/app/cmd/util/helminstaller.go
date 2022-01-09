package util

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"sort"
	"strings"
	"time"

	"github.com/blang/semver"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/storage/driver"
	"helm.sh/helm/v3/pkg/strvals"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/yaml"

	keCharts "github.com/kubeedge/kubeedge/build/helm/charts"
	"github.com/kubeedge/kubeedge/common/constants"
	types "github.com/kubeedge/kubeedge/keadm/cmd/keadm/app/cmd/common"
)

const (
	DefaultHelmRoot        = ""
	CloudCoreHelmComponent = "cloudcore"
	EdgemeshHelmComponent  = "cloudcore"
	EdgemeshHelmDir        = "edgemesh"
	CloudCoreHelmDir       = "cloudcore"
	AddonsHelmDir          = "addons"
	DefaultHelmTimeout     = time.Duration(60 * time.Second)
	DefaultHelmInstall     = true
	DefaultHelmWait        = true
	DefaultHelmCreateNs    = true
)

// KubeCloudHelmInstTool embeds Common struct
// It implements ToolsInstaller interface
type KubeCloudHelmInstTool struct {
	Common
	AdvertiseAddress string
	Manifests        string
	Namespace        string
	DryRun           bool
	CloudcoreImage   string
	CloudcoreTag     string
	IptablesMgrImage string
	IptablesMgrTag   string
	Sets             []string
	Profile          string
	ProfileKey       string
	Force            bool
}

// InstallTools downloads KubeEdge for the specified version
// and makes the required configuration changes and initiates cloudcore.
func (cu *KubeCloudHelmInstTool) InstallTools() error {
	cu.SetOSInterface(GetOSInterface())
	cu.SetKubeEdgeVersion(cu.ToolVersion)

	// --force would not care about whether the cloud components exist or not
	if !cu.Force {
		cloudCoreRunning, err := cu.IsKubeEdgeProcessRunning(KubeCloudBinaryName)
		if err != nil {
			return err
		}
		if cloudCoreRunning {
			return fmt.Errorf("CloudCore is already running on this node, please run reset to clean up first")
		}
	}

	err := cu.IsK8SComponentInstalled(cu.KubeConfig, cu.Master)
	if err != nil {
		return err
	}

	fmt.Println("Kubernetes version verification passed, KubeEdge installation will start...")

	// prepare to render
	if err := cu.BeforeRenderer(); err != nil {
		return err
	}

	// build a renderer instance with the given values and flagvals
	renderer, err := cu.buildRenderer()
	if err != nil {
		return fmt.Errorf("cannot build chart render %s, error: %s", renderer.componentName, err.Error())
	}

	// load the charts to this renderer
	if err := renderer.LoadChart(); err != nil {
		return fmt.Errorf("cannot load the given charts %s, error: %s", renderer.componentName, err.Error())
	}

	if err := cu.RunHelmInstall(renderer); err != nil {
		return err
	}

	fmt.Println("CloudCore started")
	return nil
}

// BeforeRenderer handles the value of the profile.
func (cu *KubeCloudHelmInstTool) BeforeRenderer() error {
	if cu.Profile == "" {
		cu.Profile = fmt.Sprintf("%s=%s", types.VersionProfileKey, types.HelmSupportedMinVersion)
	}
	// profile must be invalid
	p := strings.Split(cu.Profile, "=")
	if len(p) < 2 {
		return fmt.Errorf("invalid profile %s", cu.Profile)
	}

	// check and handle profile
	cu.ProfileKey = p[0]
	if err := cu.checkProfile(); err != nil {
		return fmt.Errorf("invalid profile %s", cu.Profile)
	}
	if err := cu.handleProfile(p[1]); err != nil {
		return fmt.Errorf("can not handle profile %s", cu.Profile)
	}

	// rebuild flag values
	if err := cu.rebuildFlagVals(); err != nil {
		return err
	}

	return nil
}

// buildRenderer returns a renderer instance
func (cu *KubeCloudHelmInstTool) buildRenderer() (*Renderer, error) {
	profileValsMap, err := cu.combineProfVals()
	if err != nil {
		return nil, err
	}
	// confirm which chart to load
	var componentName string
	var subDir string
	if cu.isInnerProfile() {
		switch cu.ProfileKey {
		case types.VersionProfileKey, types.IptablesMgrProfileKey:
			componentName = CloudCoreHelmComponent
			subDir = CloudCoreHelmDir
		case types.EdgemeshProfileKey:
			// edgemesh will integrate later
			componentName = EdgemeshHelmComponent
			subDir = EdgemeshHelmDir
		default:
			componentName = CloudCoreHelmComponent
			subDir = CloudCoreHelmDir
		}
	} else {
		componentName = cu.ProfileKey
		subDir = fmt.Sprintf("%s/%s", AddonsHelmDir, cu.ProfileKey)
	}

	// render the chart with the given values
	render := NewGenericRenderer(keCharts.BuiltinOrDir(DefaultHelmRoot), subDir, componentName, cu.Namespace, profileValsMap)
	return render, nil
}

// RunHelmInstall starts cloudcore deployment with the given flags
func (cu *KubeCloudHelmInstTool) HelmRenderer(r *Renderer) error {
	manifiests, err := r.RenderManifest()
	if err != nil {
		return fmt.Errorf("cannot render the given compoent %s, error: %s", r.componentName, err.Error())
	}

	// combine the given manifests and the rendered manifests
	var buf bytes.Buffer
	if cu.Manifests != "" {
		for _, manifest := range strings.Split(cu.Manifests, ",") {
			body, err := ioutil.ReadFile(manifest)
			if err != nil {
				return fmt.Errorf("cannot open file %s, error: %s", manifest, err.Error())
			}
			buf.WriteString(fmt.Sprintf("%b%s", body, YAMLSeparator))
		}
	}
	buf.WriteString(manifiests)

	cu.Manifests = buf.String()
	return nil
}

// RunHelmInstall starts cloudcore deployment with the given flags
func (cu *KubeCloudHelmInstTool) RunHelmInstall(r *Renderer) error {
	cf := genericclioptions.NewConfigFlags(true)
	cf.KubeConfig = &cu.KubeConfig
	cf.Namespace = &cu.Namespace

	cfg := &action.Configuration{}
	logFunc := func(format string, v ...interface{}) {
		fmt.Println(fmt.Sprintf(format, v...))
	}
	if err := cfg.Init(cf, cu.Namespace, "", logFunc); err != nil {
		return err
	}

	// try to update a version
	helmUpgrade := action.NewUpgrade(cfg)
	helmUpgrade.DryRun = cu.DryRun
	helmUpgrade.Namespace = cu.Namespace
	// --force would not wait.
	if !cu.Force {
		helmUpgrade.Wait = DefaultHelmWait
		helmUpgrade.Timeout = DefaultHelmTimeout
	}
	helmUpgrade.Install = DefaultHelmInstall

	_, err := helmUpgrade.Run(r.componentName, r.chart, r.profileValsMap)
	if err != nil {
		// if the error returns is errReleaseNotFound, would try to install it.
		errReleaseNotFound := driver.NewErrNoDeployedReleases(r.componentName).Error()
		if err.Error() == errReleaseNotFound {
			helmInstall := action.NewInstall(cfg)
			helmInstall.DryRun = cu.DryRun
			helmInstall.Namespace = cu.Namespace
			if !cu.Force {
				helmInstall.Wait = DefaultHelmWait
				helmInstall.Timeout = DefaultHelmTimeout
			}
			helmInstall.CreateNamespace = DefaultHelmCreateNs
			helmInstall.ReleaseName = r.componentName

			if _, err := helmInstall.Run(r.chart, r.profileValsMap); err != nil {
				return err
			}
			return nil
		}
		return err
	}
	return nil
}

// TearDown method will remove the edge node from api-server and stop cloudcore process
func (cu *KubeCloudHelmInstTool) TearDown() error {
	// clean kubeedge namespace
	err := cu.cleanNameSpace(constants.SystemNamespace, cu.KubeConfig)
	if err != nil {
		return fmt.Errorf("fail to clean kubeedge namespace, err:%v", err)
	}
	return nil
}

func (cu *KubeCloudHelmInstTool) checkProfile() error {
	validProfiles, err := readProfiles(DefaultHelmRoot)
	if err != nil {
		return fmt.Errorf("cannot list profile")
	}
	if ok := validProfiles[cu.ProfileKey]; !ok {
		return fmt.Errorf(fmt.Sprintf("unsupported profile %s", cu.Profile))
	}

	return nil
}

func (cu *KubeCloudHelmInstTool) handleProfile(profileValue string) error {
	switch cu.ProfileKey {
	case types.VersionProfileKey:
		profileValueSuffix := strings.TrimPrefix(profileValue, "v")
		// confirm it startswith "v"
		if profileValue != profileValueSuffix {
			version, err := semver.Make(profileValueSuffix)
			if err != nil {
				return err
			}
			minVersion, _ := semver.Make(strings.TrimPrefix(types.HelmSupportedMinVersion, "v"))
			if version.LT(minVersion) {
				return fmt.Errorf("the given version %s is not supported, you can try binary deployments with this version", profileValue)
			}

			cu.Sets = append(cu.Sets, fmt.Sprintf("%s=v%s", "cloudCore.image.tag", profileValueSuffix))
			cu.Sets = append(cu.Sets, fmt.Sprintf("%s=v%s", "iptablesManager.image.tag", profileValueSuffix))
		} else {
			cu.Sets = append(cu.Sets, fmt.Sprintf("%s=%s", "cloudCore.image.tag", profileValue))
			cu.Sets = append(cu.Sets, fmt.Sprintf("%s=%s", "iptablesManager.image.tag", profileValue))
		}
	case types.IptablesMgrProfileKey:
		switch profileValue {
		case types.InternalIptablesMgrMode, types.ExternalIptablesMgrMode:
			cu.Sets = append(cu.Sets, fmt.Sprintf("%s=%s", "iptablesManager.mode", profileValue))
		default:
			profileValue = types.ExternalIptablesMgrMode
		}
	default:
	}
	return nil
}

func (cu *KubeCloudHelmInstTool) rebuildFlagVals() error {
	// combine the flag values
	if cu.AdvertiseAddress != "" {
		for index, addr := range strings.Split(cu.AdvertiseAddress, ",") {
			cu.Sets = append(cu.Sets, fmt.Sprintf("%s[%d]=%s", "cloudCore.modules.cloudHub.advertiseAddress", index, addr))
		}
	}
	if cu.CloudcoreImage != "" {
		cu.Sets = append(cu.Sets, fmt.Sprintf("%s=%s", "cloudCore.image.repository", cu.CloudcoreImage))
	}
	if cu.CloudcoreTag != "" {
		cu.Sets = append(cu.Sets, fmt.Sprintf("%s=%s", "cloudCore.image.tag", cu.CloudcoreTag))
	}
	if cu.IptablesMgrImage != "" {
		cu.Sets = append(cu.Sets, fmt.Sprintf("%s=%s", "iptablesManager.image.repository", cu.IptablesMgrImage))
	}
	if cu.IptablesMgrTag != "" {
		cu.Sets = append(cu.Sets, fmt.Sprintf("%s=%s", "iptablesManager.image.tag", cu.IptablesMgrTag))
	}

	var formerValue string
	sets := make([]string, 0)

	sort.Strings(cu.Sets)
	for index, s := range cu.Sets {
		p := strings.Split(s, "=")

		if len(p) < 2 {
			fmt.Println("Unsported flags:", s)
			continue
		}

		if index > 0 && p[0] == formerValue {
			// duplicate removal
			sets[len(sets)-1] = s
		} else {
			sets = append(sets, s)
		}

		formerValue = p[0]
	}

	cu.Sets = sets
	return nil
}

func (cu *KubeCloudHelmInstTool) isInnerProfile() bool {
	return cu.ProfileKey == "" || cu.ProfileKey == DefaultProfileString || cu.ProfileKey == types.IptablesMgrProfileKey || cu.ProfileKey == types.EdgemeshProfileKey
}

// combineProfVals combines the values of the given manifests and flags into a map.
func (cu *KubeCloudHelmInstTool) combineProfVals() (map[string]interface{}, error) {
	profileValsMap := map[string]interface{}{}

	profileValue, err := LoadValues(cu.ProfileKey, DefaultHelmRoot)
	if err != nil {
		return nil, fmt.Errorf("cannot load profile yaml:%s", err.Error())
	}

	if err := yaml.Unmarshal([]byte(profileValue), &profileValsMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal values: %v", err)
	}
	// User specified a value via --set
	for _, value := range cu.Sets {
		if err := strvals.ParseInto(value, profileValsMap); err != nil {
			return nil, fmt.Errorf("failed parsing --set data:%s", err.Error())
		}
	}

	return profileValsMap, nil
}
