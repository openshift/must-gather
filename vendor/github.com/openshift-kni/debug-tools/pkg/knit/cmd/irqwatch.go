/*
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright 2020 Red Hat, Inc.
 */

package cmd

import (
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"github.com/openshift-kni/debug-tools/pkg/irqs"
)

type irqWatchOptions struct {
	period  string
	maxRuns int
	verbose int
}

func NewIRQWatchCommand(knitOpts *KnitOptions) *cobra.Command {
	opts := &irqWatchOptions{}
	irqWatch := &cobra.Command{
		Use:   "irqwatch",
		Short: "watch IRQ counters",
		RunE: func(cmd *cobra.Command, args []string) error {
			return watchIRQs(cmd, knitOpts, opts, args)
		},
		Args: cobra.NoArgs,
	}
	irqWatch.Flags().IntVarP(&opts.maxRuns, "watch-times", "T", -1, "number of watch loops to perform, each every `watch-period`. Use -1 to run forever.")
	irqWatch.Flags().StringVarP(&opts.period, "watch-period", "W", "1s", "period to poll IRQ counters.")
	irqWatch.Flags().IntVarP(&opts.verbose, "verbose", "v", 1, "verbosiness amount.")
	return irqWatch
}

func watchIRQs(cmd *cobra.Command, knitOpts *KnitOptions, opts *irqWatchOptions, args []string) error {
	if opts.maxRuns == 0 {
		return nil
	}

	var err error
	period, err := time.ParseDuration(opts.period)
	if err != nil {
		return err
	}

	var initStats irqs.Stats
	var prevStats irqs.Stats
	var lastStats irqs.Stats

	ih := irqs.New(knitOpts.Log, knitOpts.ProcFSRoot)

	initTs := time.Now()
	initStats, err = ih.ReadStats()
	if err != nil {
		return err
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	prevStats = initStats.Clone()
	ticker := time.NewTicker(period)
	reporter := irqs.NewReporter(os.Stdout, knitOpts.JsonOutput, opts.verbose, knitOpts.Cpus)

	done := false
	iterCount := 1
	for {
		select {
		case <-c:
			done = true
		case t := <-ticker.C:
			lastStats, err = ih.ReadStats()
			if err != nil {
				return err
			}
			reporter.Delta(t, prevStats, lastStats)
			prevStats = lastStats
		}

		if done {
			break
		}
		if opts.maxRuns > 0 && iterCount >= opts.maxRuns {
			break
		}
		iterCount++
	}

	reporter.Summary(initTs, initStats, lastStats)
	return nil
}
