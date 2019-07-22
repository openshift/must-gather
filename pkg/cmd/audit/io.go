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
		if _, err := fmt.Fprintf(w, "%s [%6s][%12s] [%3d]\t %s\t [%s]\n",
			event.RequestReceivedTimestamp.Format("15:04:05"),
			strings.ToUpper(event.Verb),
			duration,
			code,
			event.RequestURI,
			event.User.Username); err != nil {
			panic(err)
		}
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
		if _, err := fmt.Fprintf(w, "%8s [%12s] [%3d]\t %s\t [%s]\n",
			fmt.Sprintf("%dx", event.count),
			duration,
			code,
			event.event.RequestURI,
			event.event.User.Username); err != nil {
			panic(err)
		}
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
		if _, err := fmt.Fprintf(w, "%s (%v) [%s][%s] [%d]\t %s\t [%s]\n",
			event.RequestReceivedTimestamp.Format("15:04:05"),
			event.AuditID,
			strings.ToUpper(event.Verb),
			duration,
			code,
			event.RequestURI,
			event.User.Username); err != nil {
			panic(err)
		}
	}
}

func PrintTopByUserAuditEvents(writer io.Writer, events []*auditv1.Event) {
	countUsers := map[string][]*auditv1.Event{}

	for _, event := range events {
		countUsers[event.User.Username] = append(countUsers[event.User.Username], event)
	}

	type userWithCount struct {
		name  string
		count int
	}
	result := []userWithCount{}

	for username, userEvents := range countUsers {
		result = append(result, userWithCount{name: username, count: len(userEvents)})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].count >= result[j].count
	})

	w := tabwriter.NewWriter(writer, 20, 0, 0, ' ', tabwriter.DiscardEmptyColumns)
	defer w.Flush()

	if len(result) > 10 {
		result = result[0:10]
	}

	for _, r := range result {
		fmt.Fprintf(w, "%dx\t %s\n", r.count, r.name)
	}
}

func PrintTopByResourceAuditEvents(writer io.Writer, events []*auditv1.Event) {
	result := map[string]int64{}

	for _, event := range events {
		noParamsUri := strings.Split(event.RequestURI, "?")
		uri := strings.Split(strings.TrimPrefix(noParamsUri[0], "/"), "/")
		if len(uri) == 0 {
			continue
		}

		switch uri[0] {
		// kube api
		case "api":
			switch len(uri) {
			case 1, 2:
				continue
			case 3:
				// /api/v1/nodes -> v1/nodes
				result[strings.Join(uri[1:3], "/")]++
			default:
				// /api/v1/namespaces/foo/secrets -> v1/secrets
				if uri[2] == "namespaces" && len(uri) >= 5 {
					result[uri[1]+"/"+uri[4]]++
					continue
				}
				result[strings.Join(uri[1:3], "/")]++
			}
		case "apis":
			switch len(uri) {
			case 1, 2, 3:
				continue
			case 4:
				result[strings.Join(uri[1:4], "/")]++
			default:
				if uri[3] == "namespaces" && len(uri) >= 6 {
					result[uri[1]+"/"+uri[5]]++
					continue
				}
				result[strings.Join(uri[1:4], "/")]++
			}
		}
	}

	w := tabwriter.NewWriter(writer, 20, 0, 0, ' ', tabwriter.DiscardEmptyColumns)
	defer w.Flush()

	type sortedResultItem struct {
		resource string
		count    int64
	}

	sortedResult := []sortedResultItem{}

	for resource, count := range result {
		sortedResult = append(sortedResult, sortedResultItem{resource: resource, count: count})
	}
	sort.Slice(sortedResult, func(i, j int) bool {
		return sortedResult[i].count >= sortedResult[j].count
	})

	for _, item := range sortedResult {
		fmt.Fprintf(w, "%dx\t %s\n", item.count, item.resource)
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
				countedEvents = append(countedEvents, &eventWithCounter{event: event, count: 1})
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
		line := 0
		for scanner.Scan() {
			line++
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
				return nil, fmt.Errorf("unable to decode %q line %d: %s to audit event: %v", auditFilename, line, string(auditBytes), err)
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
			newEvents, err := GetEvents(path)
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
