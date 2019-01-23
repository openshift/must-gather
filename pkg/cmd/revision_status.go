package cmd

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericclioptions/resource"
	"k8s.io/client-go/kubernetes/scheme"
)

var (
	revisionStatusExample = `
	# Print number of failed installer pods and what revision the kube-apiserver static pods are running
	%[1]s revision-status -n openshift-kube-apiserver
`
)

type RevisionStatusOptions struct {
	builderFlags *genericclioptions.ResourceBuilderFlags
	configFlags  *genericclioptions.ConfigFlags

	resourceFinder genericclioptions.ResourceFinder

	genericclioptions.IOStreams
}

func NewRevisionStatusOptions(streams genericclioptions.IOStreams) *RevisionStatusOptions {
	return &RevisionStatusOptions{
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

func NewCmdRevisionStatus(parentName string, streams genericclioptions.IOStreams) *cobra.Command {
	o := NewRevisionStatusOptions(streams)

	cmd := &cobra.Command{
		Use:          "revision-status -n <namespace>",
		Short:        "Counts failed installer pods and current revision of static pods.",
		Example:      fmt.Sprintf(revisionStatusExample, parentName),
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(c); err != nil {
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

func (o *RevisionStatusOptions) Complete(cmd *cobra.Command) error {
	o.resourceFinder = o.builderFlags.ToBuilder(o.configFlags, []string{"configmaps"})

	return nil
}

func (o *RevisionStatusOptions) Run() error {
	visitor := o.resourceFinder.Do()
	var succeededRevisionIDs, failedRevisionIDs, inProgressRevisionIDs, unknownStatusRevisionIDs []string
	err := visitor.Visit(func(info *resource.Info, err error) error {
		if err != nil {
			return err
		}

		configMap, ok := info.Object.(*corev1.ConfigMap)
		if !ok {
			return fmt.Errorf("unable to cast resource to configmap: %v", info.Object)
		}

		if !strings.HasPrefix(configMap.Name, "revision-status-") {
			return nil
		}
		if revision, ok := configMap.Data["revision"]; ok {
			switch configMap.Data["status"] {
			case string(corev1.PodSucceeded):
				succeededRevisionIDs = append(succeededRevisionIDs, revision)
			case string(corev1.PodFailed):
				failedRevisionIDs = append(failedRevisionIDs, revision)
			case "InProgress":
				inProgressRevisionIDs = append(inProgressRevisionIDs, revision)
			default:
				unknownStatusRevisionIDs = append(unknownStatusRevisionIDs, revision)
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	revisions := []struct {
		name string
		ids  []string
	}{
		{name: "Succeeded", ids: succeededRevisionIDs},
		{name: "Failed", ids: failedRevisionIDs},
		{name: "InProgress", ids: inProgressRevisionIDs},
		{name: "Unknown", ids: unknownStatusRevisionIDs},
	}

	w := tabwriter.NewWriter(o.Out, 15, 0, 0, ' ', tabwriter.DiscardEmptyColumns)
	if _, err := fmt.Fprint(w, "STATUS\tCOUNT\tIDs\n"); err != nil {
		return err
	}
	for _, revision := range revisions {
		if _, err := fmt.Fprintf(w, "%s\t%d\t%s\n", revision.name, len(revision.ids), strings.Join(revision.ids, ",")); err != nil {
			return err
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}

	return nil
}
