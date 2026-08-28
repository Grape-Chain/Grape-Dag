package diffusion

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"time"

	"github.com/Grape-Chain/Grape-Dag/common"
	"github.com/Grape-Chain/Grape-Dag/config"
	grapepeer "github.com/Grape-Chain/Grape-Dag/peer"
	utils "github.com/Grape-Chain/Grape-Dag/utils"
	"github.com/enescakir/emoji"
	"github.com/libp2p/go-libp2p-core/event"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
)

/*
Per-peer outbound queue.

pubsub.WithPeerOutboundQueueSize is documented as "the buffer size for outbound
messages to a peer. We start dropping messages to a peer if the outbound queue
is full", and its argument is a message count; the library's own default is 32.

It was called as Qsize*config.MB. Peer.Qsize defaults to 16 and its config
comment reads "pubsub outbound queue size in mb", so the node asked for a
per-peer queue of 16 * 1048576 = 16,777,216 messages. The queue is a slice that
grows on demand, so the harm is not an allocation at start-up: it is that the
drop boundary the option exists to provide was moved beyond anything the
process could survive. A peer that stops reading accumulates RPCs, with their
payloads, until the node dies instead of being dropped.

Peer.Qsize is deliberately not wired to this any more. Its documented unit is
megabytes, which is not this option's unit, and correcting the field's meaning
is a change in config/ - see the report. The count below is what governs, and
GRAPE_PEER_OUTBOUND_QUEUE overrides it per process.
*/
const (
	// defaultPeerOutboundQueue - 1024 messages, against a library default of
	// 32. At the measured publish rate 32 is about a hundredth of a second of
	// buffer per peer, which drops on any ordinary scheduling hiccup; 1024 is
	// roughly four tenths of a second and still bounded at a few megabytes
	// across a full mesh. It is a queue with a ceiling either way, which is the
	// property that matters.
	defaultPeerOutboundQueue = 1024
)

func peerOutboundQueueSize() int {
	if env := os.Getenv("GRAPE_PEER_OUTBOUND_QUEUE"); env != "" {
		if n, err := strconv.Atoi(env); err == nil && n > 0 {
			return n
		}
		logger.Warnf("Ignoring GRAPE_PEER_OUTBOUND_QUEUE=%q: expected a positive message count", env)
	}
	return defaultPeerOutboundQueue
}

func preparePubSubTracing(cfg config.HostConfig) *pubsub.JSONTracer {
	var tracer *pubsub.JSONTracer = nil
	if cfg.Pubsub_tracing {
		tmpdir := os.TempDir()
		fn := fmt.Sprintf("/%s/pubsub-trace-%s.json", tmpdir, cfg.PeerID)
		os_file, err := os.Create(fn)
		if err != nil {
			logger.Errorf("Error creating file %s - %s", fn, err.Error())
		}
		os_file.WriteString(fmt.Sprintf("[%s] - %s\n", cfg.PeerID, time.Now().Local().String()))
		tracer, err = pubsub.NewJSONTracer(fn)
		if err != nil {
			logger.Errorf("Error while creating a json tracer. err: %s", err.Error())
			tracer = nil
		}
	}
	return tracer
}

// gossipParams - the gossipsub tuning this node runs with.
//
// Split out of CreatePubSubForPeer so the numbers can be checked without a host,
// a DHT and a network. Every value here was wrong once in a way that still
// started the node, which is the argument for testing it at all.
func gossipParams() pubsub.GossipSubParams {
	pubsubParams := pubsub.DefaultGossipSubParams()
	// ConnectionTimeout controls the timeout for connection attempts.
	pubsubParams.ConnectionTimeout = time.Second * 5
	// HistoryLength controls the size of the message cache used for gossip.
	// The message cache will remember messages for HistoryLength heartbeats.
	//
	// Left at the library default of 5. It was multiplied by ten, which is a
	// change of unit rather than of degree: the cache holds whole messages, one
	// heartbeat is a second, so fifty heartbeats is fifty seconds of every
	// message this node has seen held in memory. At the measured arrival rate
	// that is well over a hundred thousand messages resident for no benefit -
	// nobody asks for a message id that old, because HistoryGossip is what
	// governs what we advertise.
	//
	// The cost of leaving it at 5 is that a peer which asks for a message via
	// IWANT more than five seconds after we cached it gets nothing and has to
	// find it elsewhere. That is what the default is chosen for.
	pubsubParams.HistoryLength = pubsub.GossipSubHistoryLength
	// HistoryGossip controls how many cached message ids we will advertise
	// in IHAVE gossip messages. When asked for our seen message IDs,
	// we will return only those from the most recent HistoryGossip heartbeats.
	// The slack between HistoryGossip and HistoryLength allows us to avoid
	// advertising messages that will be expired by the time they're requested.
	//
	// Also back to the default of 3. Multiplied by ten it became 30, which is
	// six times the unmultiplied HistoryLength: the library only checks that
	// HistoryGossip <= HistoryLength, so multiplying both kept the check
	// satisfied while destroying the slack the comment above describes. The
	// node would then advertise ids for messages it was about to drop.
	pubsubParams.HistoryGossip = pubsub.GossipSubHistoryGossip
	// D sets the optimal degree for a GossipSub topic mesh.
	// For example, if D == 6, each peer will want to have about
	// six peers in their mesh for each topic they're subscribed to.
	// D should be set somewhere between Dlo and Dhi.
	pubsubParams.D = 8
	// Dhi sets the upper bound on the number of peers we keep in a GossipSub
	// topic mesh. If we have more than Dhi peers, we will select some to
	// prune from the mesh at the next heartbeat.
	pubsubParams.Dhi = 12
	// Dlo sets the lower bound on the number of peers we keep in a GossipSub
	// topic mesh. If we have fewer than Dlo peers, we will attempt to graft
	// some more into the mesh at the next heartbeat.
	pubsubParams.Dlo = 4
	// Dout sets the quota for the number of outbound connections to maintain in a topic mesh.
	// When the mesh is pruned due to over subscription, we make sure that we have outbound connections
	// to at least Dout of the survivor peers. This prevents sybil attackers from overwhelming
	// our mesh with incoming connections.
	pubsubParams.Dout = 2
	return pubsubParams
}

// checkGossipParams - the invariant the library does not check.
//
// go-libp2p-pubsub validates HistoryGossip <= HistoryLength and nothing more, so
// multiplying both by the same factor passed validation while removing the slack
// between them. Anything below HistoryLength is slack; equal is not.
func checkGossipParams(p pubsub.GossipSubParams) error {
	if p.HistoryLength <= 0 || p.HistoryGossip <= 0 {
		return fmt.Errorf("gossip history must be positive: HistoryLength=%d HistoryGossip=%d",
			p.HistoryLength, p.HistoryGossip)
	}
	if p.HistoryGossip >= p.HistoryLength {
		return fmt.Errorf("HistoryGossip=%d must leave slack below HistoryLength=%d, or the node advertises message ids it is about to drop",
			p.HistoryGossip, p.HistoryLength)
	}
	if !(p.Dlo <= p.D && p.D <= p.Dhi) {
		return fmt.Errorf("mesh degrees must satisfy Dlo <= D <= Dhi, got %d, %d, %d", p.Dlo, p.D, p.Dhi)
	}
	return nil
}

func CreatePubSubForPeer(rd *routing.RoutingDiscovery, evtHandler common.IEvtBus) *pubsub.PubSub {

	cfg := config.GetConfig().Host

	var options []pubsub.Option
	pubsubParams := gossipParams()
	if err := checkGossipParams(pubsubParams); err != nil {
		// Fatal rather than logged: a node whose gossip cache holds fifty
		// seconds of every message it has seen does not fail, it just uses
		// several hundred megabytes and nobody notices for a month.
		logger.Fatalf("Gossip parameters are inconsistent: %s", err.Error())
	}
	options = append(options, pubsub.WithGossipSubParams(pubsubParams))
	options = append(options, pubsub.WithDiscovery(rd))
	// WithPeerExchange is a gossipsub router option that enables Peer eXchange on PRUNE.
	// This should generally be enabled in bootstrappers and well connected/trusted nodes used for bootstrapping.
	options = append(options, pubsub.WithPeerExchange(true))
	options = append(options, pubsub.WithMessageSigning(true))
	// WithPeerOutboundQueueSize is an option to set the buffer size for outbound
	// messages to a peer We start dropping messages to a peer if the outbound queue if full
	options = append(options, pubsub.WithPeerOutboundQueueSize(peerOutboundQueueSize()))
	// Msize is in megabytes and WithMaxMessageSize takes bytes, so this one is
	// right; it is next to the queue-size option only because the two look
	// alike and one of them was not.
	options = append(options, pubsub.WithMaxMessageSize(config.GetConfig().Peer.Msize*config.MB))

	if cfg.Pubsub_tracing {
		var tracer *pubsub.JSONTracer = preparePubSubTracing(cfg)
		if tracer != nil {
			options = append(options, pubsub.WithEventTracer(tracer))
		}
	}
	rt := pubsub.DefaultGossipSubRouter(grapepeer.GetHost())

	gossipSub, err := pubsub.NewGossipSubWithRouter(context.Background(), grapepeer.GetHost(), rt, options...)
	if err != nil {
		logger.Fatalf("Create pubsub system: %s", err.Error())
	}

	// wait for the protocol upgrade event
	evtHandler.WaitForEvent(reflect.TypeOf(event.EvtLocalProtocolsUpdated{}))

	buf := bytes.Buffer{}
	for _, t := range config.RENDEZVOUS {
		buf.WriteString(fmt.Sprintf("Peers for topic %s\n", t))
		peers := gossipSub.ListPeers(t)
		for _, p := range peers {
			buf.WriteString(fmt.Sprintf("\tpeer %s\n", p.String()))
		}
	}
	logger.Infof("%s  ~ Successfully created a gossip pub/sub diffusion subsystem", emoji.CheckBoxWithCheck)
	utils.ColorizeInfo(logger, "\n%s\n", buf.String())
	return gossipSub
}
