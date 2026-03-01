package webhook

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/fireynis/ralph-hub/internal/config"
	"github.com/fireynis/ralph-hub/internal/events"
)

// Dispatcher fans out events to configured webhook endpoints.
type Dispatcher struct {
	hooks  []config.WebhookConfig
	client *http.Client
}

// New creates a Dispatcher for the given webhook configurations.
func New(hooks []config.WebhookConfig) *Dispatcher {
	return &Dispatcher{
		hooks: hooks,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Dispatch sends the event to every matching webhook asynchronously.
// It never blocks the caller.
func (d *Dispatcher) Dispatch(evt events.Event) {
	for _, hook := range d.hooks {
		if d.matches(hook, evt) {
			go d.deliver(hook, evt)
		}
	}
}

// matches returns true if the event should be delivered to the given hook.
func (d *Dispatcher) matches(hook config.WebhookConfig, evt events.Event) bool {
	// If hook.Events is non-empty, the event type must be in the list.
	if len(hook.Events) > 0 {
		found := false
		for _, e := range hook.Events {
			if e == string(evt.Type) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// If passed_only is set, only match events where Data.Passed is non-nil and true.
	if hook.Filter.PassedOnly {
		if evt.Data == nil || evt.Data.Passed == nil || !*evt.Data.Passed {
			return false
		}
	}

	return true
}

// deliver JSON-marshals the event and POSTs it to the hook URL.
// On failure it retries with exponential backoff (1s, 2s, 4s) up to 3 attempts total.
func (d *Dispatcher) deliver(hook config.WebhookConfig, evt events.Event) {
	body, err := json.Marshal(evt)
	if err != nil {
		log.Printf("webhook: failed to marshal event %s: %v", evt.EventID, err)
		return
	}

	backoff := 1 * time.Second
	const maxAttempts = 3

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resp, err := d.client.Post(hook.URL, "application/json", bytes.NewReader(body))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return
			}
			err = &httpError{statusCode: resp.StatusCode}
		}

		log.Printf("webhook: delivery attempt %d/%d to %s failed: %v", attempt, maxAttempts, hook.URL, err)

		if attempt < maxAttempts {
			time.Sleep(backoff)
			backoff *= 2
		}
	}
}

// httpError represents a non-2xx HTTP response.
type httpError struct {
	statusCode int
}

func (e *httpError) Error() string {
	return http.StatusText(e.statusCode)
}
