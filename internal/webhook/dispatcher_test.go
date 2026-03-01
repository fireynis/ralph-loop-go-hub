package webhook

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fireynis/ralph-hub/internal/config"
	"github.com/fireynis/ralph-hub/internal/events"
)

func boolPtr(b bool) *bool { return &b }

func makeEvent(t events.Type, passed *bool) events.Event {
	return events.Event{
		EventID:    "evt-1",
		Type:       t,
		Timestamp:  time.Now(),
		InstanceID: "inst-1",
		Repo:       "test/repo",
		Data:       &events.Data{Passed: passed},
		Context: &events.Context{
			SessionID: "sess-1",
		},
	}
}

// --- matches tests ---

func TestMatches_EmptyEventsMatchesAll(t *testing.T) {
	d := New(nil)
	hook := config.WebhookConfig{URL: "http://example.com"}
	// Empty events list should match any event type.
	for _, typ := range []events.Type{
		events.TypeSessionStarted,
		events.TypeIterationCompleted,
		events.TypeTaskClosed,
	} {
		evt := makeEvent(typ, nil)
		if !d.matches(hook, evt) {
			t.Errorf("expected empty events list to match %s", typ)
		}
	}
}

func TestMatches_EventTypeFiltering(t *testing.T) {
	d := New(nil)
	hook := config.WebhookConfig{
		URL:    "http://example.com",
		Events: []string{"session.started", "task.closed"},
	}

	tests := []struct {
		typ  events.Type
		want bool
	}{
		{events.TypeSessionStarted, true},
		{events.TypeTaskClosed, true},
		{events.TypeIterationCompleted, false},
		{events.TypePhaseChanged, false},
	}

	for _, tc := range tests {
		evt := makeEvent(tc.typ, nil)
		got := d.matches(hook, evt)
		if got != tc.want {
			t.Errorf("matches(%s) = %v, want %v", tc.typ, got, tc.want)
		}
	}
}

func TestMatches_PassedOnlyFilter(t *testing.T) {
	d := New(nil)
	hook := config.WebhookConfig{
		URL: "http://example.com",
	}
	hook.Filter.PassedOnly = true

	tests := []struct {
		name   string
		passed *bool
		want   bool
	}{
		{"passed true", boolPtr(true), true},
		{"passed false", boolPtr(false), false},
		{"passed nil", nil, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			evt := makeEvent(events.TypeIterationCompleted, tc.passed)
			got := d.matches(hook, evt)
			if got != tc.want {
				t.Errorf("matches() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMatches_PassedOnlyWithNilData(t *testing.T) {
	d := New(nil)
	hook := config.WebhookConfig{URL: "http://example.com"}
	hook.Filter.PassedOnly = true

	evt := events.Event{
		EventID:    "evt-1",
		Type:       events.TypeSessionStarted,
		Timestamp:  time.Now(),
		InstanceID: "inst-1",
		Repo:       "test/repo",
		Data:       nil,
		Context:    &events.Context{SessionID: "sess-1"},
	}

	if d.matches(hook, evt) {
		t.Error("expected passed_only to reject event with nil Data")
	}
}

func TestMatches_EventTypeAndPassedOnlyCombined(t *testing.T) {
	d := New(nil)
	hook := config.WebhookConfig{
		URL:    "http://example.com",
		Events: []string{"iteration.completed"},
	}
	hook.Filter.PassedOnly = true

	// Right type, passed=true -> match
	if !d.matches(hook, makeEvent(events.TypeIterationCompleted, boolPtr(true))) {
		t.Error("expected match for correct type + passed=true")
	}

	// Right type, passed=false -> no match
	if d.matches(hook, makeEvent(events.TypeIterationCompleted, boolPtr(false))) {
		t.Error("expected no match for correct type + passed=false")
	}

	// Wrong type, passed=true -> no match
	if d.matches(hook, makeEvent(events.TypeSessionStarted, boolPtr(true))) {
		t.Error("expected no match for wrong type even with passed=true")
	}
}

// --- delivery tests ---

func TestDeliver_PayloadArrivesCorrectly(t *testing.T) {
	var mu sync.Mutex
	var received events.Event
	called := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		called = true

		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}

		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("failed to decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	evt := makeEvent(events.TypeTaskClosed, boolPtr(true))
	evt.Data.TaskID = "task-42"

	hook := config.WebhookConfig{URL: srv.URL}
	d := New([]config.WebhookConfig{hook})
	d.deliver(hook, evt)

	mu.Lock()
	defer mu.Unlock()

	if !called {
		t.Fatal("webhook handler was never called")
	}
	if received.EventID != evt.EventID {
		t.Errorf("EventID = %q, want %q", received.EventID, evt.EventID)
	}
	if received.Type != evt.Type {
		t.Errorf("Type = %q, want %q", received.Type, evt.Type)
	}
	if received.Data == nil || received.Data.TaskID != "task-42" {
		t.Errorf("TaskID not delivered correctly")
	}
}

func TestDeliver_RetriesOnFailure(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	hook := config.WebhookConfig{URL: srv.URL}
	d := New([]config.WebhookConfig{hook})

	// Override client timeout but keep the real backoff for a real retry test.
	// To keep the test fast we override the deliver method's sleep by
	// calling deliver directly (it blocks).
	d.deliver(hook, makeEvent(events.TypeSessionStarted, nil))

	got := int(attempts.Load())
	if got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}

func TestDeliver_GivesUpAfterMaxAttempts(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	hook := config.WebhookConfig{URL: srv.URL}
	d := New([]config.WebhookConfig{hook})

	d.deliver(hook, makeEvent(events.TypeSessionStarted, nil))

	got := int(attempts.Load())
	if got != 3 {
		t.Errorf("expected 3 attempts (max), got %d", got)
	}
}

// --- Dispatch integration test ---

func TestDispatch_AsyncDelivery(t *testing.T) {
	var wg sync.WaitGroup
	var received atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		wg.Done()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	hooks := []config.WebhookConfig{
		{URL: srv.URL},
		{URL: srv.URL},
	}

	d := New(hooks)
	evt := makeEvent(events.TypeSessionStarted, nil)

	wg.Add(2)
	d.Dispatch(evt)
	wg.Wait()

	got := int(received.Load())
	if got != 2 {
		t.Errorf("expected 2 deliveries, got %d", got)
	}
}

func TestDispatch_SkipsNonMatchingHooks(t *testing.T) {
	var received atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	hooks := []config.WebhookConfig{
		{
			URL:    srv.URL,
			Events: []string{"task.closed"},
		},
	}

	d := New(hooks)
	evt := makeEvent(events.TypeSessionStarted, nil)

	d.Dispatch(evt)

	// Give a moment for any goroutine to fire (it should not).
	time.Sleep(50 * time.Millisecond)

	got := int(received.Load())
	if got != 0 {
		t.Errorf("expected 0 deliveries for non-matching event, got %d", got)
	}
}
