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

package soft

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/openshift-kni/debug-tools/pkg/fswrap"
)

// presented in kernel order
func Names() []string {
	// keys found in /proc/softirqs (first column)
	return []string{"HI", "TIMER", "NET_TX", "NET_RX", "BLOCK", "IRQ_POLL", "TASKLET", "SCHED", "HRTIMER", "RCU"}
}

type Info struct {
	CPUs int
	// softirq -> percpu count
	Counters map[string][]uint64
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

func (handler *Handler) ReadInfo() (*Info, error) {
	src, err := handler.fs.Open(filepath.Join(handler.procfsRoot, "softirqs"))
	if err != nil {
		return nil, fmt.Errorf("error reading softirqs from %q: %v", handler.procfsRoot, err)
	}
	defer src.Close()
	return parseSoftirqs(handler.log, src)
}

func parseSoftirqs(logger *log.Logger, rd io.Reader) (*Info, error) {
	src := bufio.NewScanner(rd)
	src.Scan()
	cpus := strings.Fields(src.Text())
	ret := Info{
		CPUs:     len(cpus),
		Counters: make(map[string][]uint64),
	}

	for src.Scan() {
		items := strings.Fields(src.Text())
		var vals []uint64
		for _, item := range items[1:] {
			v, err := strconv.ParseUint(item, 10, 64)
			if err != nil {
				log.Printf("Error parsing softirqs info from %q: %v", item, err)
				continue
			}
			vals = append(vals, v)
		}
		ret.Counters[strings.TrimSuffix(items[0], ":")] = vals
	}
	return &ret, nil
}
