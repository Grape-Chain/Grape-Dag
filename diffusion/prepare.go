package diffusion

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/VG-Grape/luna/common"
	"github.com/VG-Grape/luna/config"
	lunapeer "github.com/VG-Grape/luna/peer"
	utils "github.com/VG-Grape/luna/utils"
	"github.com/enescakir/emoji"
	"github.com/libp2p/go-libp2p-core/event"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
)

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

func CreatePubSubForPeer(rd *routing.RoutingDiscovery, evtHandler common.IEvtBus) *pubsub.PubSub {

	cfg := config.GetConfig().Host

	var options []pubsub.Option
	pubsubParams := pubsub.DefaultGossipSubParams()
	// ConnectionTimeout controls the timeout for connection attempts.
	pubsubParams.ConnectionTimeout = time.Second * 5
	// HistoryLength controls the size of the message cache used for gossip.
	// The message cache will remember messages for HistoryLength heartbeats.
	pubsubParams.HistoryLength *= 10
	// HistoryGossip controls how many cached message ids we will advertise
	// in IHAVE gossip messages. When asked for our seen message IDs,
	// we will return only those from the most recent HistoryGossip heartbeats.
	// The slack between HistoryGossip and HistoryLength allows us to avoid
	// advertising messages that will be expired by the time they're requested.
	pubsubParams.HistoryGossip *= 10
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
	options = append(options, pubsub.WithGossipSubParams(pubsubParams))
	options = append(options, pubsub.WithDiscovery(rd))
	// WithPeerExchange is a gossipsub router option that enables Peer eXchange on PRUNE.
	// This should generally be enabled in bootstrappers and well connected/trusted nodes used for bootstrapping.
	options = append(options, pubsub.WithPeerExchange(true))
	options = append(options, pubsub.WithMessageSigning(true))
	// WithPeerOutboundQueueSize is an option to set the buffer size for outbound
	// messages to a peer We start dropping messages to a peer if the outbound queue if full
	options = append(options, pubsub.WithPeerOutboundQueueSize(config.GetConfig().Peer.Qsize*config.MB))
	options = append(options, pubsub.WithMaxMessageSize(config.GetConfig().Peer.Msize*config.MB))

	if cfg.Pubsub_tracing {
		var tracer *pubsub.JSONTracer = preparePubSubTracing(cfg)
		if tracer != nil {
			options = append(options, pubsub.WithEventTracer(tracer))
		}
	}
	rt := pubsub.DefaultGossipSubRouter(lunapeer.GetHost())

	gossipSub, err := pubsub.NewGossipSubWithRouter(context.Background(), lunapeer.GetHost(), rt, options...)
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
