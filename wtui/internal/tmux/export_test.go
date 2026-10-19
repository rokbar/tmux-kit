package tmux

import (
	"slices"
	"testing"
)

// Subscribed is the value of a subscription a test added with Subscribe.
type Subscribed struct{ Name, Value string }

func (Subscribed) event() {}

// Subscribe has the Watches started from now until the test ends subscribe
// to format as name too, each value an event Subscribed.
func Subscribe(t *testing.T, name, format string) {
	old := subscriptions
	subscriptions = append(slices.Clone(old), subscription{name, format, func(value string) Event {
		return Subscribed{name, value}
	}})
	t.Cleanup(func() { subscriptions = old })
}
