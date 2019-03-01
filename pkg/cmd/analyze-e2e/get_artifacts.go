package analyze_e2e

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

type Analyzer interface {
	Analyze(content []byte) (string, error)
}

type AnalyzeResult struct {
	ArtifactName string
	Output       string
	Error        error
}

func GetArtifacts(baseURL string) ([]AnalyzeResult, error) {
	out := []AnalyzeResult{}
	for name, analyzer := range artifactsToAnalyzeList {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client := &http.Client{Transport: tr}
		artifactFileName := getArtifactStorageURL(baseURL, name)
		response, err := client.Get(artifactFileName)
		if err != nil {
			return nil, err
		}
		defer func() {
			if err := response.Body.Close(); err != nil {
			}
		}()
		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to get %q, HTTP code: %d", artifactFileName, response.StatusCode)
		}
		content, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return nil, err
		}
		result, err := analyzer.Analyze(content)
		out = append(out, AnalyzeResult{ArtifactName: name, Output: result, Error: err})
	}
	return out, nil
}

func getArtifactStorageURL(baseURL, artifactName string) string {
	return strings.TrimSuffix(strings.Replace(baseURL, "gcsweb-ci.svc.ci.openshift.org/gcs", "storage.googleapis.com", 1), "/") + "/" + artifactName
}
