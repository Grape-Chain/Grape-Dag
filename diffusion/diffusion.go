package diffusion

import (
	"context"
	"time"

	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/discovery"
	utils "github.com/Grape-Chain/Grape-Dag/utils"
	"github.com/enescakir/emoji"
	"github.com/google/uuid"
	golog "github.com/ipfs/go-log/v2"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
)

const (
	PUB_RATE = 5
)

var (
	logger      golog.EventLogger
	grapeConfig *config.Grapepeer
	txCount     uint64 = 0
)

func init() {
	logger = golog.Logger("diff")
}

func DiffusionStop() {
	// if statsCh != nil {
	// 	statsCh <- true
	// }
}

func Diffusion(ctx context.Context,
	host host.Host,
	gossipSub *pubsub.PubSub,
	subscription string,
	statsId uuid.UUID,
	leader bool) (*pubsub.Topic, *pubsub.Subscription, pubsub.RelayCancelFunc) {

	grapeConfig = config.GetConfig()

	topic, err := gossipSub.Join(subscription)
	if err != nil {
		logger.Fatalf("Diffusion: Tx sub join err: %s", err.Error())
		return nil, nil, nil
	}

	// See subBufferSize in subscribe.go: this was 1<<20, which is 8MB of
	// channel allocated at start-up to hold a million message pointers.
	subopt := []pubsub.SubOpt{pubsub.WithBufferSize(subBufferSize)}
	sub, err := topic.Subscribe(subopt...)
	if err != nil {
		utils.ColorizeError(logger, "Failed to subscribe to topic %s", subscription)
		topic.Close()
		return nil, nil, nil
	}
	logger.Infof("%s  ~ Waiting until DHT sees our peer join topic %s", emoji.HourglassNotDone, subscription)
	for {
		if discovery.GetMesh().In(subscription, host.ID()) {
			logger.Infof("%s  ~ %s registered for topic: %s", emoji.CheckBoxWithCheck, host.ID(), subscription)
			break
		} else {
			logger.Warnf("%s  ~ %s not registered for topic: %s", emoji.CrossMark, host.ID(), subscription)
		}
		tm := time.NewTimer(time.Second * 5)
		<-tm.C
		tm.Stop()
	}
	logger.Infof("%s  ~ our peer is in DHT for topic %s", emoji.CheckBoxWithCheck, subscription)

	tr, err := topic.Relay()
	if err != nil {
		logger.Errorf("Diffusion: Failed to enable relay for tx sub err: %s", err.Error())
	}

	eh, _ := topic.EventHandler()
	if err != nil {
		utils.ColorizeError(logger, "Failed to join pub/sub", err)
	}

	known_topics := gossipSub.GetTopics()
	for _, t := range known_topics {
		logger.Infof("Known topic: %s", t)
	}

	synch := grapeConfig.Peer.Qsync
	utils.ColorizeInfo(logger, "Diffusion:Topic event handler is running...")
	go topicEventHandler(eh, subscription)
	utils.ColorizeInfo(logger, "Diffusion:Publisher is running...")
	go publish(ctx, topic, statsId, leader)
	utils.ColorizeInfo(logger, "Diffusion:Subscriber is running...")
	go subscribe(sub, ctx, host.ID(), statsId, false, synch, leader)

	return topic, sub, tr
}

func topicEventHandler(eh *pubsub.TopicEventHandler, t string) {
	logger.Infof("[evt hdl] [*] Topic: %s running...", t)
	for eh != nil {
		evt, err := eh.NextPeerEvent(context.Background())
		if err != nil {
			utils.ColorizeError(logger, "[evt hdl] Topic: %s Cancel: %s", t, err.Error())
			break
		}

		switch evt.Type {
		case pubsub.PeerJoin:
			// wait for at least one more peer to join the topic before announcing [except to leader]
			logger.Infof("[evt hdl] [+] Topic: %s PEER_JOIN : %s", t, evt.Peer.String())
		case pubsub.PeerLeave:
			logger.Infof("[evt hdl] [-] Topic:%s PEER_LEAVE: %s", t, evt.Peer.String())
		}
	}
}
