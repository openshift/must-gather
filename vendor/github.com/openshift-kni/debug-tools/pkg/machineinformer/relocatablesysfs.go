// Copyright 2014 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// derived from https://github.com/google/cadvisor/blob/master/utils/sysfs/sysfs.go @ ef7e64f9
// as Apache 2.0 license allows.

package machineinformer

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"k8s.io/klog/v2"

	"github.com/google/cadvisor/utils/sysfs"

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

const (
	blockDir     = "/block"
	cacheDir     = "/devices/system/cpu/cpu"
	netDir       = "/class/net"
	dmiDir       = "/class/dmi"
	ppcDevTree   = "/proc/device-tree"
	s390xDevTree = "/etc" // s390/s390x changes

	meminfoFile = "meminfo"

	sysFsCPUTopology = "topology"

	// CPUPhysicalPackageID is a physical package id of cpu#. Typically corresponds to a physical socket number,
	// but the actual value is architecture and platform dependent.
	CPUPhysicalPackageID = "physical_package_id"
	// CPUCoreID is the CPU core ID of cpu#. Typically it is the hardware platform's identifier
	// (rather than the kernel's). The actual value is architecture and platform dependent.
	CPUCoreID = "core_id"

	coreIDFilePath    = "/" + sysFsCPUTopology + "/core_id"
	packageIDFilePath = "/" + sysFsCPUTopology + "/physical_package_id"

	// memory size calculations

	cpuDirPattern  = "cpu*[0-9]"
	nodeDirPattern = "node*[0-9]"

	//HugePagesNrFile name of nr_hugepages file in sysfs
	HugePagesNrFile = "nr_hugepages"

	nodeDir = "/devices/system/node/"
)

// RelocatableSysFs allows to consume a sysfs tree whose root is not `/sys`.
// this means we can easily consume a syfs snapshot from another node, for troubleshoot
// and debugging purposes, without complex/cumbersome chroot dances or even more complicated,
// without containerizing the test session.
// the cadvisor's SysFS interface is not formalized nor very clean, so we add the prefix
// in few key place discovered using tests and reviewing how the API is used.
type RelocatableSysFs struct {
	root string
}

func NewRelocatableSysFs(root string) sysfs.SysFs {
	return RelocatableSysFs{
		root: root,
	}
}

func (fs RelocatableSysFs) GetNodesPaths() ([]string, error) {
	pathPattern := filepath.Join(fs.root, nodeDir, nodeDirPattern)
	return filepath.Glob(pathPattern)
}

func (fs RelocatableSysFs) GetCPUsPaths(cpusPath string) ([]string, error) {
	pathPattern := filepath.Join(cpusPath, cpuDirPattern)
	return filepath.Glob(pathPattern)
}

func (fs RelocatableSysFs) GetCoreID(cpuPath string) (string, error) {
	// intentionally not prepending fs.root, because this function
	// is expected to be used with `cpuPath` as returned by
	// GetCPUsPaths
	coreIDFilePath := filepath.Join(cpuPath, coreIDFilePath)
	coreID, err := ioutil.ReadFile(coreIDFilePath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(coreID)), err
}

func (fs RelocatableSysFs) GetCPUPhysicalPackageID(cpuPath string) (string, error) {
	// intentionally not prepending fs.root, because this function
	// is expected to be used with `cpuPath` as returned by
	// GetCPUsPaths
	packageIDFilePath := filepath.Join(cpuPath, packageIDFilePath)
	packageID, err := ioutil.ReadFile(packageIDFilePath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(packageID)), err
}

func (fs RelocatableSysFs) GetMemInfo(nodePath string) (string, error) {
	meminfoPath := filepath.Join(nodePath, meminfoFile)
	meminfo, err := ioutil.ReadFile(meminfoPath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(meminfo)), err
}

func (fs RelocatableSysFs) GetHugePagesInfo(hugePagesDirectory string) ([]os.FileInfo, error) {
	return ioutil.ReadDir(filepath.Join(fs.root, hugePagesDirectory))
}

func (fs RelocatableSysFs) GetHugePagesNr(hugepagesDirectory string, hugePageName string) (string, error) {
	hugePageFilePath := filepath.Join(fs.root, hugepagesDirectory, hugePageName, HugePagesNrFile)
	hugePageFile, err := ioutil.ReadFile(hugePageFilePath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(hugePageFile)), err
}

func (fs RelocatableSysFs) GetBlockDevices() ([]os.FileInfo, error) {
	return ioutil.ReadDir(filepath.Join(fs.root, blockDir))
}

func (fs RelocatableSysFs) GetBlockDeviceNumbers(name string) (string, error) {
	dev, err := ioutil.ReadFile(filepath.Join(fs.root, blockDir, name, "/dev"))
	if err != nil {
		return "", err
	}
	return string(dev), nil
}

func (fs RelocatableSysFs) GetBlockDeviceScheduler(name string) (string, error) {
	sched, err := ioutil.ReadFile(filepath.Join(fs.root, blockDir, name, "/queue/scheduler"))
	if err != nil {
		return "", err
	}
	return string(sched), nil
}

func (fs RelocatableSysFs) GetBlockDeviceSize(name string) (string, error) {
	size, err := ioutil.ReadFile(filepath.Join(fs.root, blockDir, name, "/size"))
	if err != nil {
		return "", err
	}
	return string(size), nil
}

func (fs RelocatableSysFs) GetNetworkDevices() ([]os.FileInfo, error) {
	files, err := ioutil.ReadDir(filepath.Join(fs.root, netDir))
	if err != nil {
		return nil, err
	}

	// Filter out non-directory & non-symlink files
	var dirs []os.FileInfo
	for _, f := range files {
		if f.Mode()|os.ModeSymlink != 0 {
			f, err = os.Stat(filepath.Join(fs.root, netDir, f.Name()))
			if err != nil {
				continue
			}
		}
		if f.IsDir() {
			dirs = append(dirs, f)
		}
	}
	return dirs, nil
}

func (fs RelocatableSysFs) GetNetworkAddress(name string) (string, error) {
	address, err := ioutil.ReadFile(filepath.Join(fs.root, netDir, name, "/address"))
	if err != nil {
		return "", err
	}
	return string(address), nil
}

func (fs RelocatableSysFs) GetNetworkMtu(name string) (string, error) {
	mtu, err := ioutil.ReadFile(filepath.Join(fs.root, netDir, name, "/mtu"))
	if err != nil {
		return "", err
	}
	return string(mtu), nil
}

func (fs RelocatableSysFs) GetNetworkSpeed(name string) (string, error) {
	speed, err := ioutil.ReadFile(filepath.Join(fs.root, netDir, name, "/speed"))
	if err != nil {
		return "", err
	}
	return string(speed), nil
}

func (fs RelocatableSysFs) GetNetworkStatValue(dev string, stat string) (uint64, error) {
	statPath := filepath.Join(fs.root, netDir, dev, "/statistics", stat)
	out, err := ioutil.ReadFile(statPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read stat from %q for device %q", statPath, dev)
	}
	var s uint64
	n, err := fmt.Sscanf(string(out), "%d", &s)
	if err != nil || n != 1 {
		return 0, fmt.Errorf("could not parse value from %q for file %s", string(out), statPath)
	}
	return s, nil
}

func (fs RelocatableSysFs) GetCaches(id int) ([]os.FileInfo, error) {
	cpuPath := filepath.Join(fs.root, fmt.Sprintf("%s%d/cache", cacheDir, id))
	return ioutil.ReadDir(cpuPath)
}

func bitCount(i uint64) (count int) {
	for i != 0 {
		if i&1 == 1 {
			count++
		}
		i >>= 1
	}
	return
}

func getCPUCount(cache string) (count int, err error) {
	out, err := ioutil.ReadFile(filepath.Join(cache, "/shared_cpu_map"))
	if err != nil {
		return 0, err
	}
	masks := strings.Split(string(out), ",")
	for _, mask := range masks {
		// convert hex string to uint64
		m, err := strconv.ParseUint(strings.TrimSpace(mask), 16, 64)
		if err != nil {
			return 0, fmt.Errorf("failed to parse cpu map %q: %v", string(out), err)
		}
		count += bitCount(m)
	}
	return
}

func (fs RelocatableSysFs) GetCacheInfo(id int, name string) (sysfs.CacheInfo, error) {
	cachePath := filepath.Join(fs.root, fmt.Sprintf("%s%d/cache/%s", cacheDir, id, name))
	out, err := ioutil.ReadFile(filepath.Join(cachePath, "/size"))
	if err != nil {
		return sysfs.CacheInfo{}, err
	}
	var size uint64
	n, err := fmt.Sscanf(string(out), "%dK", &size)
	if err != nil || n != 1 {
		return sysfs.CacheInfo{}, err
	}
	// convert to bytes
	size = size * 1024
	out, err = ioutil.ReadFile(filepath.Join(cachePath, "/level"))
	if err != nil {
		return sysfs.CacheInfo{}, err
	}
	var level int
	n, err = fmt.Sscanf(string(out), "%d", &level)
	if err != nil || n != 1 {
		return sysfs.CacheInfo{}, err
	}

	out, err = ioutil.ReadFile(filepath.Join(cachePath, "/type"))
	if err != nil {
		return sysfs.CacheInfo{}, err
	}
	cacheType := strings.TrimSpace(string(out))
	cpuCount, err := getCPUCount(cachePath)
	if err != nil {
		return sysfs.CacheInfo{}, err
	}
	return sysfs.CacheInfo{
		Size:  size,
		Level: level,
		Type:  cacheType,
		Cpus:  cpuCount,
	}, nil
}

func (fs RelocatableSysFs) GetSystemUUID() (string, error) {
	if id, err := ioutil.ReadFile(filepath.Join(fs.root, dmiDir, "id", "product_uuid")); err == nil {
		return strings.TrimSpace(string(id)), nil
	} else if id, err = ioutil.ReadFile(filepath.Join(fs.root, ppcDevTree, "system-id")); err == nil {
		return strings.TrimSpace(strings.TrimRight(string(id), "\000")), nil
	} else if id, err = ioutil.ReadFile(filepath.Join(fs.root, ppcDevTree, "vm,uuid")); err == nil {
		return strings.TrimSpace(strings.TrimRight(string(id), "\000")), nil
	} else if id, err = ioutil.ReadFile(filepath.Join(fs.root, s390xDevTree, "machine-id")); err == nil {
		return strings.TrimSpace(string(id)), nil
	} else {
		return "", err
	}
}

func (fs RelocatableSysFs) IsCPUOnline(cpuPath string) bool {
	onlinePath, err := filepath.Abs(filepath.Join(fs.root, cpuPath+"/../online"))
	if err != nil {
		klog.V(1).Infof("Unable to get absolute path for %s", cpuPath)
		return false
	}

	// Quick check to determine if file exists: if it does not then kernel CPU hotplug is disabled and all CPUs are online.
	_, err = os.Stat(onlinePath)
	if err != nil && os.IsNotExist(err) {
		return true
	}
	if err != nil {
		klog.V(1).Infof("Unable to stat %s: %s", onlinePath, err)
	}

	cpuID, err := getCPUID(cpuPath)
	if err != nil {
		klog.V(1).Infof("Unable to get CPU ID from path %s: %s", cpuPath, err)
		return false
	}

	isOnline, err := isCPUOnline(onlinePath, cpuID)
	if err != nil {
		klog.V(1).Infof("Unable to get online CPUs list: %s", err)
		return false
	}
	return isOnline
}

func getCPUID(dir string) (uint16, error) {
	regex := regexp.MustCompile("cpu([0-9]+)")
	matches := regex.FindStringSubmatch(dir)
	if len(matches) == 2 {
		id, err := strconv.Atoi(matches[1])
		if err != nil {
			return 0, err
		}
		return uint16(id), nil
	}
	return 0, fmt.Errorf("can't get CPU ID from %s", dir)
}

func isCPUOnline(path string, cpuID uint16) (bool, error) {
	fileContent, err := ioutil.ReadFile(path)
	if err != nil {
		return false, err
	}
	if len(fileContent) == 0 {
		return false, fmt.Errorf("%s found to be empty", path)
	}

	cpus, err := cpuset.Parse(strings.TrimSpace(string(fileContent)))
	if err != nil {
		return false, err
	}

	for _, cpu := range cpus.ToSlice() {
		if uint16(cpu) == cpuID {
			return true, nil
		}
	}
	return false, nil
}
