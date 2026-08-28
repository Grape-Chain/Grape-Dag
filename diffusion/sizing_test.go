package diffusion

import (
	"testing"

	"github.com/Grape-Chain/Grape-Dag/config"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

/*
These tests exist because every bug they cover was a unit error, and a unit
error reads as a deliberate choice: Qsize*config.MB looks like someone sizing a
buffer in megabytes, and it was, for an option that counts messages. Nothing in
the compiler or the library objects, and the node runs. So the units are pinned
here instead.
*/

// TestThePeerOutboundQueueIsAMessageCountNotAByteCount - the option's godoc is
// "an option to set the buffer size for outbound messages to a peer. We start
// dropping messages to a peer if the outbound queue if full", and the library's
// own default is 32. It was called with Peer.Qsize*config.MB: Qsize defaults to
// 16 and is documented in config as megabytes, so the node asked for a per-peer
// queue of 16,777,216 messages and gave up the drop boundary the option exists
// to provide.
func TestThePeerOutboundQueueIsAMessageCountNotAByteCount(t *testing.T) {
	got := peerOutboundQueueSize()
	if got != defaultPeerOutboundQueue {
		t.Fatalf("expected the default of %d messages, got %d", defaultPeerOutboundQueue, got)
	}
	// The specific mistake, stated so that reintroducing it fails here.
	if wrong := 16 * config.MB; got == wrong {
		t.Fatalf("the outbound queue is being sized in bytes again (%d)", wrong)
	}
	// A queue of a hundred thousand messages is not a queue. The ceiling is
	// arbitrary; being far below one is not.
	if got > 1<<16 {
		t.Fatalf("a per-peer outbound queue of %d messages has no useful drop boundary", got)
	}
	if got <= 0 {
		t.Fatal("the library rejects a non-positive outbound queue size")
	}
}

func TestThePeerOutboundQueueIsOverridableAndValidated(t *testing.T) {
	t.Setenv("GRAPE_PEER_OUTBOUND_QUEUE", "256")
	if got := peerOutboundQueueSize(); got != 256 {
		t.Fatalf("expected the override to be honoured, got %d", got)
	}
	for _, bad := range []string{"0", "-1", "sixteen", "16MB"} {
		t.Setenv("GRAPE_PEER_OUTBOUND_QUEUE", bad)
		if got := peerOutboundQueueSize(); got != defaultPeerOutboundQueue {
			t.Fatalf("GRAPE_PEER_OUTBOUND_QUEUE=%q should have been ignored, got %d", bad, got)
		}
	}
}

// TestTheGossipHistoryKeepsItsSlack - HistoryLength is a count of heartbeats,
// and one heartbeat is a second, so it is also how long whole messages stay in
// the gossip cache. Both it and HistoryGossip were multiplied by ten, which
// kept the library's HistoryGossip <= HistoryLength check satisfied while
// turning a five-second cache into a fifty-second one and destroying the slack
// the two values exist to maintain.
func TestTheGossipHistoryKeepsItsSlack(t *testing.T) {
	params := gossipParams()

	if params.HistoryLength != pubsub.GossipSubHistoryLength {
		t.Fatalf("HistoryLength is %d heartbeats, expected the library default of %d",
			params.HistoryLength, pubsub.GossipSubHistoryLength)
	}
	if params.HistoryGossip != pubsub.GossipSubHistoryGossip {
		t.Fatalf("HistoryGossip is %d heartbeats, expected the library default of %d",
			params.HistoryGossip, pubsub.GossipSubHistoryGossip)
	}
	if params.HistoryGossip >= params.HistoryLength {
		t.Fatalf("HistoryGossip (%d) must leave slack below HistoryLength (%d), or the node advertises ids for messages it is about to drop",
			params.HistoryGossip, params.HistoryLength)
	}
	// The library only enforces <=, so the multiplied values passed validation.
	// This is the check the library does not make.
	if err := checkGossipParams(params); err != nil {
		t.Fatalf("the parameters should be valid: %s", err.Error())
	}
}

// TestTheSubscriptionBufferIsBoundedToSomethingAffordable - WithBufferSize is
// "a Subscribe option to customize the size of the subscribe output buffer",
// default 32, and it allocates the channel immediately. It was 1<<20, which is
// 8MB of channel per subscription, and there are two subscriptions.
func TestTheSubscriptionBufferIsBoundedToSomethingAffordable(t *testing.T) {
	if subBufferSize == 1<<20 {
		t.Fatal("the subscription buffer is back to a million messages, which is 8MB of channel")
	}
	if subBufferSize < 256 {
		t.Fatalf("a subscription buffer of %d messages will drop on ordinary scheduling jitter", subBufferSize)
	}
	// 8 bytes per *Message on a 64-bit build. A megabyte of channel is the most
	// that is worth spending to absorb a hiccup.
	if bytes := subBufferSize * 8; bytes > 1<<20 {
		t.Fatalf("the subscription buffer allocates %d bytes up front", bytes)
	}
}
