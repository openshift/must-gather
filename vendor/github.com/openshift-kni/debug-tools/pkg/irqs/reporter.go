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

package irqs

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

type Reporter interface {
	Delta(ts time.Time, prevStats, lastStats Stats)
	Summary(initTs time.Time, prevStats, lastStats Stats)
}

func NewReporter(sink io.Writer, jsonOutput bool, verbose int, cpus cpuset.CPUSet) Reporter {
	if jsonOutput {
		return &reporterJSON{
			verbose: verbose,
			cpus:    cpus,
			sink:    sink,
		}
	}
	return &reporterText{
		verbose: verbose,
		cpus:    cpus,
		sink:    sink,
	}

}

type reporterText struct {
	verbose int
	cpus    cpuset.CPUSet
	sink    io.Writer
}

func (rt *reporterText) Delta(ts time.Time, prevStats, lastStats Stats) {
	if rt.verbose < 2 {
		return
	}
	delta := prevStats.Delta(lastStats)
	cpuids := rt.cpus.ToSlice()
	for _, cpuid := range cpuids {
		counter, ok := delta[cpuid]
		if !ok {
			continue
		}
		for irqName, val := range counter {
			if val == 0 {
				continue
			}
			fmt.Fprintf(rt.sink, "%v CPU=%d IRQ=%s +%d\n", ts, cpuid, irqName, val)
		}
	}
}

func (rt *reporterText) Summary(initTs time.Time, prevStats, lastStats Stats) {
	if rt.verbose < 1 {
		return
	}
	timeDelta := time.Now().Sub(initTs)
	delta := prevStats.Delta(lastStats)
	cpuids := rt.cpus.ToSlice()

	fmt.Fprintf(rt.sink, "\nIRQ summary on cpus %v after %v\n", rt.cpus, timeDelta)
	for _, cpuid := range cpuids {
		counter, ok := delta[cpuid]
		if !ok {
			continue
		}
		for irqName, val := range counter {
			if val == 0 {
				continue
			}
			fmt.Fprintf(rt.sink, "CPU=%d IRQ=%s +%d\n", cpuid, irqName, val)
		}
	}
}

type reporterJSON struct {
	verbose int
	cpus    cpuset.CPUSet
	sink    io.Writer
}

type irqDelta struct {
	Timestamp time.Time `json:"timestamp"`
	Counters  Stats     `json:"counters"`
}

func (rj *reporterJSON) Delta(ts time.Time, prevStats, lastStats Stats) {
	if rj.verbose < 2 {
		return
	}
	res := irqDelta{
		Timestamp: ts,
		Counters:  countersForCPUs(rj.cpus, prevStats.Delta(lastStats)),
	}
	json.NewEncoder(rj.sink).Encode(res)
}

type irqwatchDuration struct {
	d time.Duration
}

type irqSummary struct {
	Elapsed  irqwatchDuration `json:"elapsed"`
	Counters Stats            `json:"counters"`
}

func (rj *reporterJSON) Summary(initTs time.Time, prevStats, lastStats Stats) {
	if rj.verbose < 1 {
		return
	}
	res := irqSummary{
		Elapsed: irqwatchDuration{
			d: time.Now().Sub(initTs),
		},
		Counters: countersForCPUs(rj.cpus, prevStats.Delta(lastStats)),
	}
	json.NewEncoder(rj.sink).Encode(res)
}

func countersForCPUs(cpus cpuset.CPUSet, stats Stats) Stats {
	res := make(Stats)
	cpuids := cpus.ToSlice()

	for _, cpuid := range cpuids {
		counter, ok := stats[cpuid]
		if !ok || len(counter) == 0 {
			continue
		}
		res[cpuid] = counter
	}

	return res
}
