// Package v1alpha1 contains API types for foundry.cofront.xyz/v1alpha1
//
// These types are hand-rolled to match Kubernetes API conventions without
// requiring k8s.io/apimachinery dependencies. When migrating to a full
// Kubernetes controller, these can be swapped with k8s.io/apimachinery types
// with minimal refactoring since field names and JSON tags match exactly.
package v1alpha1

import (
	"encoding/json"
	"time"

	"gopkg.in/yaml.v3"
)

// TypeMeta describes an individual object's type and API version.
// Matches k8s.io/apimachinery/pkg/apis/meta/v1.TypeMeta for future compatibility.
//
// +k8s:deepcopy-gen=true
type TypeMeta struct {
	// Kind is a string value representing the REST resource this object represents.
	// In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	// +optional
	Kind string `json:"kind,omitempty" yaml:"kind,omitempty"`

	// APIVersion defines the versioned schema of this representation of an object.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources
	// +optional
	APIVersion string `json:"apiVersion,omitempty" yaml:"apiVersion,omitempty"`
}

// ObjectMeta is metadata that all persisted resources must have.
// Simplified version matching core Kubernetes fields for future compatibility with
// k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta.
//
// +k8s:deepcopy-gen=true
type ObjectMeta struct {
	// Name must be unique within a namespace. Is required when creating resources.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names#names
	// +optional
	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	// Labels are key/value pairs attached to objects.
	// Labels can be used to organize and to select subsets of objects.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels
	// +optional
	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`

	// Annotations are unstructured key/value pairs that may be set by external tools.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations
	// +optional
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`

	// CreationTimestamp is a timestamp representing the server time when this object was created.
	// Populated by the system. Read-only.
	// +optional
	CreationTimestamp Time `json:"creationTimestamp,omitempty" yaml:"creationTimestamp,omitempty"`

	// UID is the unique identifier for this object.
	// Populated by the system. Read-only.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names#uids
	// +optional
	UID string `json:"uid,omitempty" yaml:"uid,omitempty"`

	// ResourceVersion is an opaque value that represents the internal version of this object.
	// Populated by the system. Read-only.
	// +optional
	ResourceVersion string `json:"resourceVersion,omitempty" yaml:"resourceVersion,omitempty"`

	// Generation is a sequence number representing a specific generation of the desired state.
	// Populated by the system. Read-only.
	// +optional
	Generation int64 `json:"generation,omitempty" yaml:"generation,omitempty"`
}

// Time is a wrapper around time.Time for RFC3339 JSON/YAML serialization.
// Matches k8s.io/apimachinery/pkg/apis/meta/v1.Time behavior.
//
// +k8s:deepcopy-gen=true
type Time struct {
	time.Time `json:"-" yaml:"-"`
}

// MarshalJSON implements the json.Marshaler interface.
// Returns RFC3339 formatted timestamp or null for zero values.
func (t Time) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(t.Time.Format(time.RFC3339))
}

// UnmarshalJSON implements the json.Unmarshaler interface.
// Parses RFC3339 formatted timestamp or null.
func (t *Time) UnmarshalJSON(b []byte) error {
	if string(b) == "null" || string(b) == `""` {
		t.Time = time.Time{}
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return err
	}
	t.Time = parsed
	return nil
}

// MarshalYAML implements the yaml.Marshaler interface.
func (t Time) MarshalYAML() (interface{}, error) {
	if t.IsZero() {
		return nil, nil
	}
	return t.Time.Format(time.RFC3339), nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (t *Time) UnmarshalYAML(node *yaml.Node) error {
	if node.Value == "" || node.Value == "null" {
		t.Time = time.Time{}
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, node.Value)
	if err != nil {
		return err
	}
	t.Time = parsed
	return nil
}

// Condition contains details for the current condition of this object.
// Matches k8s.io/apimachinery/pkg/apis/meta/v1.Condition for future compatibility.
//
// +k8s:deepcopy-gen=true
type Condition struct {
	// Type of condition in CamelCase or in foo.example.com/CamelCase.
	// Many .condition.type values are consistent across resources like Available, but because arbitrary conditions can be
	// useful (see .node.status.conditions), the ability to deconflict is important.
	Type string `json:"type" yaml:"type"`

	// Status of the condition, one of True, False, Unknown.
	Status ConditionStatus `json:"status" yaml:"status"`

	// ObservedGeneration represents the .metadata.generation that the condition was set based upon.
	// For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date
	// with respect to the current state of the instance.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty" yaml:"observedGeneration,omitempty"`

	// LastTransitionTime is the last time the condition transitioned from one status to another.
	// This should be when the underlying condition changed. If that is not known, then using the time when the API field changed is acceptable.
	// +optional
	LastTransitionTime Time `json:"lastTransitionTime,omitempty" yaml:"lastTransitionTime,omitempty"`

	// Reason contains a programmatic identifier indicating the reason for the condition's last transition.
	// Producers of specific condition types may define expected values and meanings for this field,
	// and whether the values are considered a guaranteed API.
	// The value should be a CamelCase string.
	// This field may not be empty.
	// +optional
	Reason string `json:"reason,omitempty" yaml:"reason,omitempty"`

	// Message is a human-readable message indicating details about the transition.
	// This may be an empty string.
	// +optional
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}

// ConditionStatus represents the status of a condition.
type ConditionStatus string

const (
	// ConditionTrue means a resource is in the condition.
	ConditionTrue ConditionStatus = "True"
	// ConditionFalse means a resource is not in the condition.
	ConditionFalse ConditionStatus = "False"
	// ConditionUnknown means the condition status is currently unknown.
	ConditionUnknown ConditionStatus = "Unknown"
)

// DeepCopy creates a deep copy of TypeMeta.
func (in *TypeMeta) DeepCopy() *TypeMeta {
	if in == nil {
		return nil
	}
	out := new(TypeMeta)
	*out = *in
	return out
}

// DeepCopy creates a deep copy of ObjectMeta.
func (in *ObjectMeta) DeepCopy() *ObjectMeta {
	if in == nil {
		return nil
	}
	out := new(ObjectMeta)
	*out = *in

	// Deep copy maps
	if in.Labels != nil {
		out.Labels = make(map[string]string, len(in.Labels))
		for k, v := range in.Labels {
			out.Labels[k] = v
		}
	}
	if in.Annotations != nil {
		out.Annotations = make(map[string]string, len(in.Annotations))
		for k, v := range in.Annotations {
			out.Annotations[k] = v
		}
	}

	return out
}

// DeepCopy creates a deep copy of Time.
func (in *Time) DeepCopy() *Time {
	if in == nil {
		return nil
	}
	out := new(Time)
	*out = *in
	return out
}

// DeepCopy creates a deep copy of Condition.
func (in *Condition) DeepCopy() *Condition {
	if in == nil {
		return nil
	}
	out := new(Condition)
	*out = *in
	return out
}
