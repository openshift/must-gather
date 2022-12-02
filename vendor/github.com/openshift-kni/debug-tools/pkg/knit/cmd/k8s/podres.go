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

package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	kubeletpodresourcesv1 "k8s.io/kubelet/pkg/apis/podresources/v1"
	"k8s.io/kubernetes/pkg/kubelet/apis/podresources"

	"github.com/openshift-kni/debug-tools/pkg/knit/cmd"
)

// see k/k/test/e2e_node/util.go
// TODO: make these options
const (
	defaultSocketPath = "unix:///var/lib/kubelet/pod-resources/kubelet.sock"

	defaultPodResourcesTimeout = 10 * time.Second
	defaultPodResourcesMaxSize = 1024 * 1024 * 16 // 16 Mb
)

const (
	apiCallList           = "list"
	apiCallGetAllocatable = "get-allocatable"
)

type podResOptions struct {
	socketPath string
}

func NewPodResourcesCommand(knitOpts *cmd.KnitOptions) *cobra.Command {
	opts := &podResOptions{}
	podRes := &cobra.Command{
		Use:   "podres",
		Short: "show currently allocated pod resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			return showPodResources(cmd, opts, args)
		},
		Args: cobra.MaximumNArgs(1),
	}
	podRes.Flags().StringVarP(&opts.socketPath, "socket-path", "R", defaultSocketPath, "podresources API socket path.")
	return podRes
}

// we fill our own structs to avoid the problem when default int value(0) removed from the json
func selectAction(apiName string) (func(cli kubeletpodresourcesv1.PodResourcesListerClient) error, error) {
	if apiName == apiCallList {
		return func(cli kubeletpodresourcesv1.PodResourcesListerClient) error {
			resp, err := cli.List(context.TODO(), &kubeletpodresourcesv1.ListPodResourcesRequest{})
			if err != nil {
				return err
			}

			listPodResourcesResp := getListPodResourcesResponse(resp)
			if err := json.NewEncoder(os.Stdout).Encode(listPodResourcesResp); err != nil {
				return err
			}

			return nil
		}, nil
	}
	if apiName == apiCallGetAllocatable {
		return func(cli kubeletpodresourcesv1.PodResourcesListerClient) error {
			resp, err := cli.GetAllocatableResources(context.TODO(), &kubeletpodresourcesv1.AllocatableResourcesRequest{})
			if err != nil {
				return err
			}

			allocatableResourcesResponse := getAllocatableResourcesResponse(resp)
			if err := json.NewEncoder(os.Stdout).Encode(allocatableResourcesResponse); err != nil {
				return err
			}

			return nil
		}, nil
	}
	return func(cli kubeletpodresourcesv1.PodResourcesListerClient) error {
		return nil
	}, fmt.Errorf("unknown API %q", apiName)
}

func showPodResources(cmd *cobra.Command, opts *podResOptions, args []string) error {
	apiName := "list"
	if len(args) == 1 {
		apiName = args[0]
	}

	action, err := selectAction(apiName)
	if err != nil {
		return err
	}

	cli, conn, err := podresources.GetV1Client(opts.socketPath, defaultPodResourcesTimeout, defaultPodResourcesMaxSize)
	if err != nil {
		return err
	}
	defer conn.Close()

	return action(cli)
}

func getListPodResourcesResponse(resp *kubeletpodresourcesv1.ListPodResourcesResponse) *ListPodResourcesResponse {
	var podResources []*PodResources
	for _, podRes := range resp.PodResources {
		var podResContainers []*ContainerResources
		for _, c := range podRes.Containers {
			podResContainers = append(podResContainers, &ContainerResources{
				Name:    c.Name,
				CpuIds:  c.CpuIds,
				Devices: getDevices(c.Devices),
				Memory:  getMemory(c.Memory),
			})
		}

		podResources = append(podResources, &PodResources{
			Name:       podRes.Name,
			Namespace:  podRes.Namespace,
			Containers: podResContainers,
		})
	}

	return &ListPodResourcesResponse{
		PodResources: podResources,
	}
}

func getAllocatableResourcesResponse(resp *kubeletpodresourcesv1.AllocatableResourcesResponse) *AllocatableResourcesResponse {
	return &AllocatableResourcesResponse{
		CpuIds:  resp.CpuIds,
		Devices: getDevices(resp.Devices),
		Memory:  getMemory(resp.Memory),
	}
}

func getDevices(containerDevices []*kubeletpodresourcesv1.ContainerDevices) []*ContainerDevices {
	var cDevices []*ContainerDevices
	for _, d := range containerDevices {
		deviceTopologyInfo := getTopologyInfo(d.Topology)
		cDevices = append(cDevices, &ContainerDevices{
			ResourceName: d.ResourceName,
			DeviceIds:    d.DeviceIds,
			Topology:     deviceTopologyInfo,
		})
	}

	return cDevices
}

func getMemory(containerMemory []*kubeletpodresourcesv1.ContainerMemory) []*ContainerMemory {
	var cMemory []*ContainerMemory
	for _, m := range containerMemory {
		memoryTopologyInfo := getTopologyInfo(m.Topology)
		cMemory = append(cMemory, &ContainerMemory{
			MemoryType: m.MemoryType,
			Size_:      m.Size_,
			Topology:   memoryTopologyInfo,
		})
	}

	return cMemory
}

func getTopologyInfo(topologyInfo *kubeletpodresourcesv1.TopologyInfo) *TopologyInfo {
	if topologyInfo == nil {
		return nil
	}

	var numaNodes []*NUMANode
	for _, numaNode := range topologyInfo.Nodes {
		numaNodeID := numaNode.ID
		numaNodes = append(numaNodes, &NUMANode{
			ID: &numaNodeID,
		})
	}

	return &TopologyInfo{
		Nodes: numaNodes,
	}
}
