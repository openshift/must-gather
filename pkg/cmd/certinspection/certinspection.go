package certinspection

import (
	"crypto/x509"
	"fmt"
	"strings"

	certificatesv1beta1 "k8s.io/api/certificates/v1beta1"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericclioptions/resource"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/cert"
)

var (
	example = `
	# look at certs on the cluster in the "openshift-kube-apiserver" namespace
	openshift-dev-helpers inspect-certs -n openshift-kube-apiserver secrets,configmaps

	# look at certs from CSRs
	openshift-dev-helpers inspect-certs csr

	# create a fake secret from a file to inspect its content 
	oc create secret generic --dry-run -oyaml kubelet --from-file=tls.crt=/home/deads/Downloads/kubelet-client-current.pem | openshift-dev-helpers inspect-certs --local -f -

	# look at a dumped file of resources for inspection
	openshift-dev-helpers inspect-certs --local -f 'path/to/core/configmaps.yaml'
`
)

type CertInspectionOptions struct {
	builderFlags *genericclioptions.ResourceBuilderFlags
	configFlags  *genericclioptions.ConfigFlags

	resourceFinder genericclioptions.ResourceFinder

	genericclioptions.IOStreams
}

func NewCertInspectionOptions(streams genericclioptions.IOStreams) *CertInspectionOptions {
	return &CertInspectionOptions{
		builderFlags: genericclioptions.NewResourceBuilderFlags().
			WithAll(true).
			WithAllNamespaces(false).
			WithFieldSelector("").
			WithLabelSelector("").
			WithLocal(false).
			WithScheme(scheme.Scheme),
		configFlags: genericclioptions.NewConfigFlags(),
		IOStreams:   streams,
	}
}

func NewCmdCertInspection(streams genericclioptions.IOStreams) *cobra.Command {
	o := NewCertInspectionOptions(streams)

	cmd := &cobra.Command{
		Use:          "inspect-certs <resource>",
		Short:        "Inspects the certs, keys, and ca-bundles in a set of resources.",
		Example:      example,
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(c, args); err != nil {
				return err
			}
			if err := o.Validate(); err != nil {
				return err
			}
			if err := o.Run(); err != nil {
				return err
			}

			return nil
		},
	}

	o.builderFlags.AddFlags(cmd.Flags())
	o.configFlags.AddFlags(cmd.Flags())

	return cmd
}

func (o *CertInspectionOptions) Complete(command *cobra.Command, args []string) error {
	o.resourceFinder = o.builderFlags.ToBuilder(o.configFlags, args)

	return nil
}

func (o *CertInspectionOptions) Validate() error {
	return nil
}

func (o *CertInspectionOptions) Run() error {
	visitor := o.resourceFinder.Do()

	err := visitor.Visit(func(info *resource.Info, err error) error {
		if err != nil {
			return err
		}

		switch castObj := info.Object.(type) {
		case *corev1.ConfigMap:
			inspectConfigMap(castObj)
		case *corev1.Secret:
			inspectSecret(castObj)
		case *certificatesv1beta1.CertificateSigningRequest:
			inspectCSR(castObj)
		default:
			return fmt.Errorf("unhandled resource: %T", castObj)
		}

		fmt.Println()
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func inspectConfigMap(obj *corev1.ConfigMap) {
	resourceString := fmt.Sprintf("configmaps/%s[%s]", obj.Name, obj.Namespace)
	caBundle, ok := obj.Data["ca-bundle.crt"]
	if !ok {
		fmt.Printf("%s NOT a ca-bundle\n", resourceString)
		return
	}
	if len(caBundle) == 0 {
		fmt.Printf("%s MISSING ca-bundle content\n", resourceString)
		return
	}

	fmt.Printf("%s - ca-bundle (%v)\n", resourceString, obj.CreationTimestamp.UTC())
	certificates, err := cert.ParseCertsPEM([]byte(caBundle))
	if err != nil {
		fmt.Printf("    ERROR - %v\n", err)
		return
	}
	for _, curr := range certificates {
		fmt.Printf("    %s\n", certDetail(curr))
	}
}

func inspectSecret(obj *corev1.Secret) {
	resourceString := fmt.Sprintf("secrets/%s[%s]", obj.Name, obj.Namespace)
	tlsCrt, isTLS := obj.Data["tls.crt"]
	if isTLS {
		fmt.Printf("%s - tls (%v)\n", resourceString, obj.CreationTimestamp.UTC())
		if len(tlsCrt) == 0 {
			fmt.Printf("%s MISSING tls.crt content\n", resourceString)
			return
		}

		certificates, err := cert.ParseCertsPEM([]byte(tlsCrt))
		if err != nil {
			fmt.Printf("    ERROR - %v\n", err)
			return
		}
		for _, curr := range certificates {
			fmt.Printf("    %s\n", certDetail(curr))
		}
	}

	caBundle, isCA := obj.Data["ca.crt"]
	if isCA {
		fmt.Printf("%s - token secret (%v)\n", resourceString, obj.CreationTimestamp.UTC())
		if len(caBundle) == 0 {
			fmt.Printf("%s MISSING ca.crt content\n", resourceString)
			return
		}

		certificates, err := cert.ParseCertsPEM([]byte(caBundle))
		if err != nil {
			fmt.Printf("    ERROR - %v\n", err)
			return
		}
		for _, curr := range certificates {
			fmt.Printf("    %s\n", certDetail(curr))
		}
	}

	if !isTLS && !isCA {
		fmt.Printf("%s NOT a tls secret or token secret\n", resourceString)
		return
	}
}

func inspectCSR(obj *certificatesv1beta1.CertificateSigningRequest) {
	resourceString := fmt.Sprintf("csr/%s", obj.Name)
	if len(obj.Status.Certificate) == 0 {
		fmt.Printf("%s NOT SIGNED\n", resourceString)
		return
	}

	fmt.Printf("%s - (%v)\n", resourceString, obj.CreationTimestamp.UTC())
	certificates, err := cert.ParseCertsPEM([]byte(obj.Status.Certificate))
	if err != nil {
		fmt.Printf("    ERROR - %v\n", err)
		return
	}
	for _, curr := range certificates {
		fmt.Printf("    %s\n", certDetail(curr))
	}
}

func certDetail(certificate *x509.Certificate) string {
	humanName := certificate.Subject.CommonName
	signerHumanName := certificate.Issuer.CommonName
	if certificate.Subject.CommonName == certificate.Issuer.CommonName {
		signerHumanName = "<self>"
	}

	usages := []string{}
	for _, curr := range certificate.ExtKeyUsage {
		if curr == x509.ExtKeyUsageClientAuth {
			usages = append(usages, "client")
			continue
		}
		if curr == x509.ExtKeyUsageServerAuth {
			usages = append(usages, "serving")
			continue
		}

		usages = append(usages, fmt.Sprintf("%d", curr))
	}

	return fmt.Sprintf("%q [%s] issuer=%q (%v to %v)", humanName, strings.Join(usages, ","), signerHumanName, certificate.NotBefore.UTC(), certificate.NotAfter.UTC())
}
