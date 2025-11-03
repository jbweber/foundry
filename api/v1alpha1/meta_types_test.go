package v1alpha1

import (
	"encoding/json"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestTime_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		time     Time
		expected string
	}{
		{
			name:     "zero time returns null",
			time:     Time{},
			expected: "null",
		},
		{
			name:     "valid time returns RFC3339",
			time:     Time{Time: time.Date(2025, 11, 3, 10, 30, 0, 0, time.UTC)},
			expected: `"2025-11-03T10:30:00Z"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.time.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON() error = %v", err)
			}
			if string(got) != tt.expected {
				t.Errorf("MarshalJSON() = %s, want %s", string(got), tt.expected)
			}
		})
	}
}

func TestTime_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantZero  bool
		wantError bool
	}{
		{
			name:     "null returns zero time",
			input:    "null",
			wantZero: true,
		},
		{
			name:     "empty string returns zero time",
			input:    `""`,
			wantZero: true,
		},
		{
			name:     "valid RFC3339 time",
			input:    `"2025-11-03T10:30:00Z"`,
			wantZero: false,
		},
		{
			name:      "invalid format errors",
			input:     `"not-a-time"`,
			wantError: true,
		},
		{
			name:      "invalid JSON errors",
			input:     `{invalid}`,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Time
			err := got.UnmarshalJSON([]byte(tt.input))

			if tt.wantError {
				if err == nil {
					t.Error("UnmarshalJSON() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("UnmarshalJSON() unexpected error = %v", err)
			}

			if tt.wantZero {
				if !got.IsZero() {
					t.Errorf("UnmarshalJSON() expected zero time, got %v", got.Time)
				}
			} else {
				if got.IsZero() {
					t.Error("UnmarshalJSON() expected non-zero time, got zero")
				}
			}
		})
	}
}

func TestTime_JSON_RoundTrip(t *testing.T) {
	original := Time{Time: time.Date(2025, 11, 3, 10, 30, 45, 0, time.UTC)}

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}

	// Unmarshal
	var decoded Time
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}

	// Compare (truncate to seconds since RFC3339 doesn't include nanoseconds by default)
	if !original.Truncate(time.Second).Equal(decoded.Truncate(time.Second)) {
		t.Errorf("Round trip failed: original = %v, decoded = %v", original, decoded)
	}
}

func TestTime_MarshalYAML(t *testing.T) {
	tests := []struct {
		name     string
		time     Time
		expected string
	}{
		{
			name:     "zero time returns nil",
			time:     Time{},
			expected: "null\n",
		},
		{
			name:     "valid time returns RFC3339",
			time:     Time{Time: time.Date(2025, 11, 3, 10, 30, 0, 0, time.UTC)},
			expected: "\"2025-11-03T10:30:00Z\"\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := yaml.Marshal(tt.time)
			if err != nil {
				t.Fatalf("MarshalYAML() error = %v", err)
			}
			if string(got) != tt.expected {
				t.Errorf("MarshalYAML() = %s, want %s", string(got), tt.expected)
			}
		})
	}
}

func TestTime_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantZero  bool
		wantError bool
	}{
		{
			name:     "null returns zero time",
			input:    "null",
			wantZero: true,
		},
		{
			name:     "empty string returns zero time",
			input:    "",
			wantZero: true,
		},
		{
			name:     "valid RFC3339 time",
			input:    "2025-11-03T10:30:00Z",
			wantZero: false,
		},
		{
			name:      "invalid format errors",
			input:     "not-a-time",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Time
			err := yaml.Unmarshal([]byte(tt.input), &got)

			if tt.wantError {
				if err == nil {
					t.Error("UnmarshalYAML() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("UnmarshalYAML() unexpected error = %v", err)
			}

			if tt.wantZero {
				if !got.IsZero() {
					t.Errorf("UnmarshalYAML() expected zero time, got %v", got.Time)
				}
			} else {
				if got.IsZero() {
					t.Error("UnmarshalYAML() expected non-zero time, got zero")
				}
			}
		})
	}
}

func TestTime_YAML_RoundTrip(t *testing.T) {
	original := Time{Time: time.Date(2025, 11, 3, 10, 30, 45, 0, time.UTC)}

	// Marshal
	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}

	// Unmarshal
	var decoded Time
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}

	// Compare (truncate to seconds since RFC3339 doesn't include nanoseconds by default)
	if !original.Truncate(time.Second).Equal(decoded.Truncate(time.Second)) {
		t.Errorf("Round trip failed: original = %v, decoded = %v", original, decoded)
	}
}

func TestTypeMeta_DeepCopy(t *testing.T) {
	tests := []struct {
		name  string
		input *TypeMeta
	}{
		{
			name:  "nil returns nil",
			input: nil,
		},
		{
			name: "copy is independent",
			input: &TypeMeta{
				Kind:       "VirtualMachine",
				APIVersion: "foundry.cofront.xyz/v1alpha1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			copy := tt.input.DeepCopy()

			if tt.input == nil {
				if copy != nil {
					t.Error("DeepCopy() of nil should return nil")
				}
				return
			}

			if copy == nil {
				t.Fatal("DeepCopy() returned nil for non-nil input")
			}

			// Verify values match
			if copy.Kind != tt.input.Kind {
				t.Errorf("Kind mismatch: got %s, want %s", copy.Kind, tt.input.Kind)
			}
			if copy.APIVersion != tt.input.APIVersion {
				t.Errorf("APIVersion mismatch: got %s, want %s", copy.APIVersion, tt.input.APIVersion)
			}

			// Verify independence (modifying copy doesn't affect original)
			copy.Kind = "Modified"
			if tt.input.Kind == "Modified" {
				t.Error("Modifying copy affected original")
			}
		})
	}
}

func TestObjectMeta_DeepCopy(t *testing.T) {
	tests := []struct {
		name  string
		input *ObjectMeta
	}{
		{
			name:  "nil returns nil",
			input: nil,
		},
		{
			name: "copy with maps is independent",
			input: &ObjectMeta{
				Name: "test-vm",
				Labels: map[string]string{
					"app": "web",
					"env": "prod",
				},
				Annotations: map[string]string{
					"description": "test VM",
				},
				UID:        "12345",
				Generation: 5,
			},
		},
		{
			name: "copy with nil maps",
			input: &ObjectMeta{
				Name:   "test-vm",
				Labels: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			copy := tt.input.DeepCopy()

			if tt.input == nil {
				if copy != nil {
					t.Error("DeepCopy() of nil should return nil")
				}
				return
			}

			if copy == nil {
				t.Fatal("DeepCopy() returned nil for non-nil input")
			}

			// Verify values match
			if copy.Name != tt.input.Name {
				t.Errorf("Name mismatch: got %s, want %s", copy.Name, tt.input.Name)
			}
			if copy.UID != tt.input.UID {
				t.Errorf("UID mismatch: got %s, want %s", copy.UID, tt.input.UID)
			}
			if copy.Generation != tt.input.Generation {
				t.Errorf("Generation mismatch: got %d, want %d", copy.Generation, tt.input.Generation)
			}

			// Verify map independence
			if tt.input.Labels != nil {
				if len(copy.Labels) != len(tt.input.Labels) {
					t.Errorf("Labels length mismatch: got %d, want %d", len(copy.Labels), len(tt.input.Labels))
				}
				for k, v := range tt.input.Labels {
					if copy.Labels[k] != v {
						t.Errorf("Label %s mismatch: got %s, want %s", k, copy.Labels[k], v)
					}
				}

				// Modify copy and verify original unchanged
				copy.Labels["new"] = "value"
				if _, exists := tt.input.Labels["new"]; exists {
					t.Error("Modifying copy.Labels affected original")
				}
			}

			if tt.input.Annotations != nil {
				if len(copy.Annotations) != len(tt.input.Annotations) {
					t.Errorf("Annotations length mismatch: got %d, want %d", len(copy.Annotations), len(tt.input.Annotations))
				}

				// Modify copy and verify original unchanged
				copy.Annotations["new"] = "annotation"
				if _, exists := tt.input.Annotations["new"]; exists {
					t.Error("Modifying copy.Annotations affected original")
				}
			}
		})
	}
}

func TestTime_DeepCopy(t *testing.T) {
	tests := []struct {
		name  string
		input *Time
	}{
		{
			name:  "nil returns nil",
			input: nil,
		},
		{
			name:  "copy is independent",
			input: &Time{Time: time.Date(2025, 11, 3, 10, 30, 0, 0, time.UTC)},
		},
		{
			name:  "zero time",
			input: &Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			copy := tt.input.DeepCopy()

			if tt.input == nil {
				if copy != nil {
					t.Error("DeepCopy() of nil should return nil")
				}
				return
			}

			if copy == nil {
				t.Fatal("DeepCopy() returned nil for non-nil input")
			}

			// Verify values match
			if !copy.Equal(tt.input.Time) {
				t.Errorf("Time mismatch: got %v, want %v", copy.Time, tt.input.Time)
			}

			// Verify independence
			copy.Time = time.Now()
			if copy.Equal(tt.input.Time) {
				t.Error("Modifying copy affected original")
			}
		})
	}
}

func TestCondition_DeepCopy(t *testing.T) {
	tests := []struct {
		name  string
		input *Condition
	}{
		{
			name:  "nil returns nil",
			input: nil,
		},
		{
			name: "copy is independent",
			input: &Condition{
				Type:               "Ready",
				Status:             ConditionTrue,
				ObservedGeneration: 5,
				LastTransitionTime: Time{Time: time.Now()},
				Reason:             "VMReady",
				Message:            "VM is ready",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			copy := tt.input.DeepCopy()

			if tt.input == nil {
				if copy != nil {
					t.Error("DeepCopy() of nil should return nil")
				}
				return
			}

			if copy == nil {
				t.Fatal("DeepCopy() returned nil for non-nil input")
			}

			// Verify values match
			if copy.Type != tt.input.Type {
				t.Errorf("Type mismatch: got %s, want %s", copy.Type, tt.input.Type)
			}
			if copy.Status != tt.input.Status {
				t.Errorf("Status mismatch: got %s, want %s", copy.Status, tt.input.Status)
			}
			if copy.Reason != tt.input.Reason {
				t.Errorf("Reason mismatch: got %s, want %s", copy.Reason, tt.input.Reason)
			}
			if copy.Message != tt.input.Message {
				t.Errorf("Message mismatch: got %s, want %s", copy.Message, tt.input.Message)
			}
			if copy.ObservedGeneration != tt.input.ObservedGeneration {
				t.Errorf("ObservedGeneration mismatch: got %d, want %d", copy.ObservedGeneration, tt.input.ObservedGeneration)
			}

			// Verify independence
			copy.Type = "Modified"
			if tt.input.Type == "Modified" {
				t.Error("Modifying copy affected original")
			}
		})
	}
}

func TestConditionStatus_Constants(t *testing.T) {
	if ConditionTrue != "True" {
		t.Errorf("ConditionTrue = %s, want True", ConditionTrue)
	}
	if ConditionFalse != "False" {
		t.Errorf("ConditionFalse = %s, want False", ConditionFalse)
	}
	if ConditionUnknown != "Unknown" {
		t.Errorf("ConditionUnknown = %s, want Unknown", ConditionUnknown)
	}
}
