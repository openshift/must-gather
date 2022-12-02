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
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-kni/debug-tools/pkg/irqs"
	softirqs "github.com/openshift-kni/debug-tools/pkg/irqs/soft"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

type irqAffOptions struct {
	checkEffective  bool
	checkSoftirqs   bool
	showEmptySource bool
}

func NewIRQAffinityCommand(knitOpts *KnitOptions) *cobra.Command {
	opts := &irqAffOptions{}
	irqAff := &cobra.Command{
		Use:   "irqaff",
		Short: "show IRQ/softirq thread affinities",
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.checkSoftirqs {
				return showSoftIRQAffinity(cmd, knitOpts, opts, args)
			} else {
				return showIRQAffinity(cmd, knitOpts, opts, args)
			}
		},
		Args: cobra.NoArgs,
	}
	irqAff.Flags().BoolVarP(&opts.checkEffective, "effective-affinity", "E", false, "check effective affinity.")
	irqAff.Flags().BoolVarP(&opts.checkSoftirqs, "softirqs", "s", false, "check softirqs counters.")
	irqAff.Flags().BoolVarP(&opts.showEmptySource, "show-empty-source", "e", false, "show infos if IRQ source is not reported.")
	return irqAff
}

type irqAffinity struct {
	IRQ         int    `json:"irq"`
	Source      string `json:"source"`
	CPUAffinity []int  `json:"affinity"`
}

func (ia irqAffinity) String() string {
	return fmt.Sprintf("IRQ %3d [%24s]: can run on %v", ia.IRQ, ia.Source, ia.CPUAffinity)
}

type softirqAffinity struct {
	SoftIRQ     string `json:"softirq"`
	CPUAffinity []int  `json:"affinity"`
}

func (sa softirqAffinity) String() string {
	return fmt.Sprintf("%8s = %v", sa.SoftIRQ, sa.CPUAffinity)
}

func showIRQAffinity(cmd *cobra.Command, knitOpts *KnitOptions, opts *irqAffOptions, args []string) error {
	ih := irqs.New(knitOpts.Log, knitOpts.ProcFSRoot)

	flags := uint(0)
	if opts.checkEffective {
		flags |= irqs.EffectiveAffinity
	}

	irqInfos, err := ih.ReadInfo(flags)
	if err != nil {
		return fmt.Errorf("error parsing irqs from %q: %v", knitOpts.ProcFSRoot, err)
	}

	var irqAffinities []irqAffinity
	for _, irqInfo := range irqInfos {
		cpus := irqInfo.CPUs.Intersection(knitOpts.Cpus)
		if cpus.Size() == 0 {
			continue
		}
		if irqInfo.Source == "" && !opts.showEmptySource {
			continue
		}
		irqAffinities = append(irqAffinities, irqAffinity{
			IRQ:         irqInfo.IRQ,
			Source:      irqInfo.Source,
			CPUAffinity: cpus.ToSlice(),
		})
	}

	if knitOpts.JsonOutput {
		json.NewEncoder(os.Stdout).Encode(irqAffinities)
	} else {
		for _, irqAffinity := range irqAffinities {
			fmt.Println(irqAffinity.String())
		}
	}
	return nil
}

func showSoftIRQAffinity(cmd *cobra.Command, knitOpts *KnitOptions, opts *irqAffOptions, args []string) error {
	sh := softirqs.New(knitOpts.Log, knitOpts.ProcFSRoot)
	info, err := sh.ReadInfo()

	if err != nil {
		return fmt.Errorf("error parsing softirqs from %q: %v", knitOpts.ProcFSRoot, err)
	}

	var softirqAffinities []softirqAffinity
	keys := softirqs.Names()
	for _, key := range keys {
		counters := info.Counters[key]
		cb := cpuset.NewBuilder()
		for idx, counter := range counters {
			if counter > 0 {
				cb.Add(idx)
			}
		}
		usedCPUs := knitOpts.Cpus.Intersection(cb.Result())

		softirqAffinities = append(softirqAffinities, softirqAffinity{
			SoftIRQ:     key,
			CPUAffinity: usedCPUs.ToSlice(),
		})
	}

	if knitOpts.JsonOutput {
		json.NewEncoder(os.Stdout).Encode(softirqAffinities)
	} else {
		for _, softirqAffinity := range softirqAffinities {
			fmt.Println(softirqAffinity.String())
		}
	}
	return nil
}
