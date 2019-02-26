package audit

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
)

func PrintAuditEvents(writer io.Writer, events []*auditv1.Event) {
	w := tabwriter.NewWriter(writer, 20, 0, 0, ' ', tabwriter.DiscardEmptyColumns)
	defer w.Flush()

	for _, event := range events {
		duration := event.StageTimestamp.Time.Sub(event.RequestReceivedTimestamp.Time)
		fmt.Fprintf(w, "%s [%s][%s] [%d]\t %s\t [%s]\n", event.RequestReceivedTimestamp.Format("15:04:05"), strings.ToUpper(event.Verb), duration, event.ResponseStatus.Code, event.RequestURI, event.User.Username)
	}
}

func GetEvents(auditFilename string) ([]*auditv1.Event, error) {
	stat, err := os.Stat(auditFilename)
	if err != nil {
		return nil, err
	}
	if !stat.IsDir() {
		file, err := os.Open(auditFilename)
		if err != nil {
			return nil, err
		}

		scanner := bufio.NewScanner(file)
		ret := []*auditv1.Event{}

		// each line in audit file use following format: `hostname {JSON}`, we are not interested in hostname,
		// so lets parse out the events.
		for scanner.Scan() {
			auditBytes := scanner.Bytes()
			if len(auditBytes) > 0 {
				if string(auditBytes[0]) != "{" {
					// strip the hostname part
					hostnameEndPos := bytes.Index(auditBytes, []byte(" "))
					if hostnameEndPos == -1 {
						// oops something is wrong in the file?
						continue
					}

					auditBytes = auditBytes[hostnameEndPos:]
				}
			}

			// shame, shame shame... we have to copy out the apiserver/apis/audit/v1alpha1.Event because adding it as dependency
			// will cause mess in flags...
			eventObj := &auditv1.Event{}
			if err := json.Unmarshal(auditBytes, eventObj); err != nil {
				return nil, fmt.Errorf("unable to decode: %s to audit event: %v", string(auditBytes), err)
			}

			// Add to index
			ret = append(ret, eventObj)
		}

		return ret, nil
	}

	// it was a directory, recurse.
	ret := []*auditv1.Event{}
	err = filepath.Walk(auditFilename,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Name() == stat.Name() {
				return nil
			}
			newEvents, err := GetEvents(info.Name())
			if err != nil {
				return err
			}
			ret = append(ret, newEvents...)

			return nil
		})

	// sort events by time
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].RequestReceivedTimestamp.Time.Before(ret[j].RequestReceivedTimestamp.Time)
	})

	return ret, err
}
