package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/jaypipes/ghw/pkg/snapshot"
	"github.com/openshift-kni/debug-tools/pkg/knit/cmd"
	"github.com/openshift-kni/debug-tools/pkg/knit/cmd/k8s"
	"github.com/openshift-kni/debug-tools/pkg/machineinformer"
	"github.com/spf13/cobra"
)

const machineInfoFilePath string = "machineinfo.json"

type snapshotOptions struct {
	dumpList  bool
	output    string
	rootDir   string
	sleepTime int
}

func main() {
	root := cmd.NewRootCommand(newSnapshotCommand,
		k8s.NewPodResourcesCommand,
		k8s.NewPodInfoCommand,
	)

	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func newSnapshotCommand(knitOpts *cmd.KnitOptions) *cobra.Command {
	opts := &snapshotOptions{}
	snap := &cobra.Command{
		Use:   "snapshot",
		Short: "snapshot pseudofilesystems for offline analysis",
		RunE: func(cmd *cobra.Command, args []string) error {
			return makeSnapshot(cmd, knitOpts, opts, args)
		},
		Args: cobra.NoArgs,
	}
	snap.Flags().StringVar(&opts.rootDir, "root", "", "pseudofs root - use this if running inside a container")
	snap.Flags().StringVar(&opts.output, "output", "", "path to clone system information into")
	snap.Flags().BoolVar(&opts.dumpList, "dump", false, "just dump the glob list of expected content and exit")
	// use this to debug container behaviour
	snap.Flags().IntVar(&opts.sleepTime, "sleep", 0, "amount of seconds to sleep once done, before exit")

	return snap
}

func collectMachineinfo(knitOpts *cmd.KnitOptions, destPath string) error {
	outfile, err := os.Create(destPath)
	if err != nil {
		return err
	}

	mih := machineinformer.Handle{
		RootDirectory: knitOpts.SysFSRoot,
		Out:           outfile,
	}
	mih.Run()

	return nil
}

func makeSnapshot(cmd *cobra.Command, knitOpts *cmd.KnitOptions, opts *snapshotOptions, args []string) error {
	// ghw can't handle duplicates in CopyFilesInto, the operation will fail.
	// Hence we need to make sure we just don't feed duplicates.
	fileSpecs := dedupExpectedContent(kniExpectedCloneContent(), snapshot.ExpectedCloneContent())
	if opts.dumpList {
		for _, fileSpec := range fileSpecs {
			fmt.Printf("%s\n", fileSpec)
		}
		return nil
	}

	if opts.output == "" {
		return fmt.Errorf("--output is required")
	}

	if knitOpts.Debug {
		snapshot.SetTraceFunction(func(msg string, args ...interface{}) {
			knitOpts.Log.Printf(msg, args...)
		})
	}

	scratchDir, err := ioutil.TempDir("", "perf-must-gather-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(scratchDir)

	if opts.rootDir != "" {
		fileSpecs = chrootFileSpecs(fileSpecs, opts.rootDir)
	}

	if err := snapshot.CopyFilesInto(fileSpecs, scratchDir, nil); err != nil {
		return fmt.Errorf("error cloning extra files into %q: %v", scratchDir, err)
	}

	if opts.rootDir != "" {
		scratchDir = filepath.Join(scratchDir, opts.rootDir)
	}

	// intentionally ignore errors, keep collecting data
	// machineinfo data is accessory, if we fail to collect
	// we want to keep going and try to collect /proc and /sys data,
	// which is more important
	localPath := filepath.Join(scratchDir, machineInfoFilePath)
	err = collectMachineinfo(knitOpts, localPath)
	if err != nil {
		log.Printf("error collecting machineinfo data: %v - continuing", err)
	}

	dest := opts.output
	if dest == "-" {
		err = snapshot.PackWithWriter(os.Stdout, scratchDir)
		dest = "stdout"
	} else {
		err = snapshot.PackFrom(dest, scratchDir)
	}
	if err != nil {
		return fmt.Errorf("error packing %q to %q: %v", scratchDir, dest, err)
	}

	if opts.sleepTime > 0 {
		knitOpts.Log.Printf("sleeping for %d seconds before exit", opts.sleepTime)
		time.Sleep(time.Duration(opts.sleepTime) * time.Second)
	}

	return nil
}

func chrootFileSpecs(fileSpecs []string, root string) []string {
	var entries []string
	for _, fileSpec := range fileSpecs {
		entries = append(entries, filepath.Join(root, fileSpec))
	}
	return entries
}

func dedupExpectedContent(fileSpecs, extraFileSpecs []string) []string {
	specSet := make(map[string]int)
	for _, fileSpec := range fileSpecs {
		specSet[fileSpec]++
	}
	for _, extraFileSpec := range extraFileSpecs {
		specSet[extraFileSpec]++
	}

	var retSpecs []string
	for retSpec := range specSet {
		retSpecs = append(retSpecs, retSpec)
	}
	return retSpecs
}

func kniExpectedCloneContent() []string {
	return []string{
		// generic information
		"/proc/cmdline",
		// IRQ affinities
		"/proc/interrupts",
		"/proc/irq/default_smp_affinity",
		"/proc/irq/*/*affinity_list",
		"/proc/irq/*/node",
		// softirqs counters
		"/proc/softirqs",
		// KNI-specific CPU infos:
		"/sys/devices/system/cpu/smt/active",
		"/proc/sys/kernel/sched_domain/cpu*/domain*/flags",
		"/sys/devices/system/cpu/offline",
		// BIOS/firmware versions
		"/sys/class/dmi/id/bios*",
		"/sys/class/dmi/id/product_family",
		"/sys/class/dmi/id/product_name",
		"/sys/class/dmi/id/product_sku",
		"/sys/class/dmi/id/product_version",
	}
}
