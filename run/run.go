package run

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"reflect"
	"syscall"

	"github.com/Grape-Chain/Grape-Dag/app"
	"github.com/Grape-Chain/Grape-Dag/common"
	config "github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/dag"
	"github.com/Grape-Chain/Grape-Dag/diffusion"
	"github.com/Grape-Chain/Grape-Dag/discovery"
	network "github.com/Grape-Chain/Grape-Dag/network"
	grapepeer "github.com/Grape-Chain/Grape-Dag/peer"
	"github.com/Grape-Chain/Grape-Dag/services"
	"github.com/Grape-Chain/Grape-Dag/services/rest"
	"github.com/Grape-Chain/Grape-Dag/stats"
	"github.com/Grape-Chain/Grape-Dag/store"
	utils "github.com/Grape-Chain/Grape-Dag/utils"
	"github.com/Grape-Chain/Grape-Dag/vm"
	"github.com/enescakir/emoji"
	"github.com/google/uuid"
	golog "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
)

var (
	cfg    *config.HostConfig
	logger golog.EventLogger
)

func Start() {
	var (
		session_id uuid.UUID = uuid.Nil
	)

	// All config related initialization is handled here
	ConfUp()
	// by now, the looger has already been created and initialized, just get it by name
	logger = golog.Logger(config.LOGGER_ROOT_ID)
	golog.SetLogLevelRegex("go-libp2p-kad-dht*", "error")
	golog.SetLogLevelRegex("dht*", "fatal")
	golog.SetLogLevelRegex("nat/nat*", "fatal")
	if !validateActivation() {
		logger.Error("This node is not activated")
		os.Exit(0)
	}

	// The ledger store has to be open before the DAG initialises: a stored
	// commit-transaction chain is what the DAG recovers from, instead of
	// starting a fresh one.
	openLedgerStore()

	// DAG initialization depends on successful processing of the config/cmd line arguments
	// Let DAG init its config after we parse and process config/cmd line args.
	dag.Init()

	// Load peer keys or generate and save in ${HOME}/.grapeone
	prvKey := utils.ManagePK(config.GetConfig().Host.PeerID)
	// Start a new p2p host
	err := grapepeer.NewHost(prvKey, cfg, config.GetConfig())
	if err != nil {
		utils.ColorizeError(logger, "Host creation failed. %v", err)
	}

	network.Init()

	utils.ColorizeInfo(logger, "Using algorithm: %s", config.GetConfig().Dag.Algorithm)

	ctx, cancel := context.WithCancel(context.Background())

	app.GetApp().App_kdht = discovery.PeerDHT(grapepeer.GetHost(), cfg.Bootstrap_peers)
	app.GetApp().App_kdht_wg, app.GetApp().App_kdht_ch = discovery.RunRoutingTableRefresh(app.GetApp().App_kdht)
	grapepeer.SetIDHT(app.GetApp().App_kdht)
	logger.Debug("Peer DHT discovery is running...")

	var routingDiscovery *routing.RoutingDiscovery
	app.GetApp().App_dhtDiscoveryDone, routingDiscovery, app.GetApp().App_dhtWg =
		discovery.HandleDht(ctx, grapepeer.GetHost(), grapepeer.GetIDHT(), config.RENDEZVOUS)

	// It is possible to enable mDns discovery if this is a non-bootstrap node and mDns requested
	if config.GetConfig().Peer.Mdns > 0 {
		app.GetApp().App_mdnsDiscovery, err = discovery.InitMDNS(grapepeer.GetHost(), config.RENDEZVOUS[0])
		if err != nil {
			logger.Error("Failed to start mDns peer discovery. Ignoring...")
		} else {
			go discovery.HandlemDns(ctx, grapepeer.GetHost(), app.GetApp().App_mdnsDiscovery)
		}
	}

	if cfg.Stats || config.GetConfig().Peer.Stats > 0 {
		logger.Info("Tx Db Stats enabled")
		session_id = stats.NewStatsSession()
		if session_id == uuid.Nil {
			logger.Error("Failed to create a stats session. Will continue without writing tx to db.")
		} else {
			dag.InitDagToStats(session_id)
		}
	}

	// Wait for a connection event - we want to connect to other nodes before
	// we subscribe to the pubsub topics. This is important to be taking
	// place before we subscribe and join topics
	var discoveryEvtH common.IEvent = &discovery.PeerDiscovery{}
	// we are waiting for at least one discovery and a successful connection
	logger.Infof("%s  ~ Waiting for peer discovery to connect to another peer...", emoji.HourglassNotDone)
	discoveryEvtH.Event(reflect.TypeOf(discovery.PeerDiscoveryEvent{}))
	logger.Infof("%s  ~ Continue with creating a pubsub subsystem", emoji.CheckBoxWithCheck)
	// this is an event handler for the pubsub protocol update
	// we want to wait for an upgrade event before we return the subscr.
	evtProtoUpdate := &grapepeer.EvtHandler{}
	gossipSub := diffusion.CreatePubSubForPeer(routingDiscovery, evtProtoUpdate)
	app.GetApp().App_topic,
		app.GetApp().App_sub,
		app.GetApp().App_TopicRelayCsl =
		diffusion.Diffusion(ctx, grapepeer.GetHost(), gossipSub, config.RENDEZVOUS[0], session_id, cfg.Leader)

	if app.GetApp().App_topic == nil || app.GetApp().App_sub == nil {
		utils.ColorizeError(logger, "Failed to create Tx Topic/Subscription")
		utils.ColorizeError(logger, "Peer cannot continue. Will terminate now")
		stopApp(cancel)
		os.Exit(1)
	}

	// @DEVJOURNAL: it takes a bit of time to show up in DHT [no reason as to why]
	// since it's holding up unnecessarily too long, not check
	//	discovery.WaitUntilinDHT(app.GetApp().App_kdht, grapepeer.GetHost().ID())

	app.GetApp().App_dagsyncmgr = dag.DagSync(grapepeer.GetHost(), app.GetApp().App_kdht, routingDiscovery, gossipSub, config.RENDEZVOUS[1], config.GetConfig().Peer.Leader, false)

	// Must run after both host and dag have been initialized
	app.GetApp().App_dagsyncmgr.RunSynchronization(cfg.Leader, cfg.WaitConnect)

	if config.GetConfig().Peer.Grpc {
		app.GetApp().App_grpcPublishDone = services.RunRoboTraderService(config.GetConfig().Peer.Grpcport)
	}
	err = vm.StartStateServer()
	if err != nil {
		utils.ColorizeError(logger, "Failed to Start gRPC VM State Server", err.Error())
		os.Exit(2)
	}
	rest.StartRestAPISrv(ctx, routingDiscovery)

	// if profiling is enabled
	if cfg.Profile {

		// To generate a profiling report, in a separate terminal window run
		//  http://localhost:6060/debug/pprof/profile?seconds=60 (generate report for 60sec); then enter png to generate
		// a png report
		// Note: do not forget to enabe -profile option on the command line
		srv := &http.Server{
			Addr:    fmt.Sprintf("localhost:%d", 6060),
			Handler: nil,
		}
		go func() {
			srv.ListenAndServe()
		}()
	}
	utils.ColorizeInfo(logger, "%s  ~ %s %s is running with ID: %d [Ctrl-C to terminate]", emoji.CheckMarkButton, config.GRAPE, grapepeer.GetHost().ID().String(), os.Getpid())
	postProcessId(config.GetConfig().Host.PeerID)
	defer cleanProcessId(config.GetConfig().Host.PeerID)
	utils.WaitOnSignal([]os.Signal{syscall.SIGINT, syscall.SIGTERM})
	if config.GetConfig().Peer.Visualize > 0 {
		dag.GetDag().Visualize(cfg.PeerID)
	}
	if session_id != uuid.Nil {
		stats.StopSession(session_id)
	}
	stopApp(cancel)
}

func stopApp(cancel context.CancelFunc) {
	logger.Infof("%s  ~ Stopping %s services %s ...", emoji.HorizontalTrafficLight, emoji.Grapes, emoji.HammerAndWrench)
	cancel()
	// Persist DAG before shutting down DB
	logger.Infof("%s  ~ Stopping %s DAG components %s ...", emoji.HorizontalTrafficLight, emoji.Grapes, emoji.HammerAndWrench)
	dag.GetDag().Terminate()

	logger.Infof("%s  ~ Stopping %s API service ...", emoji.HorizontalTrafficLight, emoji.HammerAndWrench)
	rest.StopRestAPISrv()

	logger.Infof("%s  ~ Stopping %s app components %s ...", emoji.HorizontalTrafficLight, emoji.Grapes, emoji.HammerAndWrench)
	app.GetApp().Terminate()
	logger.Infof("%s  ~ Stopping %s peer components %s ...", emoji.HorizontalTrafficLight, emoji.Grapes, emoji.HammerAndWrench)
	grapepeer.Terminate()
	logger.Infof("%s  ALL SERVICES STOPPED  %s", emoji.VerticalTrafficLight, emoji.HammerAndWrench)

}

// openLedgerStore - open the durable ledger store and hand it to the DAG.
//
// Persistence is on by default. When it cannot be opened the node still starts,
// with the store turned off: it will rebuild its state from the network as it
// always did, which is a worse start-up but not a reason to refuse to run.
func openLedgerStore() {
	cfg := config.GetConfig()
	if cfg == nil || !cfg.Store.Enabled {
		utils.ColorizeWarn(logger, "[store] Persistence is disabled; this node will resync from the network on every restart")
		return
	}
	path := cfg.Store.Path
	if path == "" {
		path = "data/ledger"
	}
	if !filepath.IsAbs(path) {
		home, err := os.UserHomeDir()
		if err != nil {
			logger.Errorf("[store] Cannot resolve the home directory: %s", err.Error())
			return
		}
		path = filepath.Join(home, config.GRAPEONE_CFG_PATH, path)
	}
	s, err := store.Open(path)
	if err != nil {
		logger.Errorf("[store] Cannot open the ledger store at %s: %s", path, err.Error())
		logger.Warn("[store] Continuing without persistence")
		return
	}
	dag.SetStore(s)
}
