/*
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright 2020 Red Hat, Inc.
 */

package cmd

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

type KnitOptions struct {
	Cpus       cpuset.CPUSet
	ProcFSRoot string
	SysFSRoot  string
	JsonOutput bool
	Debug      bool
	Log        *log.Logger
	cpuList    string
}

func ShowHelp(cmd *cobra.Command, args []string) error {
	fmt.Fprint(cmd.OutOrStderr(), cmd.UsageString())
	return nil
}

type NewCommandFunc func(ko *KnitOptions) *cobra.Command

// NewRootCommand returns entrypoint command to interact with all other commands
func NewRootCommand(extraCmds ...NewCommandFunc) *cobra.Command {
	knitOpts := &KnitOptions{}

	root := &cobra.Command{
		Use:   "knit",
		Short: "knit allows to check system settings for low-latency workload",

		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			var err error
			knitOpts.Cpus, err = cpuset.Parse(knitOpts.cpuList)
			if err != nil {
				return fmt.Errorf("error parsing %q: %v", knitOpts.cpuList, err)
			}

			if knitOpts.Debug {
				knitOpts.Log = log.New(os.Stderr, "knit ", log.LstdFlags)
			} else {
				knitOpts.Log = log.New(ioutil.Discard, "", 0)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return ShowHelp(cmd, args)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// see https://man7.org/linux/man-pages/man7/cpuset.7.html#FORMATS for more details
	root.PersistentFlags().StringVarP(&knitOpts.cpuList, "cpulist", "C", "0-16383", "isolated cpu set to check (see man (7) cpuset - List format")
	root.PersistentFlags().StringVarP(&knitOpts.ProcFSRoot, "procfs", "P", "/proc", "procfs root")
	root.PersistentFlags().StringVarP(&knitOpts.SysFSRoot, "sysfs", "S", "/sys", "sysfs root")
	root.PersistentFlags().BoolVarP(&knitOpts.Debug, "debug", "D", false, "enable debug log")
	root.PersistentFlags().BoolVarP(&knitOpts.JsonOutput, "json", "J", false, "output as JSON")

	root.AddCommand(
		NewCPUAffinityCommand(knitOpts),
		NewIRQAffinityCommand(knitOpts),
		NewIRQWatchCommand(knitOpts),
		NewWaitCommand(knitOpts),
	)
	for _, extraCmd := range extraCmds {
		root.AddCommand(extraCmd(knitOpts))
	}

	return root
}
