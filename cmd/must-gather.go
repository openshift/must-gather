package main

import (
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	"os"

	mustgather "github.com/openshift/must-gather/pkg/cmd"
	"github.com/spf13/pflag"
)

type MustGatherOptions struct {
	configFlags *genericclioptions.ConfigFlags

	genericclioptions.IOStreams
}

func NewMustGatherOptions(streams genericclioptions.IOStreams) *MustGatherOptions {
	return &MustGatherOptions{
		configFlags: genericclioptions.NewConfigFlags(),
		IOStreams:   streams,
	}
}

func NewCmdMustGather(streams genericclioptions.IOStreams) *cobra.Command {
	o := NewMustGatherOptions(streams)

	cmd := &cobra.Command{
		Use:          "openshift-must-gather",
		Short:        "Gather debugging data for a given cluster operator",
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

	cmd.AddCommand(mustgather.NewCmdInfo("openshift-must-gather", streams))
	return cmd
}

func (o *MustGatherOptions) Complete(cmd *cobra.Command, args []string) error {
	return nil
}

func (o *MustGatherOptions) Validate() error {
	return nil
}

func (o *MustGatherOptions) Run() error {
	return nil
}

func main() {
	flags := pflag.NewFlagSet("must-gather", pflag.ExitOnError)
	pflag.CommandLine = flags

	root := NewCmdMustGather(genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}

}
