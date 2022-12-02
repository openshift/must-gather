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
 * Copyright 2022 Red Hat, Inc.
 */
package k8s

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"text/template"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/openshift-kni/debug-tools/pkg/knit/cmd"
	"github.com/spf13/cobra"
)

type podInfoOptions struct {
	nodeName string
}

//Only need some info about the pod.
// Right now is:
// - pod name
// - pod namespace
// - node name
// - status.qosClass
// - containers
//   - requests cpu
//   - limits cpu
// Note this output format could change but it would be parsed on insight rules
// so the change should be sync with it.
// Caution: We filter the data from pods to avoid exposing sensible information
// (like environment variables or input parameters which can contain passwords)
// so take care of that when changing this template.
const defaultTemplate string = `
[
	{{- range $idx, $item := .Items}}
	{{- if (ne $idx 0)}},{{end}}
	{
		"namespace":"{{.ObjectMeta.Namespace}}", 
		"name":"{{.ObjectMeta.Name}}", 
		"nodeName":"{{.Spec.NodeName}}", 
		"qosClass": "{{.Status.QOSClass}}", 
		{{- if .Spec.Containers }}
		"containers": [
		{{- range $cdx, $cont := .Spec.Containers -}}
			{{- if (ne $cdx 0) }},{{ end }} 
			{
				"name":"{{.Name}}"
				{{- if or .Resources.Requests .Resources.Limits -}}
				,
				"resources": {
					{{- if .Resources.Limits}} {{if .Resources.Limits.Cpu}} 
					"limits": {
						"cpu": "{{.Resources.Limits.Cpu}}"
					}
					{{- end -}}{{end}}
					{{- if and .Resources.Requests .Resources.Limits .Resources.Requests.Cpu .Resources.Limits.Cpu -}}
					,
					{{- end -}}
					{{- if .Resources.Requests}}{{if .Resources.Requests.Cpu }}
					"requests": {
						"cpu": "{{.Resources.Requests.Cpu}}"
					}
					{{- end }}{{end}}
				} 
				{{- end }} 
			} 
		{{- end }} 
		]
		{{- end }} 
	}
	{{- end }}
]`

func NewPodInfoCommand(knitOpts *cmd.KnitOptions) *cobra.Command {

	opts := &podInfoOptions{}
	podInfo := &cobra.Command{
		Use:   "podinfo",
		Short: "get pod information complementing podresources data",
		RunE: func(cmd *cobra.Command, args []string) error {

			clientset, err := getClientSetFromClusterConfig()
			if err != nil {
				return fmt.Errorf("unable to get clientset: %w", err)
			}

			podInfoTemplate, err := createOutputTemplate("pod_info", defaultTemplate)
			if err != nil {
				return fmt.Errorf("unable to get output template: %w", err)
			}

			nodeFieldSelector := buildNodeFieldSelector(opts.nodeName)

			return showPodInfo(nodeFieldSelector, clientset, podInfoTemplate, os.Stdout)
		},
	}

	podInfo.Flags().StringVar(&opts.nodeName, "node-name", "", "node name to get pod info from.")

	return podInfo
}

// getKubeConfig creates a *rest.Config for talking to a Kubernetes apiserver.
//
// Config precedence:
// - KUBECONFIG environment variable pointing at a file
// - In-cluster config if running in cluster
// - $HOME/.kube/config if exists
//
// note: Use same precedence as controller-runtime GetConfig.
// see: https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/client/config#GetConfig
func getKubeConfig() (*rest.Config, error) {
	kubeconfigFromFilePath := func(kubeConfigFilePath string) (*rest.Config, error) {
		if _, err := os.Stat(kubeConfigFilePath); err != nil {
			return nil, fmt.Errorf("cannot stat kubeconfig '%s'", kubeConfigFilePath)
		}
		return clientcmd.BuildConfigFromFlags("", kubeConfigFilePath)
	}

	// If an env variable is specified with the config location, use that
	kubeConfig := os.Getenv("KUBECONFIG")
	if len(kubeConfig) > 0 {
		return kubeconfigFromFilePath(kubeConfig)
	}

	// try the in-cluster config
	if c, err := rest.InClusterConfig(); err == nil {
		return c, nil
	}

	// try the default location in the user's home directory
	if usr, err := user.Current(); err == nil {
		kubeConfig := filepath.Join(usr.HomeDir, ".kube", "config")
		return kubeconfigFromFilePath(kubeConfig)
	}

	return nil, fmt.Errorf("could not locate a kubeconfig")
}

func getClientSetFromClusterConfig() (kubernetes.Interface, error) {

	config, err := getKubeConfig()
	if err != nil {
		return nil, err
	}
	// creates the clientset
	return kubernetes.NewForConfig(config)
}

func createOutputTemplate(name string, tmplStr string) (*template.Template, error) {
	podInfoTemplate, err := template.New(name).Parse(tmplStr)
	if err != nil {
		return nil, err
	}
	return podInfoTemplate, nil
}

func buildNodeFieldSelector(nodeName string) string {
	fieldSelector := ""
	if len(nodeName) != 0 {
		fieldSelector = fmt.Sprintf("spec.nodeName=%s,", nodeName)
	}
	fieldSelector += "status.phase=Running"

	return fieldSelector
}

func showPodInfo(nodeFieldSelector string, clientset kubernetes.Interface, podInfoTemplate *template.Template, output io.Writer) error {

	if nil == podInfoTemplate {
		return fmt.Errorf("wrong incoming params: need an output template")
	}

	listOptions := metav1.ListOptions{
		FieldSelector: nodeFieldSelector,
	}
	// get pods in all the namespaces by omitting namespace
	// Or specify namespace to get pods in particular namespace
	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), listOptions)
	if err != nil {
		return fmt.Errorf("error while getting pods list: %w", err)
	}

	if err := podInfoTemplate.Execute(output, pods); err != nil {
		return fmt.Errorf("error while trying to format output: %w", err)
	}

	return nil
}
