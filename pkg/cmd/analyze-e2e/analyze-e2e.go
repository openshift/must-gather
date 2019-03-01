package analyze_e2e

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/openshift/must-gather/pkg/cmd/analyze-e2e/analyzers"
)

var artifactsToAnalyzeList = map[string]Analyzer{
	"clusteroperators.json": &analyzers.ClusterOperatorsAnalyzer{},
	"pods.json":             &analyzers.PodsAnalyzer{},
}

var (
	analyzeExample = `
	# print out result of analysis for given artifacts directory in e2e run
	openshift-dev-helpers analyze-e2e https://gcsweb-ci.svc.ci.openshift.org/gcs/origin-ci-test/pr-logs/pull/openshift_cluster-kube-apiserver-operator/310/pull-ci-openshift-cluster-kube-apiserver-operator-master-e2e-aws/1559/artifacts/e2e-aws
`
)

type AnalyzeOptions struct {
	artifactsBaseURL string

	genericclioptions.IOStreams
}

func NewAnalyzeOptions(streams genericclioptions.IOStreams) *AnalyzeOptions {
	return &AnalyzeOptions{
		IOStreams: streams,
	}
}

func NewCmdAnalyze(parentName string, streams genericclioptions.IOStreams) *cobra.Command {
	o := NewAnalyzeOptions(streams)

	cmd := &cobra.Command{
		Use:          "analyze-e2e URL [flags]",
		Short:        "Inspects the artifacts gathered during e2e-aws run and analyze them.",
		Example:      fmt.Sprintf(analyzeExample, parentName),
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

	return cmd
}

func (o *AnalyzeOptions) Complete(command *cobra.Command, args []string) error {
	if len(args) >= 1 {
		o.artifactsBaseURL = args[0]
	}
	return nil
}

func (o *AnalyzeOptions) Validate() error {
	if len(o.artifactsBaseURL) == 0 {
		return fmt.Errorf("the URL to e2e-aws artifacts must be specified")
	}
	return nil
}

func (o *AnalyzeOptions) Run() error {
	results, err := GetArtifacts(o.artifactsBaseURL)
	if err != nil {
		return err
	}

	sort.Slice(results, func(i, j int) bool { return results[i].ArtifactName < results[j].ArtifactName })

	for _, result := range results {
		fmt.Fprintf(o.Out, "\n%s:\n\n", result.ArtifactName)
		if result.Error != nil {
			fmt.Fprintf(o.Out, "ERROR: %v\n", result.Error)
			continue
		}
		fmt.Fprintf(o.Out, "%s", result.Output)
	}
	fmt.Fprintf(o.Out, "\n%d files analyzed\n", len(results))
	return nil
}
