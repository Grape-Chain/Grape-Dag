package discovery

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/enescakir/emoji"
	"github.com/ledongthuc/goterators"
	"github.com/libp2p/go-libp2p-core/discovery"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
)

type PeerDiscoveryEvent struct {
}

type PeerDiscovery struct {
}

var peer_discovered atomic.Bool = atomic.Bool{}

func (e *PeerDiscovery) Event(evt reflect.Type) {
	for {
		if peer_discovered.Load() {
			return
		}
		t := time.NewTimer(time.Second)
		<-t.C
	}
}

type ConnectionNotify interface {
	NotifyConnected(peer.ID)
	NotifyDisconnected(peer.ID)
}

type Notifees struct {
	mx       sync.Mutex
	notifees []ConnectionNotify
}

var notifees Notifees = Notifees{
	mx:       sync.Mutex{},
	notifees: []ConnectionNotify{},
}

func AddNotifee(n ConnectionNotify) {
	notifees.mx.Lock()
	defer notifees.mx.Unlock()
	notifees.notifees = append(notifees.notifees, n)
}

func RemoveNotifee(n ConnectionNotify) {
	notifees.mx.Lock()
	defer notifees.mx.Unlock()

	_, idx, e := goterators.Find(notifees.notifees, func(i ConnectionNotify) bool {
		return i == n
	})
	if e == nil {
		if len(notifees.notifees) == 1 {
			notifees.notifees = []ConnectionNotify{}
		} else {
			notifees.notifees = append(notifees.notifees[:idx], notifees.notifees[idx+1:]...)
		}
	}
}

func NotifyConnected(id peer.ID) {
	logger.Infof("Notifying connection listeners about a new connection with %s", id.String())
	notifees.mx.Lock()
	defer notifees.mx.Unlock()
	goterators.ForEach(notifees.notifees, func(i ConnectionNotify) {
		i.NotifyConnected(id)
	})
}

func NotifyDisconnected(id peer.ID) {
	logger.Infof("Notifying connection listeners about a new connection with %s", id.String())
	notifees.mx.Lock()
	defer notifees.mx.Unlock()
	goterators.ForEach(notifees.notifees, func(i ConnectionNotify) {
		i.NotifyDisconnected(id)
	})
}

func peer_discovery(ctx context.Context, wg *sync.WaitGroup, done chan bool, host host.Host, kdht *dht.IpfsDHT, rendezvous []string) {
	defer wg.Done()
	logger.Infof("%s  ~ DHT discovery is running %s ->", emoji.GlobeShowingAmericas, emoji.PersonRunning)
	routingDiscovery := drouting.NewRoutingDiscovery(kdht)
	var options []discovery.Option
	options = append(options, discovery.Limit(100))
	options = append(options, discovery.TTL(time.Minute*5))

	// Advertise is a utility function that persistently advertises a service through an Advertiser
	// It spawns a separate go routine - make sure we call it only once for
	// each topic
	for _, r := range rendezvous {
		dutil.Advertise(ctx, routingDiscovery, r, options...)
	}

	// we want to find new peers asap - run it once before the ticker will kick
	// it off every so often
	peerDiscovery(ctx, host, routingDiscovery, rendezvous, options...)
	ticker := time.NewTicker(time.Second * config.NET_DISCOVERY_CYCLE)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			logger.Infof("%s  ~ DHT discovery stopped", emoji.StopSign)
			ticker.Stop()
			return
		case <-ticker.C:
			kdht.RefreshRoutingTable()
			peerDiscovery(ctx, host, routingDiscovery, rendezvous, options...)
		}
	}
}
