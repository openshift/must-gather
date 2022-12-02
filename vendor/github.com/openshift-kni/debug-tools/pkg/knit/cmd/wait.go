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
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

type waitOpts struct {
	healthFile string
}

func waitForever(dwOpts *waitOpts) error {
	if dwOpts.healthFile != "" {
		message := []byte("ok")
		ioutil.WriteFile(dwOpts.healthFile, message, 0644) // intentionally ignore error
	}

	exitSignal := make(chan os.Signal)
	signal.Notify(exitSignal, syscall.SIGINT, syscall.SIGTERM)
	<-exitSignal
	return nil
}

func NewWaitCommand(_ *KnitOptions) *cobra.Command {
	flags := &waitOpts{}
	show := &cobra.Command{
		Use:   "wait",
		Short: "wait forever, or until a UNIX signal (SIGINT, SIGTERM) arrives",
		RunE: func(cmd *cobra.Command, args []string) error {
			return waitForever(flags)
		},
		Args: cobra.NoArgs,
	}
	show.Flags().StringVarP(&flags.healthFile, "health-file", "H", "", "health file full path. Use \"\" to disable.")
	return show
}
