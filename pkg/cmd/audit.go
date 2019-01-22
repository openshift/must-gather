package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericclioptions/resource"

	"github.com/openshift/must-gather/pkg/util"
)

var (
	auditExample = `
	# Print all audit log events formatted:
	%[1]s events https://<ci-artifacts>/masters-kube-apiserver-audit.log.gz

	# Search for all audit events that include 'openshift-kube-apiserver-operator' in URI
	%[1]s events https://<ci-artifacts>/masters-kube-apiserver-audit.log.gz --contain=openshift-kube-apiserver-operator

	# Search for GET audit events that matches regexp 'apiserver-operator$' in URI
	%[1]s events https://<ci-artifacts>/masters-kube-apiserver-audit.log.gz --regexp="apiserver-operator$" --verb=get
`
)

type AuditOptions struct {
	fileWriter *util.MultiSourceFileWriter
	builder    *resource.Builder
	args       []string

	auditFileURL string
	contains     string
	regexp       string
	verb         string

	genericclioptions.IOStreams
}

func NewAuditOptions(streams genericclioptions.IOStreams) *AuditOptions {
	return &AuditOptions{
		IOStreams: streams,
	}
}

func NewCmdAudit(parentName string, streams genericclioptions.IOStreams) *cobra.Command {
	o := NewAuditOptions(streams)

	cmd := &cobra.Command{
		Use:          "audit <URL> [flags]",
		Short:        "Inspects the audit logs captured during CI test run.",
		Example:      fmt.Sprintf(auditExample, parentName),
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

	cmd.Flags().StringVar(&o.contains, "contains", "", "Search for audit logs that contains specified URI")
	cmd.Flags().StringVar(&o.regexp, "regexp", "", "Search for audit logs with URI that matches the regexp")
	cmd.Flags().StringVar(&o.verb, "verb", "", "Filter result of search to only contain the specified verb (eg. 'update', 'get', etc..)")

	return cmd
}

func (o *AuditOptions) Complete(command *cobra.Command, args []string) error {
	if len(args) == 1 {
		o.auditFileURL = args[0]
	}
	return nil
}

func (o *AuditOptions) Validate() error {
	if len(o.auditFileURL) == 0 {
		fmt.Errorf("you must specify the audit log URL")
	}
	if len(o.regexp) > 0 && len(o.contains) > 0 {
		fmt.Errorf("either regexp or contains are supported, not both")
	}

	if len(o.regexp) == 0 && len(o.contains) == 0 && len(o.verb) > 0 {
		fmt.Errorf("verb only supported for searching")
	}
	return nil
}

func (o *AuditOptions) Run() error {
	events, err := util.GetAuditEventsFromURL(o.auditFileURL)
	if err != nil {
		return err
	}

	// no search, just print the audit event statistic
	if len(o.regexp) == 0 && len(o.contains) == 0 {
		util.PrintAllAuditEvents(o.Out, events)
		return nil
	}

	// print verb nicely on zero match
	verbHuman := o.verb
	if len(verbHuman) == 0 {
		verbHuman = "*"
	}

	if len(o.regexp) > 0 {
		result, err := events.SearchEventsRegexp(o.regexp, o.verb)
		if err != nil {
			return err
		}
		if len(result) == 0 {
			return fmt.Errorf("No audit events found matching query %q and verb %q", o.regexp, verbHuman)
		}
		util.PrintSearchAuditEvents(o.Out, result)
		return nil
	}

	if len(o.contains) > 0 {
		result := events.SearchEventsContains(o.contains, o.verb)
		if len(result) == 0 {
			return fmt.Errorf("No audit events found matching query %q and verb %q", o.contains, verbHuman)
		}
		util.PrintSearchAuditEvents(o.Out, result)
		return nil
	}

	return nil
}
