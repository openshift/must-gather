package main

import (
	"context"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/component-base/cli"

	otecmd "github.com/openshift-eng/openshift-tests-extension/pkg/cmd"
	oteextension "github.com/openshift-eng/openshift-tests-extension/pkg/extension"

	"k8s.io/klog/v2"
)

func main() {
	command := newOperatorTestCommand(context.Background())
	code := cli.Run(command)
	os.Exit(code)
}

func newOperatorTestCommand(ctx context.Context) *cobra.Command {
	registry := prepareOperatorTestsRegistry()

	cmd := &cobra.Command{
		Use:     "must-gather-tests-ext",
		Short:   "A binary used to run must-gather tests as part of OTE.",
		Version: "1.0",
		Run: func(cmd *cobra.Command, args []string) {
			if err := cmd.Help(); err != nil {
				klog.Fatal(err)
			}
		},
	}

	cmd.AddCommand(otecmd.DefaultExtensionCommands(registry)...)

	return cmd
}

func prepareOperatorTestsRegistry() *oteextension.Registry {
	registry := oteextension.NewRegistry()
	extension := oteextension.NewExtension("openshift", "payload", "must-gather")

	registry.Register(extension)
	return registry
}
