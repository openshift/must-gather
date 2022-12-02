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

package procs

import (
	"bufio"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"

	"github.com/openshift-kni/debug-tools/pkg/fswrap"
)

type TIDInfo struct {
	Tid      int    `json:"tid"`
	Name     string `json:"name"`
	Affinity []int  `json:"affinity"`
}

type PIDInfo struct {
	Pid  int             `json:"pid"`
	Name string          `json:"name"`
	TIDs map[int]TIDInfo `json:"threads"`
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

func (handler *Handler) ListAll() (map[int]PIDInfo, error) {
	infos := make(map[int]PIDInfo)
	pidEntries, err := handler.fs.ReadDir(handler.procfsRoot)
	if err != nil {
		return infos, err
	}

	for _, pidEntry := range pidEntries {
		if !pidEntry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(pidEntry.Name())
		if err != nil {
			// doesn't look like a pid
			continue
		}

		pidInfo, err := handler.FromPID(pid)
		if err != nil {
			handler.log.Printf("Error extracting PID info from %d: %v", pid, err)
			continue
		}

		infos[pid] = pidInfo
	}
	return infos, nil
}

func (handler *Handler) FromPID(pid int) (PIDInfo, error) {
	pidInfo := PIDInfo{
		Pid:  pid,
		TIDs: make(map[int]TIDInfo),
	}

	procName, err := handler.readProcessName(pid)
	if err == nil {
		pidInfo.Name = fixFilename(procName)
	} else {
		// failures are not critical
		handler.log.Printf("Error reading process name for pid %d: %v", pid, err)
	}

	tasksDir := filepath.Join(handler.procfsRoot, procEntry(pid), "task")
	tidEntries, err := handler.fs.ReadDir(tasksDir)
	if err != nil {
		handler.log.Printf("Error reading the tasks %q for %d: %v", tasksDir, pid, err)
		return pidInfo, err
	}

	for _, tidEntry := range tidEntries {
		// TODO: use x/sys/unix schedGetAffinity?
		info, err := handler.parseProcStatus(filepath.Join(tasksDir, tidEntry.Name(), "status"))
		if err != nil {
			// failures are not critical
			handler.log.Printf("Error parsing status for pid %d tid %s: %v", pid, tidEntry.Name(), err)
			continue
		}
		pidInfo.TIDs[info.Tid] = info
	}

	return pidInfo, nil
}

func (handler *Handler) parseProcStatus(path string) (TIDInfo, error) {
	info := TIDInfo{}
	file, err := handler.fs.Open(path)
	if err != nil {
		return info, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		// see /proc/self/status to check the file content
		line := scanner.Text()
		if strings.HasPrefix(line, "Name:") {
			items := strings.SplitN(line, ":", 2)
			info.Name = strings.TrimSpace(items[1])
		}
		if strings.HasPrefix(line, "Pid:") {
			items := strings.SplitN(line, ":", 2)
			pid, err := strconv.Atoi(strings.TrimSpace(items[1]))
			if err != nil {
				return info, err
			}
			// yep, the field is called "pid" even if we are scaning /proc/XXX/task/YYY/status
			info.Tid = pid
		}
		if strings.HasPrefix(line, "Cpus_allowed_list:") {
			items := strings.SplitN(line, ":", 2)
			cpuIDs, err := cpuset.Parse(strings.TrimSpace(items[1]))
			if err != nil {
				return info, err
			}
			info.Affinity = cpuIDs.ToSlice()
		}
	}

	return info, scanner.Err()
}

func (handler *Handler) readProcessName(pid int) (string, error) {
	data, err := handler.fs.ReadFile(filepath.Join(handler.procfsRoot, procEntry(pid), "cmdline"))
	if err != nil {
		return "", err
	}
	cmdline := string(data)
	// workaround for running on developer laptops, on which the slack process seems to
	// unexpectedly use " " as non standard separator.
	if off := strings.Index(cmdline, " "); off > 0 {
		cmdline = cmdline[:off]
	}
	if off := strings.Index(cmdline, "\x00"); off > 0 {
		return cmdline[:off], nil
	}
	return cmdline, nil
}

func fixFilename(filename string) string {
	name := filepath.Base(filename)
	if name == "." {
		return ""
	}
	return strings.Trim(name, "\x00")
}

func procEntry(pid int) string {
	if pid == 0 {
		return "self"
	}
	return fmt.Sprintf("%d", pid)
}
