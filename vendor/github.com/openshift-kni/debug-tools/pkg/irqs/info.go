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
	"bufio"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	"github.com/openshift-kni/debug-tools/pkg/fswrap"
)

const (
	EffectiveAffinity = 1 << iota
)

// the IRQ name is not always a number, can be like "NMI", "TLB"...
type Counter map[string]uint64

// assume x is fresher than x. IRQ (all stems by how the kernel does the accounting):
// if a irq is counted in both c and x, then val(x) >= val(x)
// x is always a superset of c
func (C Counter) Delta(X Counter) Counter {
	R := make(Counter)
	for name, amount := range X {
		if delta := amount - C[name]; delta > 0 {
			R[name] = delta
		}
	}
	return R
}

func (C Counter) Clone() Counter {
	R := make(Counter)
	for k, v := range C {
		R[k] = v
	}
	return R
}

// CPUid -> counter
type Stats map[int]Counter

func (S Stats) Delta(X Stats) Stats {
	R := make(Stats)
	for cpuid, counter := range X {
		R[cpuid] = S[cpuid].Delta(counter)
	}
	return R
}

func (S Stats) Clone() Stats {
	R := make(Stats)
	for k, v := range S {
		R[k] = v.Clone()
	}
	return R
}

type Info struct {
	Source string
	IRQ    int
	CPUs   cpuset.CPUSet
}

type Handler struct {
	log        *log.Logger
	procfsRoot string
	fs         fswrap.FSWrapper
}

func New(logger *log.Logger, procfsRoot string) *Handler {
	return &Handler{
		log:        logger,
		procfsRoot: procfsRoot,
		fs:         fswrap.FSWrapper{Log: logger},
	}
}

func (handler *Handler) ReadInfo(flags uint) ([]Info, error) {
	// the best source of information here is man 5 procfs
	// and https://www.kernel.org/doc/Documentation/IRQ-affinity.txt
	irqRoot := filepath.Join(handler.procfsRoot, "irq")

	files, err := handler.fs.ReadDir(irqRoot)
	if err != nil {
		return nil, err
	}

	var irqs []int
	for _, file := range files {
		irq, err := strconv.Atoi(file.Name())
		if err != nil {
			continue // just skip not-irq-looking dirs
		}
		irqs = append(irqs, irq)
	}

	sort.Ints(irqs)

	affinityListFile := "smp_affinity_list"
	if (flags & EffectiveAffinity) == EffectiveAffinity {
		affinityListFile = "effective_affinity_list"
	}

	irqInfos := make([]Info, len(irqs))
	for _, irq := range irqs {
		irqDir := filepath.Join(irqRoot, fmt.Sprintf("%d", irq))

		irqCpuList, err := handler.fs.ReadFile(filepath.Join(irqDir, affinityListFile))
		if err != nil {
			return nil, err
		}

		irqCpus, err := cpuset.Parse(strings.TrimSpace(string(irqCpuList)))
		if err != nil {
			handler.log.Printf("Error parsing cpulist in %q: %v", irqCpuList, err)
			continue // keep running
		}

		irqInfos = append(irqInfos, Info{
			CPUs:   irqCpus,
			IRQ:    irq,
			Source: handler.findSourceForIRQ(irq),
		})
	}
	return irqInfos, nil
}

func (handler *Handler) ReadStats() (Stats, error) {
	src, err := handler.fs.Open(filepath.Join(handler.procfsRoot, "interrupts"))
	if err != nil {
		return nil, fmt.Errorf("error reading interrupts from %q: %v", handler.procfsRoot, err)
	}
	defer src.Close()
	return parseInterrupts(handler.log, src)
}

func parseInterrupts(logger *log.Logger, rd io.Reader) (Stats, error) {
	src := bufio.NewScanner(rd)
	src.Scan()
	cpus := strings.Fields(src.Text())

	// we should never assume columnid == cpuid, because we will need to handle offlined cpus
	// - aka holes in the sequence. Hence we use a mapping.
	col2cpu := make(map[int]int)

	stats := make(Stats)
	// we split the line using whitespaces as separator. So the first line is something like
	// "            CPU0       CPU1       CPU2       CPU3" (all spaces, no tabs)
	// and the `cpus` slice is something like ["CPU0" "CPU1" "CPU2" "CPU3"]
	for colIdx, cpu := range cpus {
		var cpuid int
		n, err := fmt.Sscanf(cpu, "CPU%d", &cpuid)
		if n != 1 || err != nil {
			return nil, fmt.Errorf("cannot parse cpu name %q: err=%v", cpu, err)
		}
		stats[cpuid] = make(Counter)
		// if all the cpus are online, this is the trivial mapping 0:0, 1:1, ...
		col2cpu[colIdx] = cpuid
	}

	// format:
	// IRQ: cpu0_counter ... cpuN_counter [stuff we dont care]
	// so we need to scan only the first len(cpus) + 1 columns
	maxCols := 1 + len(cpus)
	for src.Scan() {
		items := strings.Fields(src.Text())
		if len(items) < maxCols {
			// irq name == MIS
			continue
		}
		irqName := strings.TrimSuffix(items[0], ":")
		// so from now on we consider only len(cpus) columns, shifted by one to the left
		//                [ "0:" "13" "0" "0" "0" "IR-IO-APIC" "2-edge" "timer"]
		// column index:    0    1    2   3   4   5            6        7
		//                  |----+                ============================ we don't care about this
		//                       |----|---|---|
		//                                    `- we only care about 1 + len(cpus) = 4 = 5 columns
		// `cpuColIdx`:    {skip} 0   1   2   3
		for cpuColIdx, item := range items[1:maxCols] {
			count, err := strconv.ParseUint(item, 10, 64)
			if err != nil {
				log.Printf("Error parsing interrupts info from %q: %v", item, err)
				continue
			}

			cpuId := col2cpu[cpuColIdx]
			stats[cpuId][irqName] = count
		}
	}
	return stats, nil
}

// TODO: we may want to crosscorrelate with `/proc/interrupts, which always give a valid (!= "") source
func (handler *Handler) findSourceForIRQ(irq int) string {
	irqDir := filepath.Join(handler.procfsRoot, "irq", fmt.Sprintf("%d", irq))
	files, err := handler.fs.ReadDir(irqDir)
	if err != nil {
		handler.log.Printf("Error reading %q: %v", irqDir, err)
		return "MISSING"
	}
	for _, file := range files {
		if file.IsDir() {
			return file.Name()
		}
	}
	handler.log.Printf("Cannot find source for irq %d", irq)
	return ""
}
