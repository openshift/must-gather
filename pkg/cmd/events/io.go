package events

import (
	"fmt"
	"io"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

func PrintComponents(writer io.Writer, events []*corev1.Event) error {
	components := sets.NewString()
	for _, event := range events {
		if !components.Has(event.Source.Component) {
			components.Insert(event.Source.Component)
		}
	}

	if _, err := fmt.Fprintln(writer, strings.Join(components.List(), ",")); err != nil {
		return err
	}

	return nil
}

func PrintEvents(writer io.Writer, events []*corev1.Event) error {
	for _, event := range events {
		message := event.Message
		message = strings.Replace(message, "\\\\", "\\", -1)
		message = strings.Replace(message, "\\n", "\n\t", -1)
		message = strings.Replace(message, "\\", "\"", -1)
		message = strings.Replace(message, `"""`, `"`, -1)
		message = strings.Replace(message, "\t", "\t", -1)

		if _, err := fmt.Fprintf(writer, "%s (%d) %q %s %s\n", event.FirstTimestamp.Format("15:04:05"), event.Count, event.InvolvedObject.Namespace, event.Reason, message); err != nil {
			return err
		}
	}

	return nil
}

func PrintEventsWide(writer io.Writer, events []*corev1.Event) error {
	return PrintEvents(writer, events)
}
