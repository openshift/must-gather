package cmd

import (
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

type MustGatherOptions struct {
	configFlags *genericclioptions.ConfigFlags

	genericclioptions.IOStreams
}

func NewMustGatherOptions(streams genericclioptions.IOStreams) *MustGatherOptions {
	return &MustGatherOptions{
		configFlags: genericclioptions.NewConfigFlags(),
		IOStreams: streams,
	}
}

func NewCmdMustGather(streams genericclioptions.IOStreams) *cobra.Command {
	o := NewMustGatherOptions(streams)

	cmd := &cobra.Command{
		Use: "openshift-must-gather",
		Short: "Gather debugging data for a given cluster operator",
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

	cmd.AddCommand(NewCmdInfo("openshift-must-gather", streams))
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