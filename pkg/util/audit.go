package util

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/schollz/progressbar"
)

// AuditEventsMap type converts the slice of audit events to map[URI]map[VERB]
type AuditEventsMap struct {
	uriVerbsIndex map[string]map[string][]Event
	sync.Mutex
}

func (m *AuditEventsMap) Add(event Event) {
	m.Lock()
	defer m.Unlock()
	if m.uriVerbsIndex == nil {
		m.uriVerbsIndex = map[string]map[string][]Event{}
	}
	_, found := m.uriVerbsIndex[event.RequestURI]
	if !found {
		m.uriVerbsIndex[event.RequestURI] = map[string][]Event{}
	}
	m.uriVerbsIndex[event.RequestURI][event.Verb] = append(m.uriVerbsIndex[event.RequestURI][event.Verb], event)
}

// SearchEventsContains searches audit events that contains specific keyword. Additionally can be filtered using verbs.
func (m *AuditEventsMap) SearchEventsContains(keyword string, verb string) []Event {
	result := []Event{}
	for u := range m.uriVerbsIndex {
		if !strings.Contains(u, keyword) {
			continue
		}
		if len(verb) > 0 {
			if m.uriVerbsIndex[u][verb] != nil {
				result = append(result, m.uriVerbsIndex[u][verb]...)
			}
			continue
		}
		for v := range m.uriVerbsIndex[u] {
			result = append(result, m.uriVerbsIndex[u][v]...)
		}
	}
	return result
}

// SearchEventsContains searches audit events that matches given regexp. Additionally can be filtered using verbs.
func (m *AuditEventsMap) SearchEventsRegexp(regexpInput string, verb string) ([]Event, error) {
	re, err := regexp.Compile(regexpInput)
	if err != nil {
		return nil, err
	}
	result := []Event{}
	for u := range m.uriVerbsIndex {
		if !re.MatchString(u) {
			continue
		}
		if len(verb) > 0 {
			if m.uriVerbsIndex[u][verb] != nil {
				result = append(result, m.uriVerbsIndex[u][verb]...)
			}
			continue
		}
		for v := range m.uriVerbsIndex[u] {
			result = append(result, m.uriVerbsIndex[u][v]...)
		}
	}
	return result, nil
}

// GetAuditEventsFromURL fetch the remote audit log gzip file and convert it to AuditEventsMap
func GetAuditEventsFromURL(auditURL string) (*AuditEventsMap, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	response, err := client.Get(auditURL)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
		}
	}()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get %q, HTTP code: %d", auditURL, response.StatusCode)
	}

	// print progress bar for people with slow internet (Maciej :-)
	bar := progressbar.NewOptions(
		100,
		progressbar.OptionSetRenderBlankState(true),
	)

	// make sure we finish the bar in case of any error
	barFinished := false
	finishBar := func() {
		if barFinished {
			return
		}
		barFinished = true
		bar.Finish()
		bar.Clear()
		fmt.Println()
	}
	defer finishBar()

	// we recieve the gzipped audit file, so we have to pipe it trough gzip reader
	responseBody, err := gzip.NewReader(response.Body)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(responseBody)
	auditEventIndex := &AuditEventsMap{}

	// each line in audit file use following format: `hostname {JSON}`, we are not interested in hostname,
	// so lets parse out the events.
	var bytesRead int64
	for scanner.Scan() {
		line := scanner.Bytes()

		// strip the hostname part
		hostnameEndPos := bytes.Index(line, []byte(" "))
		if hostnameEndPos == -1 {
			// oops something is wrong in the file?
			continue
		}

		auditBytes := line[hostnameEndPos:]

		// shame, shame shame... we have to copy out the apiserver/apis/audit/v1alpha1.Event because adding it as dependency
		// will cause mess in flags...
		eventObj := Event{}
		if err := json.Unmarshal(auditBytes, &eventObj); err != nil {
			return nil, fmt.Errorf("unable to decode: %s to audit event: %v", string(auditBytes), err)
		}

		// normalize the request URI (we are not interesting in GET params with resourceVersion/etc.?)
		if pos := strings.Index(eventObj.RequestURI, "?"); pos > 0 {
			eventObj.RequestURI = eventObj.RequestURI[0:pos]
		}

		// Add to index
		auditEventIndex.Add(eventObj)

		// update progress bar
		bytesRead += int64(len(line))
		if bytesRead >= response.ContentLength/100 {
			bar.Add(1)
			bytesRead = 0
		}
	}

	// stop progress bar
	finishBar()

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return auditEventIndex, nil
}

func PrintSearchAuditEvents(writer io.Writer, events []Event) {
	w := tabwriter.NewWriter(writer, 20, 0, 0, ' ', tabwriter.DiscardEmptyColumns)
	defer w.Flush()

	// sort events by time
	sort.Slice(events, func(i, j int) bool {
		return events[i].RequestReceivedTimestamp.Time.Before(events[j].RequestReceivedTimestamp.Time)
	})

	for _, event := range events {
		duration := event.StageTimestamp.Time.Sub(event.RequestReceivedTimestamp.Time)
		fmt.Fprintf(w, "%s [%s][%s] [%d]\t %s\t [%s]\n", event.RequestReceivedTimestamp.Format("15:04:05"), strings.ToUpper(event.Verb), duration, event.ResponseStatus.Code, event.RequestURI, event.User.Username)
	}
}

func PrintAllAuditEvents(writer io.Writer, auditEventsIndex *AuditEventsMap) {
	w := tabwriter.NewWriter(writer, 20, 0, 0, ' ', tabwriter.DiscardEmptyColumns)
	defer w.Flush()

	keys := []string{}
	for k := range auditEventsIndex.uriVerbsIndex {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	type eventMessageRecord struct {
		count int
		msg   string
	}

	totalEventCount := 0
	eventMessages := []eventMessageRecord{}

	for _, auditEventURI := range keys {
		for verb, events := range auditEventsIndex.uriVerbsIndex[auditEventURI] {
			eventMsg := fmt.Sprintf("%s \t(x%d)\t %s\t\n", strings.ToUpper(verb), len(events), auditEventURI)
			totalEventCount += len(events)
			eventMessages = append(eventMessages, eventMessageRecord{
				count: len(events),
				msg:   eventMsg,
			})
			fmt.Fprintf(w, eventMsg)
		}
	}

	// sort events by cap
	sort.Slice(eventMessages, func(i, j int) bool {
		return eventMessages[i].count >= eventMessages[j].count
	})

	fmt.Fprintf(w, "\nFound %d audit events.\n", totalEventCount)

	if len(eventMessages) > 10 {
		fmt.Fprintf(w, "\tTop 10 audit events: \n")
		for _, e := range eventMessages[0:10] {
			fmt.Fprintf(w, e.msg)
		}
	}
}
