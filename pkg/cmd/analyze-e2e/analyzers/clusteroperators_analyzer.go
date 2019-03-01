package analyzers

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

type ClusterOperatorsAnalyzer struct{}

func (*ClusterOperatorsAnalyzer) Analyze(content []byte) (string, error) {
	manifestObj, err := runtime.Decode(unstructured.UnstructuredJSONScheme, content)
	if err != nil {
		return "", err
	}
	manifestUnstructured := manifestObj.(*unstructured.UnstructuredList)

	writer := &bytes.Buffer{}
	w := tabwriter.NewWriter(writer, 60, 0, 0, ' ', tabwriter.DiscardEmptyColumns)

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
		sort.Strings(resultConditions)
		fmt.Fprintf(w, "%s\t%s\n", u.GetName(), strings.Join(resultConditions, ", "))
		return nil
	})

	w.Flush()

	return writer.String(), err
}
