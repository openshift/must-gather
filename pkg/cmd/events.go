package cmd

import (
	"fmt"

	"github.com/openshift/must-gather/pkg/util"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericclioptions/resource"
)

var (
	eventsExample = `
	# Parse events for "openshift-apiserver-operator"
	%[1]s events https://<ci-artifacts>/events.json --component=openshift-apiserver-operator

	# Print all available components in events
	%[1]s events https://<ci-artifacts>/events.json --list-components
`
)

type EventsOptions struct {
	fileWriter *util.MultiSourceFileWriter
	builder    *resource.Builder
	args       []string

	eventFileURL string

	componentName  string
	listComponents bool

	genericclioptions.IOStreams
}

func NewEventsOptions(streams genericclioptions.IOStreams) *EventsOptions {
	return &EventsOptions{
		IOStreams: streams,
	}
}

func NewCmdEvents(parentName string, streams genericclioptions.IOStreams) *cobra.Command {
	o := NewEventsOptions(streams)

	cmd := &cobra.Command{
		Use:          "events <URL> [flags]",
		Short:        "Collect debugging data for a given cluster operator",
		Example:      fmt.Sprintf(inspectExample, parentName),
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

	cmd.Flags().StringVar(&o.componentName, "component", "", "Name of the component to filter events for (eg. 'openshift-apiserver-operator')")
	cmd.Flags().BoolVar(&o.listComponents, "list-components", false, "List all available component names in events")

	return cmd
}

func (o *EventsOptions) Complete(command *cobra.Command, args []string) error {
	if len(args) == 1 {
		o.eventFileURL = args[0]
	}
	return nil
}

func (o *EventsOptions) Validate() error {
	if o.listComponents && len(o.componentName) > 0 {
		return fmt.Errorf("cannot use list-events with component specified")
	}
	if o.listComponents {
		return nil
	}
	if len(o.eventFileURL) == 0 {
		return fmt.Errorf("the event URL must be specified")
	}
	if len(o.componentName) == 0 {
		return fmt.Errorf("the component name must be specified")
	}
	return nil
}

func (o *EventsOptions) Run() error {
	eventFileBytes, err := util.GetEventBytesFromURL(o.eventFileURL)
	if err != nil {
		return err
	}
	if err := util.PrintEvents(o.Out, eventFileBytes, o.componentName, o.listComponents); err != nil {
		return err
	}
	return nil
}
