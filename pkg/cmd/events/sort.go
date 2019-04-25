package events

import corev1 "k8s.io/api/core/v1"

type byTime []*corev1.Event

func (s byTime) Len() int {
	return len(s)
}
func (s byTime) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s byTime) Less(i, j int) bool {
	return s[i].FirstTimestamp.Before(&s[j].FirstTimestamp)
}

type byFrequency []*corev1.Event

func (s byFrequency) Len() int {
	return len(s)
}
func (s byFrequency) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s byFrequency) Less(i, j int) bool {
	return s[i].Count < s[j].Count
}
