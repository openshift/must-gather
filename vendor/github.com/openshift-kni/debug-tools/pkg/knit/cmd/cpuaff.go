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
	"sort"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/openshift-kni/debug-tools/pkg/procs"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

type cpuAffOptions struct {
	pidIdent string
}

func NewCPUAffinityCommand(knitOpts *KnitOptions) *cobra.Command {
	opts := &cpuAffOptions{}
	cpuAff := &cobra.Command{
		Use:   "cpuaff",
		Short: "show cpu thread affinities",
		RunE: func(cmd *cobra.Command, args []string) error {
			return showCPUAffinity(cmd, knitOpts, opts, args)
		},
		Args: cobra.NoArgs,
	}
	cpuAff.Flags().StringVarP(&opts.pidIdent, "pid", "p", "", "monitor only threads belonging to this pid (default is all).")
	return cpuAff
}

type runnable struct {
	PID         int    `json:"pid"`
	TID         int    `json:"tid"`
	ProcessName string `json:"process"`
	ThreadName  string `json:"thread"`
	CPUAffinity []int  `json:"affinity"`
}

func (ru runnable) String() string {
	// see: https://man7.org/linux/man-pages/man3/pthread_setname_np.3.html
	// "The thread name is a meaningful C language string, whose length is restricted to 16 characters,
	// including the terminating null byte"
	// for process names howevwver we just pick a "usually long enough" format value
	return fmt.Sprintf("PID %6d (%-32s) TID %6d (%-16s) can run on %v", ru.PID, ru.ProcessName, ru.TID, ru.ThreadName, ru.CPUAffinity)
}

func showCPUAffinity(cmd *cobra.Command, knitOpts *KnitOptions, opts *cpuAffOptions, args []string) error {
	ph := procs.New(knitOpts.Log, knitOpts.ProcFSRoot)

	if opts.pidIdent != "" {
		pid, err := strconv.Atoi(opts.pidIdent)
		if err != nil {
			return fmt.Errorf("error parsing %q: %v", opts.pidIdent, err)
		}
		procInfo, err := ph.FromPID(pid)
		for tid, tidInfo := range procInfo.TIDs {
			threadCpus := cpuset.NewCPUSet(tidInfo.Affinity...)

			cpus := threadCpus.Intersection(knitOpts.Cpus)
			if cpus.Size() != 0 {
				fmt.Printf("PID %6d TID %6d can run on %v\n", pid, tid, cpus.String())
			}
		}
		return nil
	}

	procInfos, err := ph.ListAll()
	if err != nil {
		return fmt.Errorf("error getting process infos from %q: %v", knitOpts.ProcFSRoot, err)
	}

	var runnables []runnable
	for _, pid := range sortedPids(procInfos) {
		procInfo := procInfos[pid]

		for _, tid := range sortedTids(procInfo.TIDs) {
			tidInfo := procInfo.TIDs[tid]

			threadCpus := cpuset.NewCPUSet(tidInfo.Affinity...)
			cpus := threadCpus.Intersection(knitOpts.Cpus)
			if cpus.Size() == 0 {
				continue
			}
			runnables = append(runnables, runnable{
				PID:         pid,
				TID:         tid,
				ProcessName: procInfo.Name,
				ThreadName:  tidInfo.Name,
				CPUAffinity: cpus.ToSlice(),
			})
		}
	}

	if knitOpts.JsonOutput {
		json.NewEncoder(os.Stdout).Encode(runnables)
	} else {
		for _, runnable := range runnables {
			fmt.Println(runnable.String())
		}
	}
	return nil
}

func sortedPids(procInfos map[int]procs.PIDInfo) []int {
	pids := make([]int, len(procInfos))
	for pid := range procInfos {
		pids = append(pids, pid)
	}
	sort.Ints(pids)
	return pids
}

func sortedTids(tidInfos map[int]procs.TIDInfo) []int {
	tids := make([]int, len(tidInfos))
	for tid := range tidInfos {
		tids = append(tids, tid)
	}
	sort.Ints(tids)
	return tids
}
