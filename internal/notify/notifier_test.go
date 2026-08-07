package notify

import (
	"testing"
	"time"
)

func waitWake(t *testing.T, s *Subscription) {
	t.Helper()
	select {
	case <-s.Wake():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for wake-up")
	}
}

func waitDone(t *testing.T, s *Subscription) {
	t.Helper()
	select {
	case <-s.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for subscription to be cancelled")
	}
}

func assertNoWake(t *testing.T, s *Subscription) {
	t.Helper()
	select {
	case <-s.Wake():
		t.Fatal("unexpected wake-up")
	default:
	}
}

// TestSubscribeReceivesWakeUp proves a subscribed agent receives a Notify.
func TestSubscribeReceivesWakeUp(t *testing.T) {
	n := New()
	defer n.Close()

	s := n.Subscribe("agent-1")
	defer s.Unsubscribe()

	n.Notify("agent-1")
	waitWake(t, s)
}

// TestNotifyWithoutSubscriberIsNoOp proves Notify never blocks and is harmless
// when no agent is connected.
func TestNotifyWithoutSubscriberIsNoOp(t *testing.T) {
	n := New()
	defer n.Close()

	done := make(chan struct{})
	go func() {
		n.Notify("ghost")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Notify blocked with no subscriber")
	}
}

// TestNotifyDoesNotWakeAnotherAgent proves notifications are scoped per agent.
func TestNotifyDoesNotWakeAnotherAgent(t *testing.T) {
	n := New()
	defer n.Close()

	s1 := n.Subscribe("agent-1")
	defer s1.Unsubscribe()
	s2 := n.Subscribe("agent-2")
	defer s2.Unsubscribe()

	n.Notify("agent-1")
	waitWake(t, s1)
	assertNoWake(t, s2)
}

// TestDuplicateNotifyCoalesces proves repeated Notify calls collapse into a
// single pending wake-up: the subscription channel has capacity one.
func TestDuplicateNotifyCoalesces(t *testing.T) {
	n := New()
	defer n.Close()

	s := n.Subscribe("agent-1")
	defer s.Unsubscribe()

	n.Notify("agent-1")
	n.Notify("agent-1")
	n.Notify("agent-1")

	waitWake(t, s)
	assertNoWake(t, s)
}

// TestSlowSubscriberCannotBlockNotify proves Notify never blocks even when the
// consumer never drains the channel.
func TestSlowSubscriberCannotBlockNotify(t *testing.T) {
	n := New()
	defer n.Close()

	s := n.Subscribe("agent-1")
	defer s.Unsubscribe()

	// Saturate the pending wake-up channel without consuming it.
	for i := 0; i < 100; i++ {
		n.Notify("agent-1")
	}

	done := make(chan struct{})
	go func() {
		n.Notify("agent-1")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Notify blocked behind a slow subscriber")
	}
	_ = s
}

// TestReplacingSubscriptionCancelsPriorStream proves a second Subscribe for the
// same agent cancels the first stream so only one is active at a time.
func TestReplacingSubscriptionCancelsPriorStream(t *testing.T) {
	n := New()
	defer n.Close()

	first := n.Subscribe("agent-1")
	second := n.Subscribe("agent-1")

	waitDone(t, first)
	assertNoWake(t, second)

	// The old stream must not receive further wake-ups.
	n.Notify("agent-1")
	assertNoWake(t, first)
	waitWake(t, second)

	second.Unsubscribe()
}

// TestUnsubscribeRemovesActiveStream proves Unsubscribe frees the slot so a new
// Subscribe is not considered a replacement and the old channel is cancelled.
func TestUnsubscribeRemovesActiveStream(t *testing.T) {
	n := New()
	defer n.Close()

	first := n.Subscribe("agent-1")
	first.Unsubscribe()
	waitDone(t, first)

	second := n.Subscribe("agent-1")
	defer second.Unsubscribe()
	n.Notify("agent-1")
	waitWake(t, second)

	// Replacing the unsubscribed stream must not cancel the new one.
	third := n.Subscribe("agent-1")
	waitDone(t, second)
	n.Notify("agent-1")
	waitWake(t, third)
	third.Unsubscribe()
}

// TestCloseCancelsAllSubscriptions proves Notifier.Close ends every stream and
// makes later Notify calls no-ops.
func TestCloseCancelsAllSubscriptions(t *testing.T) {
	n := New()

	a := n.Subscribe("agent-1")
	b := n.Subscribe("agent-2")

	n.Close()

	waitDone(t, a)
	waitDone(t, b)

	// Notify after Close is a no-op (does not panic).
	n.Notify("agent-1")

	// A Subscribe after Close returns an already-cancelled subscription.
	c := n.Subscribe("agent-1")
	waitDone(t, c)
}

// TestUnsubscribeAfterReplaceIsNoOp proves calling Unsubscribe on a replaced
// subscription does not cancel the replacement.
func TestUnsubscribeAfterReplaceIsNoOp(t *testing.T) {
	n := New()
	defer n.Close()

	first := n.Subscribe("agent-1")
	second := n.Subscribe("agent-1")

	// Late unsubscribe from the replaced stream must not touch the new one.
	first.Unsubscribe()

	n.Notify("agent-1")
	waitWake(t, second)
	second.Unsubscribe()
}
