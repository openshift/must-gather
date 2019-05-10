package events

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/spf13/cobra"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericclioptions/resource"
)

var (
	eventExample = `
	# find all GC calls to deployments in any apigroup (extensions or apps)
	%[1]s event -f event.json --user=system:serviceaccount:kube-system:generic-garbage-collector --resource=deployments.*

	# find all failed calls to kube-system and olm namespaces
	%[1]s event -f event.json --namespace=kube-system --namespace=openshift-operator-lifecycle-manager --failed-only

	# find all GETs against deployments and any resource under config.openshift.io
	%[1]s event -f event.json --resource=deployments.* --resource=*.config.openshift.io --verb=get

	# find CREATEs of everything except SAR and tokenreview
	%[1]s event -f event.json --verb=create --resource=*.* --resource=-subjectaccessreviews.* --resource=-tokenreviews.*
`
)

type EventOptions struct {
	configFlags  *genericclioptions.ConfigFlags
	builderFlags *genericclioptions.ResourceBuilderFlags

	kinds       []string
	namespaces  []string
	names       []string
	reasons     []string
	components  []string
	uids        []string
	filename    string
	warningOnly bool
	output      string
	sortBy      string

	genericclioptions.IOStreams
}

func NewEventOptions(streams genericclioptions.IOStreams) *EventOptions {
	configFlags := genericclioptions.NewConfigFlags()
	configFlags.Namespace = nil

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		panic(err)
	}

	return &EventOptions{
		configFlags: configFlags,
		builderFlags: genericclioptions.NewResourceBuilderFlags().
			WithLocal(true).WithScheme(scheme).WithAllNamespaces(true).WithLatest().WithAll(true),

		IOStreams: streams,
	}
}

func NewCmdEvent(parentName string, streams genericclioptions.IOStreams) *cobra.Command {
	o := NewEventOptions(streams)

	cmd := &cobra.Command{
		Use:          "event -f=event.file [flags]",
		Short:        "Inspects the event logs captured during CI test run.",
		Example:      fmt.Sprintf(eventExample, parentName),
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

	cmd.Flags().StringVarP(&o.output, "output", "o", o.output, "Choose your output format")
	cmd.Flags().StringSliceVar(&o.uids, "uid", o.uids, "Only match specific UIDs")
	cmd.Flags().StringSliceVar(&o.kinds, "kinds", o.kinds, "Filter result of search to only contain the specified kind.)")
	cmd.Flags().StringSliceVarP(&o.namespaces, "namespace", "n", o.namespaces, "Filter result of search to only contain the specified namespace.)")
	cmd.Flags().StringSliceVar(&o.names, "name", o.names, "Filter result of search to only contain the specified name.)")
	cmd.Flags().StringSliceVar(&o.reasons, "reason", o.reasons, "Filter result of search to only contain the specified reason.)")
	cmd.Flags().StringSliceVar(&o.components, "component", o.components, "Filter result of search to only contain the specified component.)")
	cmd.Flags().BoolVar(&o.warningOnly, "warning-only", false, "Filter result of search to only contain http failures.)")
	cmd.Flags().StringVar(&o.sortBy, "by", o.sortBy, "Choose how to sort")

	o.configFlags.AddFlags(cmd.Flags())
	o.builderFlags.AddFlags(cmd.Flags())

	return cmd
}

func (o *EventOptions) Complete(command *cobra.Command, args []string) error {

	return nil
}

func (o *EventOptions) Validate() error {
	return nil
}

func (o *EventOptions) Run() error {
	events := []*corev1.Event{}

	visitor := o.builderFlags.ToBuilder(o.configFlags, nil).Do()
	err := visitor.Visit(func(info *resource.Info, err error) error {
		if err != nil {
			return err
		}

		switch castObj := info.Object.(type) {
		case *corev1.Event:
			event := info.Object.(*corev1.Event)
			events = append(events, event)

			// inject the event twice when it appeared multiple times for easy sorting/reading
			if event.LastTimestamp != event.FirstTimestamp {
				alternateEvent := event.DeepCopy()
				alternateEvent.FirstTimestamp = event.LastTimestamp
				events = append(events, alternateEvent)
			}
		default:
			return fmt.Errorf("unhandled resource: %T", castObj)
		}

		return nil
	})
	if err != nil {
		return err
	}

	filters := EventFilters{}
	if len(o.uids) > 0 {
		filters = append(filters, &FilterByUIDs{UIDs: sets.NewString(o.uids...)})
	}
	if len(o.reasons) > 0 {
		filters = append(filters, &FilterByReasons{Reasons: sets.NewString(o.reasons...)})
	}
	if len(o.names) > 0 {
		filters = append(filters, &FilterByNames{Names: sets.NewString(o.names...)})
	}
	if len(o.namespaces) > 0 {
		filters = append(filters, &FilterByNamespaces{Namespaces: sets.NewString(o.namespaces...)})
	}
	if len(o.kinds) > 0 {
		kinds := map[schema.GroupKind]bool{}
		for _, kind := range o.kinds {
			parts := strings.Split(kind, ".")
			gk := schema.GroupKind{}
			gk.Kind = parts[0]
			if len(parts) >= 2 {
				gk.Group = strings.Join(parts[1:], ".")
			}
			kinds[gk] = true
		}

		filters = append(filters, &FilterByKind{Kinds: kinds})
	}
	if len(o.components) > 0 {
		filters = append(filters, &FilterByComponent{Components: sets.NewString(o.components...)})
	}
	if o.warningOnly {
		filters = append(filters, &FilterByWarnings{})
	}

	events = filters.FilterEvents(events...)

	switch o.sortBy {
	case "", "time":
		sort.Sort(byTime(events))
	case "count":
		sort.Sort(byFrequency(events))
	}

	switch o.output {
	case "components":
		PrintComponents(o.Out, events)
	case "":
		PrintEvents(o.Out, events)
	case "wide":
		PrintEventsWide(o.Out, events)
	case "json":
		encoder := json.NewEncoder(o.Out)
		for _, event := range events {
			if err := encoder.Encode(event); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported output format")
	}

	return nil
}
