package inspectcontrolplane

import (
	"github.com/openshift/must-gather/pkg/cmd/inspect"
	"k8s.io/cli-runtime/pkg/genericclioptions/resource"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

type InspectControlPlane struct {
	InspectOptions *inspect.InspectOptions

	genericclioptions.IOStreams
}

func NewInspectControlPlaneOptions(streams genericclioptions.IOStreams) *InspectControlPlane {
	return &InspectControlPlane{
		InspectOptions: inspect.NewInspectOptions(streams),
		IOStreams:      streams,
	}
}

func NewCmdInspectControlPlane(streams genericclioptions.IOStreams) *cobra.Command {
	o := NewInspectControlPlaneOptions(streams)

	cmd := &cobra.Command{
		Use:          "inspect-control-plane",
		Short:        "Collect debugging data for a given cluster operator",
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

	o.InspectOptions.AddFlags(cmd)
	return cmd
}

func (o *InspectControlPlane) Complete(cmd *cobra.Command, args []string) error {
	o.InspectOptions.Complete(cmd, args)

	return nil
}

func (o *InspectControlPlane) Validate() error {
	return o.InspectOptions.Validate()
}

func (o *InspectControlPlane) Run() error {
	// call the inspect command with the args we know we need to collect

	argsList := [][]string{
		{"clusteroperators.config.openshift.io"},
		{"certificatesigningrequests.certificates.k8s.io"},
		{"nodes"},
	}

	errs := []error{}
	for _, args := range argsList {
		o.InspectOptions.Builder = resource.NewBuilder(o.InspectOptions.ConfigFlags)

		if err := o.InspectOptions.Run(args...); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.NewAggregate(errs)
}
