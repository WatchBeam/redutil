package pubsub

import (
	"fmt"
	"math/rand"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEventTypes(t *testing.T) {
	assert.Equal(t, "SUBSCRIBE", PlainEvent.SubCommand())
	assert.Equal(t, "UNSUBSCRIBE", PlainEvent.UnsubCommand())
	assert.Equal(t, "PSUBSCRIBE", PatternEvent.SubCommand())
	assert.Equal(t, "PUNSUBSCRIBE", PatternEvent.UnsubCommand())
}

func TestEventBuildsString(t *testing.T) {
	e := NewEvent("foo")
	assert.Equal(t, PlainEvent, e.kind)
	assert.Equal(t, e.Name(), "foo")
}

func TestEventBuildsPattern(t *testing.T) {
	e := NewPattern("foo")
	assert.Equal(t, PatternEvent, e.kind)
	assert.Equal(t, e.Name(), "foo")
}

func TestEventBuildsMultipart(t *testing.T) {
	e := NewEvent("prefix:").String("foo:").Int(42)
	assert.Equal(t, "prefix:foo:42", e.Name())

	b := e.ToEvent("prefix:foo:42", "prefix:foo:42")
	assert.Equal(t, 3, b.Len())

	assert.Equal(t, "prefix:", b.Get(0).String())
	id, _ := b.Get(2).Int()
	assert.Equal(t, "foo:", b.Get(1).String())
	assert.Equal(t, 42, id)
	assert.Equal(t, "prefix:foo:42", b.Channel())
	assert.Equal(t, "prefix:foo:42", b.Pattern())
}

func TestEventReturnsZeroOnDNE(t *testing.T) {
	assert.True(t, NewEvent("foo").ToEvent("", "").Get(1).IsZero())
	assert.False(t, NewEvent("foo").ToEvent("", "").Get(0).IsZero())
	assert.True(t, NewEvent("foo").Int(1).As("bar").ToEvent("", "").Find("bleh").IsZero())
	assert.False(t, NewEvent("foo").Int(1).As("bar").ToEvent("", "").Find("bar").IsZero())
}

func TestEventMatchesPattern(t *testing.T) {
	tt := []struct {
		isMatch bool
		event   EventBuilder
		channel string
	}{
		{true, NewPattern("foo"), "foo"},
		{false, NewPattern("foo"), "bar"},
		{false, NewPattern("fooo"), "foo"},
		{false, NewPattern("foo"), "fooo"},

		{true, NewPattern("foo").Star(), "foo"},
		{true, NewPattern("foo").Star(), "fooasdf"},
		{true, NewPattern("foo").Star().String("bar"), "foo42bar"},
		{false, NewPattern("foo").Star().String("nar"), "foo42bar"},
		{true, NewPattern("foo").Star().String("bar").Star(), "foo42bar"},
		{true, NewPattern("foo").Star().String("bar").Star(), "foo42bar42"},
		{false, NewPattern("foo").Star().String("baz").Star(), "foo42bar42"},

		{false, NewPattern("foo").Alternatives("123"), "foo6"},
		{true, NewPattern("foo").Alternatives("123"), "foo2"},
	}

	for _, test := range tt {
		actual, _ := matchPatternAgainst(test.event, test.channel)
		matches := test.channel == actual.Name()
		if test.isMatch {
			assert.True(t, matches, fmt.Sprintf("%s ∉ %s", test.channel, test.event.Name()))
		} else {
			assert.False(t, matches, fmt.Sprintf("%s ∈ %s", test.channel, test.event.Name()))
		}
	}
}

func TestFuzz(t *testing.T) {
	if os.Getenv("FUZZ") == "" {
		return
	}

	fields := []struct {
		Transform func(e EventBuilder) EventBuilder
		Matching  string
	}{
		{func(e EventBuilder) EventBuilder { return e.Star() }, "adsf"},
		{func(e EventBuilder) EventBuilder { return e.String("foo") }, "foo"},
		{func(e EventBuilder) EventBuilder { return e.String("bar") }, "bar"},
		{func(e EventBuilder) EventBuilder { return e.Alternatives("123") }, "2"},
	}
	// Transition matrix for fields, by index. Given an [x, y], field[y] has
	// a matrix[x][y] chance of transitioning into field[x] next
	transitions := [][]float64{
		{0, 0.34, 0.34, 0.32},
		{0.3, 0.2, 0.3, 0.2},
		{0.3, 0.3, 0.2, 0.2},
		{0.25, 0.25, 0.25, 0.25},
	}

	fmt.Println("")

	for k := 0; true; k++ {
		event := NewPattern()
		matching := ""

		// 1. build the event
		for i := rand.Intn(len(fields)); len(event.fields) == 0 || rand.Float64() < 0.8; {
			event = fields[i].Transform(event)
			matching += fields[i].Matching

			x := rand.Float64()
			sum := float64(0)
			for idx, p := range transitions[i] {
				sum += p
				if x < sum {
					i = idx
					break
				}
			}
		}

		actual, _ := matchPatternAgainst(event, matching)
		if matching != actual.Name() {
			panic(fmt.Sprintf("%s ∉ %s", matching, event.Name()))
		}

		if k%100000 == 0 {
			fmt.Printf("\033[2K\r%d tests run -- %s ∈ %s", k, matching, event.Name())
		}
	}
}
