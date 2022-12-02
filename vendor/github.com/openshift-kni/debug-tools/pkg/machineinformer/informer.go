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
 * Copyright 2021 Red Hat, Inc.
 */

package machineinformer

import (
	"encoding/json"
	"io"
	"time"

	infov1 "github.com/google/cadvisor/info/v1"
	"github.com/google/cadvisor/machine"
	"k8s.io/klog/v2"
)

type Handle struct {
	RootDirectory   string
	RawOutput       bool
	CleanTimestamp  bool
	CleanProcfsInfo bool
	Out             io.Writer
}

func (handle *Handle) Run() {
	var err error
	var info *infov1.MachineInfo

	info, err = GetRaw(handle.RootDirectory)
	if err != nil {
		klog.Fatalf("Cannot get machine info: %v")
	}

	if !handle.RawOutput {
		info = cleanInfo(info)
	}
	if handle.CleanProcfsInfo {
		info.NumPhysicalCores = 0
		info.CpuFrequency = 0
		info.MemoryCapacity = 0
	}
	if handle.CleanTimestamp {
		info.Timestamp = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
	}

	json.NewEncoder(handle.Out).Encode(info)
}

func GetRaw(root string) (*infov1.MachineInfo, error) {
	fsInfo := newFakeFsInfo()
	sysFs := NewRelocatableSysFs(root)
	inHostNamespace := true

	return machine.Info(sysFs, fsInfo, inHostNamespace)
}

func Get(root string) (*infov1.MachineInfo, error) {
	info, err := GetRaw(root)
	if err != nil {
		return nil, err
	}
	return cleanInfo(info), nil
}

func cleanInfo(in *infov1.MachineInfo) *infov1.MachineInfo {
	out := in.Clone()
	out.MachineID = ""
	out.SystemUUID = ""
	out.BootID = ""
	for i := 0; i < len(out.NetworkDevices); i++ {
		out.NetworkDevices[i].MacAddress = ""
	}
	out.CloudProvider = infov1.UnknownProvider
	out.InstanceType = infov1.UnknownInstance
	out.InstanceID = infov1.UnNamedInstance
	return out
}
