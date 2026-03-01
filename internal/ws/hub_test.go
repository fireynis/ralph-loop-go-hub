package ws

import "testing"

func TestHub_RegisterUnregister(t *testing.T) {
	h := NewHub()

	ch := make(chan []byte, 1)
	h.Register(ch)

	if h.ClientCount() != 1 {
		t.Fatalf("client count = %d, want 1", h.ClientCount())
	}

	h.Unregister(ch)

	if h.ClientCount() != 0 {
		t.Fatalf("client count = %d, want 0", h.ClientCount())
	}

	// Verify channel was closed.
	_, ok := <-ch
	if ok {
		t.Error("channel should be closed after unregister")
	}
}

func TestHub_UnregisterUnknownChannel(t *testing.T) {
	h := NewHub()

	ch := make(chan []byte, 1)
	// Unregistering a channel that was never registered should not panic.
	h.Unregister(ch)

	if h.ClientCount() != 0 {
		t.Fatalf("client count = %d, want 0", h.ClientCount())
	}
}

func TestHub_BroadcastMultipleClients(t *testing.T) {
	h := NewHub()

	ch1 := make(chan []byte, 1)
	ch2 := make(chan []byte, 1)
	ch3 := make(chan []byte, 1)

	h.Register(ch1)
	h.Register(ch2)
	h.Register(ch3)

	msg := []byte(`{"type":"test"}`)
	h.Broadcast(msg)

	for i, ch := range []chan []byte{ch1, ch2, ch3} {
		select {
		case got := <-ch:
			if string(got) != string(msg) {
				t.Errorf("client %d: got %s, want %s", i, got, msg)
			}
		default:
			t.Errorf("client %d: expected message but channel was empty", i)
		}
	}
}

func TestHub_BroadcastSkipsFullChannel(t *testing.T) {
	h := NewHub()

	full := make(chan []byte, 1)
	ready := make(chan []byte, 1)

	h.Register(full)
	h.Register(ready)

	// Fill the full channel so the next send would block.
	full <- []byte("old")

	msg := []byte(`{"type":"new"}`)
	h.Broadcast(msg)

	// The ready channel should have received the broadcast.
	select {
	case got := <-ready:
		if string(got) != string(msg) {
			t.Errorf("ready client: got %s, want %s", got, msg)
		}
	default:
		t.Error("ready client: expected message but channel was empty")
	}

	// The full channel should still contain the old message, not the new one.
	select {
	case got := <-full:
		if string(got) != "old" {
			t.Errorf("full client: got %s, want old", got)
		}
	default:
		t.Error("full client: expected old message but channel was empty")
	}

	// Nothing else in the full channel.
	select {
	case extra := <-full:
		t.Errorf("full client: unexpected extra message: %s", extra)
	default:
		// expected
	}
}

func TestHub_ClientCount(t *testing.T) {
	h := NewHub()

	if h.ClientCount() != 0 {
		t.Fatalf("initial count = %d, want 0", h.ClientCount())
	}

	channels := make([]chan []byte, 5)
	for i := range channels {
		channels[i] = make(chan []byte, 1)
		h.Register(channels[i])
	}

	if h.ClientCount() != 5 {
		t.Fatalf("count after registering 5 = %d, want 5", h.ClientCount())
	}

	h.Unregister(channels[0])
	h.Unregister(channels[1])

	if h.ClientCount() != 3 {
		t.Fatalf("count after unregistering 2 = %d, want 3", h.ClientCount())
	}
}

func TestHub_BroadcastNoClients(t *testing.T) {
	h := NewHub()

	// Broadcasting with no clients should not panic.
	h.Broadcast([]byte(`{"type":"test"}`))
}
