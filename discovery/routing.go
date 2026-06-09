package discovery

import (
	"context"
	"sync"
	"time"

	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/enescakir/emoji"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/peer"
)

func WaitUntilinDHT(idht *dht.IpfsDHT, id peer.ID) {
	// before we create our pubsub, make sure we are already in the DHT table
	logger.Infof("%s  ~ Waiting until we appear in DHT...", emoji.HourglassNotDone)
	for {
		ctx, csl := context.WithTimeout(context.Background(), time.Second*10)
		_, err := idht.FindPeer(ctx, id)
		csl()
		if err == nil {
			logger.Infof("%s  ~ our peer is in DHT. Continue", emoji.MagnifyingGlassTiltedRight)
			break
		}
	}
	logger.Infof("%s  ~ %s is in DHT", emoji.CheckBoxWithCheck, id)
}

func RunRoutingTableRefresh(idht *dht.IpfsDHT) (*sync.WaitGroup, chan bool) {
	idht.ForceRefresh()
	wg := &sync.WaitGroup{}
	ch := make(chan bool)
	wg.Add(1)
	go func(idht *dht.IpfsDHT) {
		defer wg.Done()
		t := time.NewTicker(time.Second * config.ROUTING_TABLE_REFRESH)
		for {
			select {
			case <-ch:
				t.Stop()
				return
			case <-t.C:
				idht.RefreshRoutingTable()
				idht.RoutingTable().Print()
			}
		}
	}(idht)
	return wg, ch
}

func WaitUntilinDHTForTopic() {
	// before the node is ready, must assure that our peer is in dht table
	// logger.Infof("%s  ~ Waiting until the mesh sees our peer", emoji.HourglassNotDone)
	// var flags []bool = make([]bool, len(config.RENDEZVOUS))
	// for {
	// 	c := idht.RefreshRoutingTable()
	// 	<-c
	// 	for i, t := range config.RENDEZVOUS {
	// 		if discovery.GetMesh().In(t, grapepeer.GetHost().ID()) {
	// 			flags[i] = true
	// 			logger.Infof("%s  ~ %s registered for topic: %s", emoji.CheckBoxWithCheck, grapepeer.GetHost().ID(), t)
	// 		} else {
	// 			logger.Warnf("%s  ~ %s not registered for topic: %s", emoji.CrossMark, grapepeer.GetHost().ID(), t)
	// 		}
	// 	}
	// 	exit := goterators.Reduce(flags, true, func(prev bool, cur bool, _ int, _ []bool) bool {
	// 		return prev && cur
	// 	})
	// 	if exit {
	// 		break
	// 	}
	// 	tm := time.NewTimer(time.Second * 5)
	// 	<-tm.C
	// 	tm.Stop()
	// }
	// logger.Infof("%s  ~ our peer is in DHT", emoji.CheckBoxWithCheck)
}
