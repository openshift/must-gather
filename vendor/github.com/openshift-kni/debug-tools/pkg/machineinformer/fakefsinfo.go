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
	"fmt"

	"github.com/google/cadvisor/fs"
)

type fakeFsInfo struct {
	notImplemented error
}

func newFakeFsInfo() fs.FsInfo {
	return fakeFsInfo{
		notImplemented: fmt.Errorf("not implemented"),
	}
}

func (ffi fakeFsInfo) GetGlobalFsInfo() ([]fs.Fs, error) {
	return nil, ffi.notImplemented
}

func (ffi fakeFsInfo) GetFsInfoForPath(mountSet map[string]struct{}) ([]fs.Fs, error) {
	return nil, ffi.notImplemented
}

func (ffi fakeFsInfo) GetDirUsage(dir string) (fs.UsageInfo, error) {
	return fs.UsageInfo{}, ffi.notImplemented
}

func (ffi fakeFsInfo) GetDeviceInfoByFsUUID(uuid string) (*fs.DeviceInfo, error) {
	return nil, ffi.notImplemented
}

func (ffi fakeFsInfo) GetDirFsDevice(dir string) (*fs.DeviceInfo, error) {
	return nil, ffi.notImplemented
}

func (ffi fakeFsInfo) GetDeviceForLabel(label string) (string, error) {
	return "", ffi.notImplemented
}

func (ffi fakeFsInfo) GetLabelsForDevice(device string) ([]string, error) {
	return nil, ffi.notImplemented
}

func (ffi fakeFsInfo) GetMountpointForDevice(device string) (string, error) {
	return "", ffi.notImplemented
}
