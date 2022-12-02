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

package fswrap

import (
	"io/ioutil"
	"log"
	"os"
)

type FSWrapper struct {
	Log *log.Logger
}

func (fs FSWrapper) Open(name string) (*os.File, error) {
	fs.Log.Printf("fswrap %-8s %q", "Open", name)
	return os.Open(name)
}

func (fs FSWrapper) ReadFile(filename string) ([]byte, error) {
	fs.Log.Printf("fswrap %-8s %q", "ReadFile", filename)
	return ioutil.ReadFile(filename)
}

func (fs FSWrapper) ReadDir(dirname string) ([]os.FileInfo, error) {
	fs.Log.Printf("fswrap %-8s %q", "ReadDir", dirname)
	return ioutil.ReadDir(dirname)
}
