package util

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kapierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	clientv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/openshift/must-gather/pkg/util/term"
)

var (
	bearerTokenCreationTimeout = 10 * time.Second
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

	bearerTokenTask *bearerTokenGetter
}

func (c *RemotePodURLGetter) Get(urlPath string, pod *corev1.Pod, config *restclient.Config) (string, error) {
	url := strings.Join([]string{c.Protocol, "://", c.Host, ":", c.Port, path.Join("/", urlPath)}, "")
	command := []string{"/bin/curl", "-k", url}
	if len(c.Token) > 0 {
		command = []string{"/bin/curl", "-H", fmt.Sprintf("%s", "Authorization: Bearer "+c.Token), "-k", url}

	}
	return ExecCommandInPod(config, pod, command)
}

// FetchToken triggers token gathering without having to make a request
func (c *RemotePodURLGetter) FetchToken(config *restclient.Config) error {
	if len(c.Token) > 0 {
		return nil
	}
	if c.bearerTokenTask == nil {
		c.bearerTokenTask = &bearerTokenGetter{}
	}
	if c.bearerTokenTask.finished {
		c.Token = c.bearerTokenTask.tokenStore
		return nil
	}
	c.bearerTokenTask.onTokenObtained(func(err error) {
		if err != nil {
			return
		}
		c.Token = c.bearerTokenTask.tokenStore
	})
	if c.bearerTokenTask.started {
		return nil
	}

	// token task not yet started, start it
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	c.bearerTokenTask.Start(client)
	return nil
}

// EnsureGetWithTokenAsync retrieves the specified URL path once a token is obtained
func (c *RemotePodURLGetter) EnsureGetWithTokenAsync(urlPath string, pod *corev1.Pod, config *restclient.Config, onRequestMade func(string, error)) error {
	if len(c.Token) > 0 {
		result, err := c.Get(urlPath, pod, config)
		onRequestMade(result, err)
		return nil
	}

	if c.bearerTokenTask == nil {
		c.bearerTokenTask = &bearerTokenGetter{}
	}
	if c.bearerTokenTask.finished {
		c.Token = c.bearerTokenTask.tokenStore
		result, err := c.Get(urlPath, pod, config)
		onRequestMade(result, err)
		return nil
	}

	c.bearerTokenTask.onTokenObtained(func(err error) {
		if err != nil {
			onRequestMade("", err)
			return
		}

		c.Token = c.bearerTokenTask.tokenStore
		result, err := c.Get(urlPath, pod, config)
		onRequestMade(result, err)
	})
	if c.bearerTokenTask.started {
		// in progress, but not yet finished, leave task in queue
		return nil
	}

	// token task not yet started, start it
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	c.bearerTokenTask.Start(client)
	return nil
}

type bearerTokenGetter struct {
	lock sync.Mutex

	waitQueue []func(error)

	lastError  error
	started    bool
	finished   bool
	tokenStore string
}

func (b *bearerTokenGetter) onTokenObtained(task func(error)) {
	if b.finished {
		task(b.lastError)
		return
	}

	b.waitQueue = append(b.waitQueue, task)
}

func (b *bearerTokenGetter) executeWaitQueue() {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.finished = true
	for len(b.waitQueue) > 0 {
		b.waitQueue[0](b.lastError)
		b.waitQueue = b.waitQueue[1:]
	}
}

// Start is a concurrency-safe async function that obtains a bearerToken
// with cluster-admin privileges
func (b *bearerTokenGetter) Start(adminClient kubernetes.Interface) {
	if b.started {
		panic("attempt to start bearerTokenGetter task a second time")
	}
	if b.finished {
		return
	}

	b.started = true
	b.lock.Lock()
	defer b.lock.Unlock()
	go b.getBearerToken(adminClient, "openshift-must-gather")
}

func (b *bearerTokenGetter) getBearerToken(adminClient kubernetes.Interface, namespace string) {
	// ensure our namespace exists
	_, err := adminClient.CoreV1().Namespaces().Create(&corev1.Namespace{
		TypeMeta:   metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String(), Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	})
	if err != nil && !kapierrs.IsAlreadyExists(err) {
		b.lastError = err
		return
	}

	sa := &corev1.ServiceAccount{
		TypeMeta:   metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String(), Kind: "ServiceAccount"},
		ObjectMeta: metav1.ObjectMeta{Name: namespace + "-sa", Namespace: namespace},
	}

	_, err = adminClient.CoreV1().ServiceAccounts(namespace).Create(sa)
	if err != nil && !kapierrs.IsAlreadyExists(err) {
		b.lastError = err
		return
	}

	// ensure our service account can GET /metrics
	_, err = adminClient.RbacV1().ClusterRoleBindings().Create(&rbacv1.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{APIVersion: rbacv1.SchemeGroupVersion.String(), Kind: "ClusterRoleBinding"},
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace + "-metrics-viewer",
		},
		RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "cluster-admin"},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      sa.Name,
				Namespace: namespace,
			},
		},
	})
	if err != nil && !kapierrs.IsAlreadyExists(err) {
		b.lastError = err
		return
	}
	if err := wait.PollImmediate(100*time.Millisecond, bearerTokenCreationTimeout, func() (bool, error) {
		secrets, err := adminClient.CoreV1().Secrets(namespace).List(metav1.ListOptions{})
		if err != nil && !kapierrs.IsNotFound(err) {
			return false, err
		}

		bearerToken := ""
		for _, secret := range secrets.Items {
			annotations := secret.GetAnnotations()
			annotation, ok := annotations[corev1.ServiceAccountNameKey]
			if !ok || annotation != sa.Name {
				continue
			}
			if secret.Type != corev1.SecretTypeServiceAccountToken {
				continue
			}

			token, ok := secret.Data[corev1.ServiceAccountTokenKey]
			if !ok || len(token) == 0 {
				return false, nil
			}

			bearerToken = string(token)
			break
		}

		if len(bearerToken) == 0 {
			return false, nil
		}

		b.finished = true
		b.tokenStore = bearerToken
		b.executeWaitQueue()
		return true, nil
	}); err != nil {

	}
}
