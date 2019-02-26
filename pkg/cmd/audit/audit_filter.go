package audit

import (
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
)

type AuditFilter interface {
	FilterEvents(events ...*auditv1.Event) []*auditv1.Event
}

type AuditFilters []AuditFilter

func (f AuditFilters) FilterEvents(events ...*auditv1.Event) []*auditv1.Event {
	ret := make([]*auditv1.Event, len(events), len(events))
	copy(ret, events)

	for _, filter := range f {
		ret = filter.FilterEvents(ret...)
	}

	return ret
}

type FilterByFailures struct {
}

func (f *FilterByFailures) FilterEvents(events ...*auditv1.Event) []*auditv1.Event {
	ret := []*auditv1.Event{}
	for i := range events {
		event := events[i]
		if event.ResponseStatus == nil {
			continue
		}
		if event.ResponseStatus.Code > 299 {
			ret = append(ret, event)
		}
	}

	return ret
}

type FilterByNamespaces struct {
	Namespaces sets.String
}

func (f *FilterByNamespaces) FilterEvents(events ...*auditv1.Event) []*auditv1.Event {
	ret := []*auditv1.Event{}
	for i := range events {
		event := events[i]
		ns, _, _ := URIToParts(event.RequestURI)
		// check for an anti-match
		if f.Namespaces.Has("-" + ns) {
			continue
		}
		if f.Namespaces.Has(ns) {
			ret = append(ret, event)
		}
	}

	return ret
}

type FilterByNames struct {
	Names sets.String
}

func (f *FilterByNames) FilterEvents(events ...*auditv1.Event) []*auditv1.Event {
	ret := []*auditv1.Event{}
	for i := range events {
		event := events[i]
		_, _, name := URIToParts(event.RequestURI)
		// check for an anti-match
		if f.Names.Has("-" + name) {
			continue
		}
		if f.Names.Has(name) {
			ret = append(ret, event)
		}

		// if we didn't match, check the objectref
		if event.ObjectRef == nil {
			continue
		}
		// check for an anti-match
		if f.Names.Has("-" + event.ObjectRef.Name) {
			continue
		}
		if f.Names.Has(event.ObjectRef.Name) {
			ret = append(ret, event)
		}
	}

	return ret
}

type FilterByUIDs struct {
	UIDs sets.String
}

func (f *FilterByUIDs) FilterEvents(events ...*auditv1.Event) []*auditv1.Event {
	ret := []*auditv1.Event{}
	for i := range events {
		event := events[i]
		currUID := string(event.AuditID)
		// check for an anti-match
		if f.UIDs.Has("-" + currUID) {
			continue
		}
		if f.UIDs.Has(currUID) {
			ret = append(ret, event)
		}
	}

	return ret
}

type FilterByUser struct {
	Users sets.String
}

func (f *FilterByUser) FilterEvents(events ...*auditv1.Event) []*auditv1.Event {
	ret := []*auditv1.Event{}
	for i := range events {
		event := events[i]
		// check for an anti-match
		if f.Users.Has("-" + event.User.Username) {
			continue
		}
		if f.Users.Has(event.User.Username) {
			ret = append(ret, event)
		}
	}

	return ret
}

type FilterByVerbs struct {
	Verbs sets.String
}

func (f *FilterByVerbs) FilterEvents(events ...*auditv1.Event) []*auditv1.Event {
	ret := []*auditv1.Event{}
	for i := range events {
		event := events[i]
		// check for an anti-match
		if f.Verbs.Has("-" + event.Verb) {
			continue
		}
		if f.Verbs.Has(event.Verb) {
			ret = append(ret, event)
		}
	}

	return ret
}

type FilterByResources struct {
	Resources map[schema.GroupResource]bool
}

func (f *FilterByResources) FilterEvents(events ...*auditv1.Event) []*auditv1.Event {
	ret := []*auditv1.Event{}
	for i := range events {
		event := events[i]
		_, gvr, _ := URIToParts(event.RequestURI)
		antiMatch := schema.GroupResource{Resource: "-" + gvr.Resource, Group: gvr.Group}

		// check for an anti-match
		if f.Resources[antiMatch] {
			continue
		}
		if f.Resources[gvr.GroupResource()] {
			ret = append(ret, event)
		}

		// if we aren't an exact match, match on resource only if group is '*'
		// check for an anti-match
		antiMatched := false
		for currResource := range f.Resources {
			if currResource.Group == "*" && currResource.Resource == antiMatch.Resource {
				antiMatched = true
				break
			}
			if currResource.Resource == "-*" && currResource.Group == gvr.Group {
				antiMatched = true
				break
			}
		}
		if antiMatched {
			continue
		}

		for currResource := range f.Resources {
			if currResource.Group == "*" && currResource.Resource == "*" {
				ret = append(ret, event)
				break
			}
			if currResource.Group == "*" && currResource.Resource == gvr.Resource {
				ret = append(ret, event)
				break
			}
			if currResource.Resource == "*" && currResource.Group == gvr.Group {
				ret = append(ret, event)
				break
			}
		}
	}

	return ret
}

func URIToParts(uri string) (string, schema.GroupVersionResource, string) {
	ns := ""
	gvr := schema.GroupVersionResource{}
	name := ""

	if len(uri) >= 1 {
		if uri[0] == '/' {
			uri = uri[1:]
		}
	}
	parts := strings.Split(uri, "/")
	if len(parts) == 0 {
		return ns, gvr, name
	}
	// /api/v1/namespaces/<name>
	if parts[0] == "api" {
		if len(parts) >= 2 {
			gvr.Version = parts[1]
		}
		if len(parts) < 3 {
			return ns, gvr, name
		}

		if parts[2] != "namespaces" {
			gvr.Resource = parts[2]
			if len(parts) >= 4 {
				name = parts[3]
				return ns, gvr, name
			}
		}
		if len(parts) < 4 {
			return ns, gvr, name
		}

		ns = parts[3]
		if len(parts) >= 5 {
			gvr.Resource = parts[4]
		}
		if len(parts) >= 6 {
			name = parts[5]
		}
		return ns, gvr, name
	}

	if parts[0] != "apis" {
		return ns, gvr, name
	}

	// /apis/group/v1/namespaces/<name>
	if len(parts) >= 2 {
		gvr.Group = parts[1]
	}
	if len(parts) >= 3 {
		gvr.Version = parts[2]
	}
	if len(parts) < 4 {
		return ns, gvr, name
	}

	if parts[3] != "namespaces" {
		gvr.Resource = parts[3]
		if len(parts) >= 5 {
			name = parts[4]
			return ns, gvr, name
		}
	}
	if len(parts) < 5 {
		return ns, gvr, name
	}

	ns = parts[4]
	if len(parts) >= 6 {
		gvr.Resource = parts[5]
	}
	if len(parts) >= 7 {
		name = parts[6]
	}
	return ns, gvr, name
}
