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

type eventWithCounter struct {
	event *auditv1.Event
	count int64
}

func PrintAuditEvents(writer io.Writer, events []*auditv1.Event) {
	w := tabwriter.NewWriter(writer, 20, 0, 0, ' ', tabwriter.DiscardEmptyColumns)
	defer w.Flush()

	//
	for _, event := range events {
		duration := event.StageTimestamp.Time.Sub(event.RequestReceivedTimestamp.Time)
		code := int32(0)
		if event.ResponseStatus != nil {
			code = event.ResponseStatus.Code
		}
		fmt.Fprintf(w, "%s [%s][%s] [%d]\t %s\t [%s]\n", event.RequestReceivedTimestamp.Format("15:04:05"), strings.ToUpper(event.Verb), duration, code, event.RequestURI, event.User.Username)
	}
}

func PrintAuditEventsWithCount(writer io.Writer, events []*eventWithCounter) {
	w := tabwriter.NewWriter(writer, 20, 0, 0, ' ', tabwriter.DiscardEmptyColumns)
	defer w.Flush()

	//
	for _, event := range events {
		duration := event.event.StageTimestamp.Time.Sub(event.event.RequestReceivedTimestamp.Time)
		code := int32(0)
		if event.event.ResponseStatus != nil {
			code = event.event.ResponseStatus.Code
		}
		fmt.Fprintf(w, "%dx %s [%s][%s] [%d]\t %s\t [%s]\n", event.count, event.event.RequestReceivedTimestamp.Format("15:04:05"), strings.ToUpper(event.event.Verb), duration, code, event.event.RequestURI,
			event.event.User.Username)
	}
}

func PrintAuditEventsWide(writer io.Writer, events []*auditv1.Event) {
	w := tabwriter.NewWriter(writer, 20, 0, 0, ' ', tabwriter.DiscardEmptyColumns)
	defer w.Flush()

	for _, event := range events {
		duration := event.StageTimestamp.Time.Sub(event.RequestReceivedTimestamp.Time)
		code := int32(0)
		if event.ResponseStatus != nil {
			code = event.ResponseStatus.Code
		}
		fmt.Fprintf(w, "%s (%v) [%s][%s] [%d]\t %s\t [%s]\n", event.RequestReceivedTimestamp.Format("15:04:05"), event.AuditID, strings.ToUpper(event.Verb), duration, code, event.RequestURI, event.User.Username)
	}
}

func PrintTopByVerbAuditEvents(writer io.Writer, events []*auditv1.Event) {
	countVerbs := map[string][]*auditv1.Event{}

	for _, event := range events {
		countVerbs[event.Verb] = append(countVerbs[event.Verb], event)
	}

	result := map[string][]*eventWithCounter{}

	for verb, eventList := range countVerbs {
		countedEvents := []*eventWithCounter{}
		for _, event := range eventList {
			found := false
			for i, countedEvent := range countedEvents {
				if countedEvent.event.RequestURI == event.RequestURI && countedEvent.event.User.Username == event.User.Username {
					countedEvents[i].count += 1
					found = true
					break
				}
			}
			if !found {
				countedEvents = append(countedEvents, &eventWithCounter{event: event, count: 0})
			}
		}

		sort.Slice(countedEvents, func(i, j int) bool {
			return countedEvents[i].count >= countedEvents[j].count
		})
		if len(countedEvents) <= 5 {
			result[verb] = countedEvents
			continue
		}
		result[verb] = countedEvents[0:5]
	}

	w := tabwriter.NewWriter(writer, 20, 0, 0, ' ', tabwriter.DiscardEmptyColumns)
	defer w.Flush()

	for verb, eventWithCounter := range result {
		fmt.Fprintf(w, "\nTop 5 %q:\n", strings.ToUpper(verb))
		PrintAuditEventsWithCount(writer, eventWithCounter)
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
			newEvents, err := GetEvents(filepath.Join(auditFilename, info.Name()))
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
