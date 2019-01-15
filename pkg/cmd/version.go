package cmd

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"

	"github.com/spf13/cobra"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericclioptions/resource"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/must-gather/pkg/util"
)

var (
	versionExample = `
	# Collect debugging data for the "openshift-apiserver-operator"
	%[1]s version
`
)

type VersionOptions struct {
	printFlags  *genericclioptions.PrintFlags
	configFlags *genericclioptions.ConfigFlags

	restConfig      *rest.Config
	kubeClient      kubernetes.Interface
	discoveryClient discovery.CachedDiscoveryInterface
	dynamicClient   dynamic.Interface

	podUrlGetter *util.RemotePodURLGetter

	fileWriter *util.MultiSourceFileWriter
	builder    *resource.Builder
	args       []string

	// directory where all gathered data will be stored
	baseDir string
	// whether or not to allow writes to an existing and populated base directory
	overwrite bool

	genericclioptions.IOStreams
}

func NewVersionOptions(streams genericclioptions.IOStreams) *VersionOptions {
	return &VersionOptions{
		printFlags:  genericclioptions.NewPrintFlags("gathered").WithDefaultOutput("yaml").WithTypeSetter(scheme.Scheme),
		configFlags: genericclioptions.NewConfigFlags(),
		IOStreams:   streams,
	}
}

func NewCmdVersion(parentName string, streams genericclioptions.IOStreams) *cobra.Command {
	o := NewVersionOptions(streams)

	cmd := &cobra.Command{
		Use:          "version [flags]",
		Short:        "Gather debugging data for a given Cluser Version",
		Example:      fmt.Sprintf(versionExample, parentName),
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

	cmd.Flags().StringVar(&o.baseDir, "base-dir", "must-gather", "Root directory used for storing all gathered cluster operator data. Defaults to $(PWD)/must-gather")
	cmd.Flags().BoolVar(&o.overwrite, "overwrite", false, "If true, allow this command to write to an existing location with previous data present")

	o.printFlags.AddFlags(cmd)
	return cmd
}

func (o *VersionOptions) Complete(cmd *cobra.Command, args []string) error {
	o.args = args

	var err error
	o.restConfig, err = o.configFlags.ToRESTConfig()
	if err != nil {
		return err
	}

	o.kubeClient, err = kubernetes.NewForConfig(o.restConfig)
	if err != nil {
		return err
	}

	o.dynamicClient, err = dynamic.NewForConfig(o.restConfig)
	if err != nil {
		return err
	}

	o.discoveryClient, err = o.configFlags.ToDiscoveryClient()
	if err != nil {
		return err
	}

	printer, err := o.printFlags.ToPrinter()
	if err != nil {
		return err
	}
	o.fileWriter = util.NewMultiSourceWriter(printer)
	o.podUrlGetter = &util.RemotePodURLGetter{
		Protocol: "https",
		Host:     "localhost",
		Port:     "8443",
	}

	// pre-fetch token while we perform other tasks
	if err := o.podUrlGetter.FetchToken(o.restConfig); err != nil {
		return err
	}

	o.builder = resource.NewBuilder(o.configFlags)
	return nil
}

func (o *VersionOptions) Validate() error {
	if len(o.args) >= 1 {
		return fmt.Errorf("this option takes not arguments")
	}
	if len(o.baseDir) == 0 {
		return fmt.Errorf("--base-dir must not be empty")
	}
	return nil
}

func (o *VersionOptions) Run() error {
	// next, ensure we're able to proceed writing data to specified destination
	if err := o.ensureDirectoryViable(o.baseDir, o.overwrite); err != nil {
		log.Printf("Failed to setup %q", o.baseDir)
		return err
	}

	errs := []error{}
	clusterVersion, err := o.dynamicClient.Resource(configv1.SchemeGroupVersion.WithResource("clusterversions")).List(metav1.ListOptions{})
	if err != nil {
		errs = append(errs, err)
	}

	objToPrint := runtime.Object(clusterVersion)

	filename := fmt.Sprintf("%s.yaml", "clusterversion")

	if err := o.fileWriter.WriteFromResource(path.Join(o.baseDir, "/"+filename), objToPrint); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("One or more errors ocurred gathering cluster data:\n\n    %v", errors.NewAggregate(errs))
	}

	log.Printf("Finished successfully with no errors.\n")
	return nil
}

// ensureDirectoryViable returns an error if the given path:
// 1a. create it if it does not exsist. 
// 1b. already exists AND is a file (not a directory)
//  2. already exists AND is NOT empty
//  3. an IO error occurs
func (o *VersionOptions) ensureDirectoryViable(dirPath string, allowDataOverride bool) error {
	baseDirInfo, err := os.Stat(dirPath)
	if err != nil && os.IsNotExist(err) {
		// no error, directory simply does not exist yet
		log.Printf("%q does not exsist, working to create", dirPath)
		// ensure destination path exists
		if err := os.MkdirAll(dirPath, os.ModePerm); err != nil {
			return err
		}
		return nil
	}
	if err != nil {
		return err
	}

	if !baseDirInfo.IsDir() {
		return fmt.Errorf("%q exists and is a file", dirPath)
	}
	files, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return err
	}
	if len(files) > 0 && !allowDataOverride {
		return fmt.Errorf("%q exists and is not empty. Pass --overwrite to allow data overwrites", dirPath)
	}
	return nil
}
