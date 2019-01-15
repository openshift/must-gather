package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"

	routev1 "github.com/openshift/client-go/route/clientset/versioned/typed/route/v1"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericclioptions/printers"
	"k8s.io/cli-runtime/pkg/genericclioptions/resource"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type EverythingOptions struct {
	printFlags  *genericclioptions.PrintFlags
	configFlags *genericclioptions.ConfigFlags

	restConfig      *rest.Config
	routesClient    routev1.RouteV1Interface
	kubeClient      kubernetes.Interface
	discoveryClient discovery.CachedDiscoveryInterface
	dynamicClient   dynamic.Interface

	printer printers.ResourcePrinter
	builder *resource.Builder
	args    []string

	// directory where all gathered data will be stored
	baseDir string
	// whether or not to allow writes to an existing and populated base directory
	overwrite bool

	genericclioptions.IOStreams
}

func NewEverythingOptions(streams genericclioptions.IOStreams) *EverythingOptions {
	return &EverythingOptions{
		printFlags:  genericclioptions.NewPrintFlags("gathered").WithDefaultOutput("yaml"),
		configFlags: genericclioptions.NewConfigFlags(),
		IOStreams:   streams,
	}
}

func NewCmdEverything(parentName string, streams genericclioptions.IOStreams) *cobra.Command {
	o := NewEverythingOptions(streams)

	cmd := &cobra.Command{
		Use:          "everything",
		Short:        "Gather debugging data for a given cluster operator",
		Example:      fmt.Sprintf(infoExample, parentName),
		SilenceUsage: true,
		Run: func(cmd *cobra.Command, args []string) {
			o := &PluginListOptions{
				IOStreams: streams,
			}
			if err := o.Complete(cmd); err != nil {
				panic(err)
			}
			plugins, err := o.ListPlugins()
			if err != nil {
				panic(err)
			}

			fmt.Printf("Plugins %v\n", plugins)

			// run our other command like info first

			newArgs := []string{}
			if len(args) > 1 {
				newArgs = args[1:]
			}

			for _, plugin := range plugins {
				if err := handleEndpointExtensions(&defaultPluginHandler{}, plugin, newArgs); err != nil {
					panic(err)
				}
			}
		},
	}

	cmd.Flags().StringVar(&o.baseDir, "base-dir", "must-gather", "Root directory used for storing all gathered cluster operator data. Defaults to $(PWD)/must-gather")
	cmd.Flags().BoolVar(&o.overwrite, "overwrite", false, "If true, allow this command to write to an existing location with previous data present")

	o.printFlags.AddFlags(cmd)
	return cmd
}

// PluginHandler is capable of parsing command line arguments
// and performing executable filename lookups to search
// for valid plugin files, and execute found plugins.
type PluginHandler interface {
	// Lookup receives a potential filename and returns
	// a full or relative path to an executable, if one
	// exists at the given filename, or an error.
	Lookup(filename string) (string, error)
	// Execute receives an executable's filepath, a slice
	// of arguments, and a slice of environment variables
	// to relay to the executable.
	Execute(executablePath string, cmdArgs, environment []string) error
}

type defaultPluginHandler struct{}

// Lookup implements PluginHandler
func (h *defaultPluginHandler) Lookup(filename string) (string, error) {
	// if on Windows, append the "exe" extension
	// to the filename that we are looking up.
	if runtime.GOOS == "windows" {
		filename = filename + ".exe"
	}

	return exec.LookPath(filename)
}

// Execute implements PluginHandler
func (h *defaultPluginHandler) Execute(executablePath string, cmdArgs, environment []string) error {
	return syscall.Exec(executablePath, cmdArgs, environment)
}

func handleEndpointExtensions(pluginHandler PluginHandler, cmd string, cmdArgs []string) error {
	// invoke cmd binary relaying the current environment and args given
	// remainingArgs will always have at least one element.
	// execve will make remainingArgs[0] the "binary name".
	if err := pluginHandler.Execute(cmd, append([]string{cmd}, cmdArgs[len(cmdArgs):]...), os.Environ()); err != nil {
		return err
	}

	return nil
}
