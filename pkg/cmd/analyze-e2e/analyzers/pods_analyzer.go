package analyzers

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

type PodsAnalyzer struct{}

func (*PodsAnalyzer) Analyze(content []byte) (string, error) {
	manifestObj, err := runtime.Decode(unstructured.UnstructuredJSONScheme, content)
	if err != nil {
		return "", err
	}
	manifestUnstructured := manifestObj.(*unstructured.UnstructuredList)

	writer := &bytes.Buffer{}
	w := tabwriter.NewWriter(writer, 70, 0, 0, ' ', tabwriter.DiscardEmptyColumns)

	err = manifestUnstructured.EachListItem(func(object runtime.Object) error {
		u := object.(*unstructured.Unstructured)
		conditions, _, err := unstructured.NestedSlice(u.Object, "status", "conditions")
		if err != nil {
			return err
		}
		resultConditions := []string{}
		for _, condition := range conditions {
			condType, _, err := unstructured.NestedString(condition.(map[string]interface{}), "type")
			if err != nil {
				return err
			}
			condStatus, _, err := unstructured.NestedString(condition.(map[string]interface{}), "status")
			if err != nil {
				return err
			}
			resultConditions = append(resultConditions, fmt.Sprintf("%s=%s", condType, condStatus))
		}

		resultContainers := []string{}
		containerStatuses, _, err := unstructured.NestedSlice(u.Object, "status", "containerStatuses")
		if err != nil {
			return err
		}
		for _, status := range containerStatuses {
			exitCode, exists, err := unstructured.NestedInt64(status.(map[string]interface{}), "lastState", "terminated", "exitCode")
			if !exists {
				continue
			}
			if err != nil {
				return err
			}
			if exitCode != 0 {
				restartCount, _, _ := unstructured.NestedInt64(status.(map[string]interface{}), "restartCount")
				message, _, err := unstructured.NestedString(status.(map[string]interface{}), "lastState", "terminated", "message")
				if err != nil {
					return err
				}
				containerName, _, err := unstructured.NestedString(status.(map[string]interface{}), "name")
				if err != nil {
					return err
				}
				resultContainers = append(resultContainers, fmt.Sprintf("  [!] Container %q restarted %d times, last exit %d caused by:\n   %s\n", containerName, restartCount, exitCode, message))
			}
		}

		fmt.Fprintf(w, "%s\t%s\n", u.GetName(), strings.Join(resultConditions, ", "))

		if len(resultContainers) > 0 {
			fmt.Fprintf(w, "%s\n", strings.Join(resultContainers, ", "))
		}
		return nil
	})

	w.Flush()

	return writer.String(), err
}
