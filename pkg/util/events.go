package util

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/xeonx/timeago"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

func GetEventBytesFromLocalFile(eventFileName string) ([]byte, error) {
	return ioutil.ReadFile(eventFileName)
}

func GetEventBytesFromURL(eventFileURL string) ([]byte, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	response, err := client.Get(eventFileURL)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
		}
	}()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get %q, HTTP code: %d", eventFileURL, response.StatusCode)
	}
	return ioutil.ReadAll(response.Body)
}

// PrintEvents prints the given events JSON in human readable way.
// If componentName is provided, only events related to that component are printed out (use '*' to print all events).
// If printComponents is provided we only print the component names.
func PrintEvents(writer io.Writer, eventBytes []byte, absoluteTime bool, componentName string, printComponents bool) error {
	eventList := v1.EventList{}
	if err := json.Unmarshal(eventBytes, &eventList); err != nil {
		log.Fatal(err.Error())
	}

	sort.Slice(eventList.Items, func(i, j int) bool {
		return eventList.Items[i].FirstTimestamp.Before(&eventList.Items[j].FirstTimestamp)
	})

	englishFormat := timeago.English
	englishFormat.PastSuffix = " "
	w := tabwriter.NewWriter(writer, 60, 0, 0, ' ', tabwriter.DiscardEmptyColumns)

	components := sets.NewString()

	for _, item := range eventList.Items {
		if !components.Has(item.Source.Component) {
			components.Insert(item.Source.Component)
		}
		if printComponents {
			continue
		}
		if item.Source.Component != componentName && componentName != `*` {
			continue
		}
		message := item.Message
		humanTime := item.FirstTimestamp.Time.String()
		if !absoluteTime {
			humanTime := englishFormat.FormatReference(eventList.Items[0].FirstTimestamp.Time, item.FirstTimestamp.Time)
			if componentName == `*` {
				component := item.Source.Component
				if len(component) > 35 {
					component = component[0:35] + "..."
				}
				humanTime = component + "\t" + humanTime
			}
		}
		if _, err := fmt.Fprintf(w, "%s  %s\t%s\n", humanTime, item.Reason, message); err != nil {
			return err
		}
		if err := w.Flush(); err != nil {
			return err
		}
	}

	if printComponents {
		if _, err := fmt.Fprintln(writer, strings.Join(components.List(), ",")); err != nil {
			return err
		}
	}

	return nil
}
