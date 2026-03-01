package events

import (
	"encoding/json"
	"testing"
	"time"
)

func boolPtr(b bool) *bool { return &b }

func validEvent() *Event {
	return &Event{
		EventID:    "evt-001",
		Type:       TypeIterationCompleted,
		Timestamp:  time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
		InstanceID: "inst-abc",
		Repo:       "services",
		Epic:       "ralph-hub",
		Data: &Data{
			Iteration:  1,
			DurationMs: 45000,
			Passed:     boolPtr(true),
			TaskID:     "ralph-hub-k7p",
		},
		Context: &Context{
			SessionID:        "sess-xyz",
			SessionStart:     time.Date(2026, 3, 1, 11, 0, 0, 0, time.UTC),
			MaxIterations:    20,
			CurrentIteration: 1,
			Status:           "running",
			CurrentPhase:     "implementing",
			Analytics: &Analytics{
				PassedCount:     1,
				FailedCount:     0,
				TasksClosed:     0,
				InitialReady:    3,
				CurrentReady:    3,
				AvgDurationMs:   45000,
				TotalDurationMs: 45000,
			},
		},
	}
}

func TestJSONRoundTrip(t *testing.T) {
	evt := validEvent()

	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Event
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Check top-level fields.
	if got.EventID != evt.EventID {
		t.Errorf("EventID = %q, want %q", got.EventID, evt.EventID)
	}
	if got.Type != evt.Type {
		t.Errorf("Type = %q, want %q", got.Type, evt.Type)
	}
	if !got.Timestamp.Equal(evt.Timestamp) {
		t.Errorf("Timestamp = %v, want %v", got.Timestamp, evt.Timestamp)
	}
	if got.InstanceID != evt.InstanceID {
		t.Errorf("InstanceID = %q, want %q", got.InstanceID, evt.InstanceID)
	}
	if got.Repo != evt.Repo {
		t.Errorf("Repo = %q, want %q", got.Repo, evt.Repo)
	}
	if got.Epic != evt.Epic {
		t.Errorf("Epic = %q, want %q", got.Epic, evt.Epic)
	}

	// Check Data.
	if got.Data == nil {
		t.Fatal("Data is nil after round-trip")
	}
	if got.Data.Iteration != evt.Data.Iteration {
		t.Errorf("Data.Iteration = %d, want %d", got.Data.Iteration, evt.Data.Iteration)
	}
	if got.Data.DurationMs != evt.Data.DurationMs {
		t.Errorf("Data.DurationMs = %d, want %d", got.Data.DurationMs, evt.Data.DurationMs)
	}
	if got.Data.Passed == nil || *got.Data.Passed != *evt.Data.Passed {
		t.Errorf("Data.Passed = %v, want %v", got.Data.Passed, evt.Data.Passed)
	}
	if got.Data.TaskID != evt.Data.TaskID {
		t.Errorf("Data.TaskID = %q, want %q", got.Data.TaskID, evt.Data.TaskID)
	}

	// Check Context.
	if got.Context == nil {
		t.Fatal("Context is nil after round-trip")
	}
	if got.Context.SessionID != evt.Context.SessionID {
		t.Errorf("Context.SessionID = %q, want %q", got.Context.SessionID, evt.Context.SessionID)
	}
	if got.Context.MaxIterations != evt.Context.MaxIterations {
		t.Errorf("Context.MaxIterations = %d, want %d", got.Context.MaxIterations, evt.Context.MaxIterations)
	}
	if got.Context.Status != evt.Context.Status {
		t.Errorf("Context.Status = %q, want %q", got.Context.Status, evt.Context.Status)
	}

	// Check Analytics.
	if got.Context.Analytics == nil {
		t.Fatal("Context.Analytics is nil after round-trip")
	}
	if got.Context.Analytics.PassedCount != evt.Context.Analytics.PassedCount {
		t.Errorf("Analytics.PassedCount = %d, want %d", got.Context.Analytics.PassedCount, evt.Context.Analytics.PassedCount)
	}
	if got.Context.Analytics.TotalDurationMs != evt.Context.Analytics.TotalDurationMs {
		t.Errorf("Analytics.TotalDurationMs = %d, want %d", got.Context.Analytics.TotalDurationMs, evt.Context.Analytics.TotalDurationMs)
	}
}

func TestJSONRoundTripPassedFalse(t *testing.T) {
	evt := validEvent()
	evt.Data.Passed = boolPtr(false)

	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Event
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Data.Passed == nil {
		t.Fatal("Data.Passed is nil; false should survive round-trip")
	}
	if *got.Data.Passed != false {
		t.Errorf("Data.Passed = %v, want false", *got.Data.Passed)
	}
}

func TestValidate_Valid(t *testing.T) {
	evt := validEvent()
	if err := evt.Validate(); err != nil {
		t.Errorf("valid event failed validation: %v", err)
	}
}

func TestValidate_MissingEventID(t *testing.T) {
	evt := validEvent()
	evt.EventID = ""
	if err := evt.Validate(); err == nil {
		t.Error("expected error for missing EventID")
	}
}

func TestValidate_MissingType(t *testing.T) {
	evt := validEvent()
	evt.Type = ""
	if err := evt.Validate(); err == nil {
		t.Error("expected error for missing Type")
	}
}

func TestValidate_UnknownType(t *testing.T) {
	evt := validEvent()
	evt.Type = "invalid.type"
	if err := evt.Validate(); err == nil {
		t.Error("expected error for unknown Type")
	}
}

func TestValidate_MissingTimestamp(t *testing.T) {
	evt := validEvent()
	evt.Timestamp = time.Time{}
	if err := evt.Validate(); err == nil {
		t.Error("expected error for zero Timestamp")
	}
}

func TestValidate_MissingInstanceID(t *testing.T) {
	evt := validEvent()
	evt.InstanceID = ""
	if err := evt.Validate(); err == nil {
		t.Error("expected error for missing InstanceID")
	}
}

func TestValidate_MissingRepo(t *testing.T) {
	evt := validEvent()
	evt.Repo = ""
	if err := evt.Validate(); err == nil {
		t.Error("expected error for missing Repo")
	}
}

func TestValidate_MissingContext(t *testing.T) {
	evt := validEvent()
	evt.Context = nil
	if err := evt.Validate(); err == nil {
		t.Error("expected error for nil Context")
	}
}

func TestValidate_AllKnownTypes(t *testing.T) {
	types := []Type{
		TypeSessionStarted,
		TypeSessionEnded,
		TypeIterationStarted,
		TypeIterationCompleted,
		TypePhaseChanged,
		TypeTaskClaimed,
		TypeTaskClosed,
	}

	for _, typ := range types {
		evt := validEvent()
		evt.Type = typ
		if err := evt.Validate(); err != nil {
			t.Errorf("type %q should be valid, got: %v", typ, err)
		}
	}
}

func TestValidate_NilData(t *testing.T) {
	evt := validEvent()
	evt.Data = nil
	if err := evt.Validate(); err != nil {
		t.Errorf("nil Data should be valid, got: %v", err)
	}
}
