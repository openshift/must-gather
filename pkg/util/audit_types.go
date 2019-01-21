package util

// TODO: Remove this once we can import apiserver apis.
// NOTE: This is dirty, but I can't get to import apiserver without messing up flags.

import (
	authnv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// Event captures all the information that can be included in an API audit log.
type Event struct {
	// Time the request reached the apiserver.
	Timestamp metav1.Time `json:"timestamp" protobuf:"bytes,3,opt,name=timestamp"`
	// Unique audit ID, generated for each request.
	AuditID types.UID `json:"auditID" protobuf:"bytes,4,opt,name=auditID,casttype=k8s.io/apimachinery/pkg/types.UID"`

	// RequestURI is the request URI as sent by the client to a server.
	RequestURI string `json:"requestURI" protobuf:"bytes,6,opt,name=requestURI"`
	// Verb is the kubernetes verb associated with the request.
	// For non-resource requests, this is the lower-cased HTTP method.
	Verb string `json:"verb" protobuf:"bytes,7,opt,name=verb"`
	// Authenticated user information.
	User authnv1.UserInfo `json:"user" protobuf:"bytes,8,opt,name=user"`
	// Source IPs, from where the request originated and intermediate proxies.
	// +optional
	SourceIPs []string `json:"sourceIPs,omitempty" protobuf:"bytes,10,rep,name=sourceIPs"`
	// The response status, populated even when the ResponseObject is not a Status type.
	// For successful responses, this will only include the Code and StatusSuccess.
	// For non-status type error responses, this will be auto-populated with the error Message.
	// +optional
	ResponseStatus *metav1.Status `json:"responseStatus,omitempty" protobuf:"bytes,12,opt,name=responseStatus"`

	// API object from the request, in JSON format. The RequestObject is recorded as-is in the request
	// (possibly re-encoded as JSON), prior to version conversion, defaulting, admission or
	// merging. It is an external versioned object type, and may not be a valid object on its own.
	// Omitted for non-resource requests.  Only logged at Request Level and higher.
	// +optional
	RequestObject *runtime.Unknown `json:"requestObject,omitempty" protobuf:"bytes,13,opt,name=requestObject"`
	// API object returned in the response, in JSON. The ResponseObject is recorded after conversion
	// to the external type, and serialized as JSON.  Omitted for non-resource requests.  Only logged
	// at Response Level.
	// +optional
	ResponseObject *runtime.Unknown `json:"responseObject,omitempty" protobuf:"bytes,14,opt,name=responseObject"`
	// Time the request reached the apiserver.
	// +optional
	RequestReceivedTimestamp metav1.MicroTime `json:"requestReceivedTimestamp" protobuf:"bytes,15,opt,name=requestReceivedTimestamp"`
	// Time the request reached current audit stage.
	// +optional
	StageTimestamp metav1.MicroTime `json:"stageTimestamp" protobuf:"bytes,16,opt,name=stageTimestamp"`

	// Annotations is an unstructured key value map stored with an audit event that may be set by
	// plugins invoked in the request serving chain, including authentication, authorization and
	// admission plugins. Keys should uniquely identify the informing component to avoid name
	// collisions (e.g. podsecuritypolicy.admission.k8s.io/policy). Values should be short. Annotations
	// are included in the Metadata level.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty" protobuf:"bytes,17,rep,name=annotations"`
}
