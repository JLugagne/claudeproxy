package app_test

import (
	"testing"
	"time"

	"github.com/JLugagne/claudeproxy/internal/proxy/app"
	"github.com/JLugagne/claudeproxy/internal/proxy/domain"
)

func TestUsageHubPublishWithoutSubscribers(t *testing.T) {
	hub := app.NewUsageHub()
	defer hub.Close()

	// Publishing with no listener must not block or panic.
	for i := 0; i < 10; i++ {
		hub.Publish(domain.Usage{Status: 200})
	}
}

func TestUsageHubDeliversToSubscriber(t *testing.T) {
	hub := app.NewUsageHub()
	defer hub.Close()
	events, cancel := hub.Subscribe()
	defer cancel()

	want := domain.Usage{
		Attribution: domain.Attribution{Project: "acme/gateway", Session: "s1"},
		Status:      200,
	}
	hub.Publish(want)

	select {
	case got := <-events:
		if got.Project != want.Project || got.Session != want.Session || got.Status != want.Status {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("no event received")
	}
}

func TestUsageHubSlowSubscriberDoesNotBlock(t *testing.T) {
	const buffer = 64

	hub := app.NewUsageHub()
	defer hub.Close()
	events, cancel := hub.Subscribe()
	defer cancel()

	// The subscriber never reads: Publish must still return promptly.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < buffer+10; i++ {
			hub.Publish(domain.Usage{Status: 200})
		}
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on a slow subscriber")
	}

	// Exactly the buffered events are retained; the rest were dropped.
	for i := 0; i < buffer; i++ {
		select {
		case _, ok := <-events:
			if !ok {
				t.Fatalf("channel closed after %d events", i)
			}
		case <-time.After(time.Second):
			t.Fatalf("buffered event %d was missing", i)
		}
	}
	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("more events were buffered than the subscriber buffer")
		}
	default:
	}
}

func TestUsageHubCancelClosesChannelOnce(t *testing.T) {
	hub := app.NewUsageHub()
	defer hub.Close()
	events, cancel := hub.Subscribe()

	cancel()
	if _, ok := <-events; ok {
		t.Fatal("cancel did not close the channel")
	}
	cancel() // Must be idempotent.

	hub.Publish(domain.Usage{Status: 200})
	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("event delivered after cancel")
		}
	default:
	}
}

func TestUsageHubCloseEndsFeedAndIsIdempotent(t *testing.T) {
	hub := app.NewUsageHub()
	first, cancelFirst := hub.Subscribe()
	defer cancelFirst()
	second, cancelSecond := hub.Subscribe()
	defer cancelSecond()

	hub.Close()
	hub.Close()

	for i, events := range []<-chan domain.Usage{first, second} {
		if _, ok := <-events; ok {
			t.Fatalf("subscriber %d channel was not closed", i)
		}
	}

	events, cancel := hub.Subscribe()
	defer cancel()
	if _, ok := <-events; ok {
		t.Fatal("Subscribe after Close returned an open channel")
	}
}
