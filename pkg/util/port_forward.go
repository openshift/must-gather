package util

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	rbacv1client "k8s.io/client-go/kubernetes/typed/rbac/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

const defaultPortForwardTokenNamespace = "must-gather"

type defaultPortForwarder struct {
	restConfig *rest.Config

	StopChannel  chan struct{}
	ReadyChannel chan struct{}
}

func newDefaultPortForwarder(adminConfig *rest.Config) *defaultPortForwarder {
	return &defaultPortForwarder{
		restConfig:   adminConfig,
		StopChannel:  make(chan struct{}, 1),
		ReadyChannel: make(chan struct{}, 1),
	}
}

func (f *defaultPortForwarder) ForwardPortsAndExecute(pod *corev1.Pod, ports []string, toExecute func()) error {
	if len(ports) < 1 {
		return fmt.Errorf("at least 1 PORT is required for port-forward")
	}

	restClient, err := rest.RESTClientFor(setRESTConfigDefaults(*f.restConfig))
	if err != nil {
		return err
	}

	if pod.Status.Phase != corev1.PodRunning {
		return fmt.Errorf("unable to forward port because pod is not running. Current status=%v", pod.Status.Phase)
	}

	stdout := bytes.NewBuffer(nil)
	req := restClient.Post().
		Resource("pods").
		Namespace(pod.Namespace).
		Name(pod.Name).
		SubResource("portforward")

	transport, upgrader, err := spdy.RoundTripperFor(f.restConfig)
	if err != nil {
		return err
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", req.URL())
	fw, err := portforward.New(dialer, ports, f.StopChannel, f.ReadyChannel, stdout, ioutil.Discard)
	if err != nil {
		return err
	}

	go func() {
		if f.StopChannel != nil {
			defer close(f.StopChannel)
		}

		<-f.ReadyChannel
		toExecute()
	}()

	return fw.ForwardPorts()
}

func setRESTConfigDefaults(config rest.Config) *rest.Config {
	if config.GroupVersion == nil {
		config.GroupVersion = &schema.GroupVersion{Group: "", Version: "v1"}
	}
	if config.NegotiatedSerializer == nil {
		config.NegotiatedSerializer = scheme.Codecs
	}
	if len(config.UserAgent) == 0 {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}
	config.APIPath = "/api"
	return &config
}

func newInsecureRESTClientForHost(host string) (rest.Interface, error) {
	insecure := true

	configFlags := &genericclioptions.ConfigFlags{}
	configFlags.Insecure = &insecure
	configFlags.APIServer = &host

	newConfig, err := configFlags.ToRESTConfig()
	if err != nil {
		return nil, err
	}

	return rest.RESTClientFor(setRESTConfigDefaults(*newConfig))
}

type RemoteContainerPort struct {
	Port     int32
	Protocol string
}

type PortForwardURLGetter struct {
	Protocol  string
	Host      string
	LocalPort string

	Token string
}

func NewPortForwardUrlGetter(localPort string) *PortForwardURLGetter {
	return &PortForwardURLGetter{
		Protocol:  "https",
		Host:      "localhost",
		LocalPort: localPort,
	}
}

func (c *PortForwardURLGetter) WithToken(config *rest.Config) (*PortForwardURLGetter, error) {
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return c, err
	}

	_, err = kubeClient.CoreV1().Namespaces().Create(&corev1.Namespace{
		TypeMeta: metav1.TypeMeta{Kind: "Namespace", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultPortForwardTokenNamespace,
		},
	})
	if err != nil && !errors.IsAlreadyExists(err) {
		return nil, err
	}

	defaultSaName := fmt.Sprintf("%s-sa", defaultPortForwardTokenNamespace)
	_, err = kubeClient.CoreV1().ServiceAccounts(defaultPortForwardTokenNamespace).Create(&corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{Kind: "ServiceAccount", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultSaName,
			Namespace: defaultPortForwardTokenNamespace,
		},
	})
	if err != nil && !errors.IsAlreadyExists(err) {
		return c, err
	}

	rbacClient, err := rbacv1client.NewForConfig(config)
	if err != nil {
		return c, err
	}

	_, err = rbacClient.ClusterRoleBindings().Create(&rbacv1.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{Kind: "ClusterRoleBinding", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-rolebinding", defaultPortForwardTokenNamespace),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      defaultSaName,
				Namespace: defaultPortForwardTokenNamespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
	})
	if err != nil && !errors.IsAlreadyExists(err) {
		return c, err
	}

	secretName := ""
	if err := wait.PollImmediate(time.Second, time.Minute, func() (bool, error) {
		sa, err := kubeClient.CoreV1().ServiceAccounts(defaultPortForwardTokenNamespace).Get(defaultSaName, metav1.GetOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return false, err
		}

		for _, secret := range sa.Secrets {
			if !strings.HasPrefix(secret.Name, defaultSaName+"-token") {
				continue
			}

			secretName = secret.Name
			break
		}
		if len(secretName) == 0 {
			return false, nil
		}

		return true, nil
	}); err != nil {
		return c, err
	}

	secret, err := kubeClient.CoreV1().Secrets(defaultPortForwardTokenNamespace).Get(secretName, metav1.GetOptions{})
	if err != nil {
		return c, err
	}
	token, ok := secret.Data[corev1.ServiceAccountTokenKey]
	if !ok {
		return c, fmt.Errorf("unable to retrieve secret with token for must-gather ServiceAccount")
	}

	c.Token = string(token)
	return c, nil
}

func (c *PortForwardURLGetter) Get(urlPath string, pod *corev1.Pod, config *rest.Config, containerPort *RemoteContainerPort) (string, error) {
	var result string
	var lastErr error
	forwarder := newDefaultPortForwarder(config)

	if err := forwarder.ForwardPortsAndExecute(pod, []string{fmt.Sprintf("%v:%v", c.LocalPort, containerPort.Port)}, func() {
		url := fmt.Sprintf("%s://%s:%s", containerPort.Protocol, c.Host, c.LocalPort)
		restClient, err := newInsecureRESTClientForHost(url)
		if err != nil {
			lastErr = err
			return
		}

		req := restClient.Get().RequestURI(urlPath)
		if len(c.Token) > 0 {
			log.Printf("        Using ServiceAccount token to perform request to %q\n", req.URL().String())
			req = req.SetHeader("Authorization", fmt.Sprintf("Bearer %s", c.Token))
		}

		ioCloser, err := req.Stream()
		if err != nil {
			lastErr = err
			return
		}
		defer ioCloser.Close()

		data := bytes.NewBuffer(nil)
		_, lastErr = io.Copy(data, ioCloser)
		result = data.String()
	}); err != nil {
		return "", err
	}
	return result, lastErr
}
