/*
Copyright (C) 2017 Red Hat, Inc.

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

package addon

import (
	"fmt"

	"github.com/docker/machine/libmachine"
	"github.com/docker/machine/libmachine/drivers"
	"github.com/docker/machine/libmachine/provision"
	"github.com/minishift/minishift/cmd/minishift/cmd/util"
	"github.com/minishift/minishift/pkg/minikube/constants"
	"github.com/minishift/minishift/pkg/minishift/clusterup"
	minishiftConfig "github.com/minishift/minishift/pkg/minishift/config"
	"github.com/minishift/minishift/pkg/minishift/docker"
	"github.com/minishift/minishift/pkg/minishift/oc"
	"github.com/minishift/minishift/pkg/minishift/openshift"
	"github.com/minishift/minishift/pkg/util/os/atexit"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

const (
	routingSuffix = "routing-suffix"
)

var addonsApplyCmd = &cobra.Command{
	Use:   "apply ADDON_NAME ...",
	Short: "Executes the specified add-ons.",
	Long:  "Executes the specified add-ons. The command works with both enabled and disabled add-ons.",
	Run:   runApplyAddon,
}

func init() {
	AddonsCmd.AddCommand(addonsApplyCmd)
}

func runApplyAddon(cmd *cobra.Command, args []string) {

	if len(args) == 0 {
		atexit.ExitWithMessage(1, emptyAddOnError)
	}

	addOnManager := GetAddOnManager()
	for i := range args {
		addonName := args[i]
		if !addOnManager.IsInstalled(addonName) {
			atexit.ExitWithMessage(0, fmt.Sprintf(noAddOnMessage, addonName))
		}
	}

	api := libmachine.NewClient(constants.Minipath, constants.MakeMiniPath("certs"))
	defer api.Close()

	util.ExitIfUndefined(api, constants.MachineName)

	host, err := api.Load(constants.MachineName)
	if err != nil {
		atexit.ExitWithMessage(1, err.Error())
	}

	util.ExitIfNotRunning(host.Driver, constants.MachineName)

	ip, err := host.Driver.GetIP()
	if err != nil {
		atexit.ExitWithMessage(1, fmt.Sprintf("Error getting IP: %s", err.Error()))
	}

	routingSuffix := determineRoutingSuffix(host.Driver)
	sshCommander := provision.GenericSSHCommander{Driver: host.Driver}
	ocRunner, err := oc.NewOcRunner(minishiftConfig.InstanceConfig.OcPath, constants.KubeConfigPath)
	if err != nil {
		atexit.ExitWithMessage(1, fmt.Sprintf("Error applying addon: %s", err.Error()))
	}

	for i := range args {
		addonName := args[i]
		addon := addOnManager.Get(addonName)
		addonContext, err := clusterup.GetExecutionContext(ip, routingSuffix, ocRunner, sshCommander)
		if err != nil {
			atexit.ExitWithMessage(1, fmt.Sprint("Error executing addon commands: ", err))
		}
		err = addOnManager.ApplyAddOn(addon, addonContext)
		if err != nil {
			atexit.ExitWithMessage(1, fmt.Sprint("Error executing addon commands: ", err))
		}
	}
}

func determineRoutingSuffix(driver drivers.Driver) string {
	defer func() {
		if r := recover(); r != nil {
			atexit.ExitWithMessage(1, "Unable to determine routing suffix from OpenShift master config.")
		}
	}()

	sshCommander := provision.GenericSSHCommander{Driver: driver}
	dockerCommander := docker.NewVmDockerCommander(sshCommander)

	raw, err := openshift.ViewConfig(openshift.MASTER, dockerCommander)
	if err != nil {
		atexit.ExitWithMessage(1, fmt.Sprintf("Unable to retrieve OpenShift master configuration: %s", err.Error()))
	}

	var config map[interface{}]interface{}
	err = yaml.Unmarshal([]byte(raw), &config)
	if err != nil {
		atexit.ExitWithMessage(1, fmt.Sprintf("Unable to parse OpenShift master configuration: %s", err.Error()))
	}

	// making assumptions about the master config here. In case the config structure changes, the code might panic here
	return config["routingConfig"].(map[interface{}]interface{})["subdomain"].(string)
}
