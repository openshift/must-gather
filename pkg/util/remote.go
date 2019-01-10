package util

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	clientv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"

	"strings"

	"path"

	"github.com/openshift/must-gather/pkg/util/term"
)

type remoteExecutor struct{}

func (*remoteExecutor) Execute(method string, url *url.URL, config *restclient.Config, stdin io.Reader, stdout, stderr io.Writer, tty bool, terminalSizeQueue remotecommand.TerminalSizeQueue) error {
	exec, err := remotecommand.NewSPDYExecutor(config, method, url)
	if err != nil {
		return err
	}

	return exec.Stream(remotecommand.StreamOptions{
		Stdin:             stdin,
		Stdout:            stdout,
		Stderr:            stderr,
		Tty:               tty,
		TerminalSizeQueue: terminalSizeQueue,
	})
}

func ExecCommandInPod(adminConfig *restclient.Config, pod *corev1.Pod, command []string) (string, error) {
	cmdOutput := bytes.NewBuffer(nil)
	cmdErr := bytes.NewBuffer(nil)

	if len(pod.Spec.Containers) == 0 {
		return "", fmt.Errorf("no containers found")
	}

	t := term.TTY{
		Out: os.Stdout,
	}
	if err := t.Safe(func() error {
		restClient, err := clientv1.NewForConfig(adminConfig)
		if err != nil {
			return err
		}

		containerName := pod.Spec.Containers[0].Name
		req := restClient.RESTClient().Post().
			Resource("pods").
			Name(pod.GetName()).
			Namespace(pod.GetNamespace()).
			SubResource("exec").
			Param("container", containerName)
		req.VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command:   command,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

		var sizeQueue remotecommand.TerminalSizeQueue
		executor := &remoteExecutor{}
		return executor.Execute("POST", req.URL(), adminConfig, nil, cmdOutput, cmdErr, false, sizeQueue)
	}); err != nil {
		return "", fmt.Errorf("%v: %v", cmdErr.String(), err)
	}

	return cmdOutput.String(), nil
}

type RemotePodURLGetter struct {
	Token string

	Protocol string
	Host     string
	Port     string
}

func (c *RemotePodURLGetter) Get(urlPath string, pod *corev1.Pod, config *restclient.Config) (string, error) {
	url := strings.Join([]string{c.Protocol, "://", c.Host, ":", c.Port, path.Join("/", urlPath)}, "")
	command := []string{"/bin/curl", "-k", url}
	if len(c.Token) > 0 {
		command = []string{"/bin/curl", "-H", fmt.Sprintf("%s", "Authorization: Bearer "+c.Token), "-k", url}

	}
	return ExecCommandInPod(config, pod, command)
}
