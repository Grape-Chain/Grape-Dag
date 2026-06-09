package app

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/dag"
	"github.com/Grape-Chain/Grape-Dag/db/base"
	"github.com/Grape-Chain/Grape-Dag/diffusion"
	"github.com/Grape-Chain/Grape-Dag/network"
	"github.com/enescakir/emoji"
	golog "github.com/ipfs/go-log/v2"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
)

type App struct {
	App_globalStop       atomic.Bool
	App_topic            *pubsub.Topic
	App_sub              *pubsub.Subscription
	App_mdnsDiscovery    chan peer.AddrInfo
	App_dhtDiscoveryDone chan bool
	App_dhtWg            *sync.WaitGroup
	App_grpcPublishDone  chan<- bool
	App_dbmngr           base.DbManager
	App_dagsyncmgr       *dag.DagSyncMngr
	App_kdht             *dht.IpfsDHT
	App_kdht_ch          chan<- bool
	App_kdht_wg          *sync.WaitGroup
	App_TopicRelayCsl    pubsub.RelayCancelFunc
	App_Activation       []byte
}

var (
	app    *App              = nil
	logger golog.EventLogger = golog.Logger("p2p-app")
)

func GetApp() *App {
	if app == nil {
		app = &App{}
	}
	return app
}

func (app *App) Terminate() {
	logger.Info("Peer is going offline")
	app.App_globalStop.Store(true)
	network.TerminatePortMappings()
	if config.GetConfig().Peer.Grpc {
		if app.App_grpcPublishDone != nil {
			logger.Info("Stopping gRpc Publish Service")
			app.App_grpcPublishDone <- true
			logger.Info("Closing gRpc channel")
			close(app.App_grpcPublishDone)
		}
	}
	if app.App_mdnsDiscovery != nil {
		logger.Info("Stopping mDns discovery")
		close(app.App_mdnsDiscovery)
	}
	// Diffusion shutdown sequence
	diffusion.DiffusionStop()
	if app.App_sub != nil {
		logger.Info("Cancelling transaction subscription")
		app.App_sub.Cancel()
	}
	if app.App_TopicRelayCsl != nil {
		app.App_TopicRelayCsl()
	}
	if app.App_topic != nil {
		logger.Info("Closing transaction topic")
		app.App_topic.Close()
	}
	if app.App_dagsyncmgr != nil {
		logger.Info("Shuttind down sync manager")
		app.App_dagsyncmgr.Stop()
	}
	if app.App_dbmngr != nil {
		logger.Info("Disconnecting from DB")
		app.App_dbmngr.Disconnect()
	}
	if app.App_kdht != nil {
		// we have dht table and pingpong services
		logger.Info("Stopping DHT services")
		app.App_dhtDiscoveryDone <- true
		t := time.NewTimer(time.Second)
		<-t.C
		t.Stop()
		app.App_dhtDiscoveryDone <- true
		t = time.NewTimer(time.Second)
		<-t.C
		t.Stop()
		close(app.App_dhtDiscoveryDone)
		app.App_dhtWg.Wait()
		app.App_kdht_ch <- true
		app.App_kdht_wg.Wait()
		close(app.App_kdht_ch)
		app.App_kdht.Close()
		logger.Infof("%s  ~ %s DHT service stopped", emoji.StopSign, emoji.HammerAndWrench)
	}
}
