package run

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	config "github.com/Grape-Chain/Grape-Dag/config"
	utils "github.com/Grape-Chain/Grape-Dag/utils"
	golog "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p"
	p2p_config "github.com/libp2p/go-libp2p/config"
	"github.com/libp2p/go-libp2p/core/host"
)

func purgeStore(h host.Host) {
	if h == nil {
		prvKey := utils.ManagePK(config.GetConfig().Host.PeerID)

		options := []p2p_config.Option{
			libp2p.Identity(prvKey),
		}
		h, _ = libp2p.New(options...)
	}
	peers := h.Peerstore().Peers()
	for _, p := range peers {
		if p == h.ID() {
			continue
		}
		h.Peerstore().ClearAddrs(p)
		h.Peerstore().RemovePeer(p)
	}
	h.Close()
}

func detectlogLevel(level golog.LogLevel, debug bool) golog.LogLevel {
	var logLevel golog.LogLevel
	switch config.GetConfig().Peer.Logging {
	case int(golog.LevelDebug):
		logLevel = golog.LevelDebug
	case int(golog.LevelInfo):
		logLevel = golog.LevelInfo
	case int(golog.LevelWarn):
		logLevel = golog.LevelWarn
	case int(golog.LevelError):
		logLevel = golog.LevelError
	case int(golog.LevelFatal):
		logLevel = golog.LevelFatal
	case int(golog.LevelPanic):
		logLevel = golog.LevelPanic
	default:
		logLevel = golog.LevelPanic
	}
	// command line arg overrides the log level in yml file
	if cfg.Debug {
		logLevel = golog.LevelDebug
	}
	return logLevel
}

func GearUpConfig() *config.Grapepeer {
	help := flag.Bool("help", false, "Show usage")
	cfg = config.ParseCliArgs()
	if *help || len(cfg.PeerID) == 0 {
		utils.ColorizePrint(config.APP_NAME)
		utils.ColorizePrint("Usage: \n   Run grapeone -id <peerID>'\n")
		os.Exit(0)
	}
	if cfg.Home && (cfg.Port < 1023) {
		utils.ColorizePrint("When option <home> is given, option <port> must also be given")
		os.Exit(0)
	}

	// Load peer and dag configs
	grapePeerConf := config.LoadGrapePeerFromConfig(cfg)
	if grapePeerConf == nil {
		logger.Warnf("Failed to load %s. To better customize please create %s/%s",
			config.GRAPEPEER_FILE, config.GRAPEONE_CFG_PATH, config.GRAPEPEER_FILE)
	}

	grapePeerConf.Peer.Id = cfg.PeerID

	if cfg.Profile {
		fmt.Println("* Profiling is enabled")
		go func() {
			fmt.Println(http.ListenAndServe("localhost:6060", nil))
		}()
	}

	if cfg.VmServerPort > 1023 && cfg.VmServerPort <= 65535 {
		fmt.Printf("Use VmServerPort supplied via CMD (override yml file config) %d\n", cfg.VmServerPort)
		grapePeerConf.Peer.VmServerPort = cfg.VmServerPort
	}
	if cfg.StateServerPort > 1023 && cfg.StateServerPort <= 65535 {
		fmt.Printf("Use StateServerPort supplied via CMD (override yml file config) %d\n", cfg.StateServerPort)
		grapePeerConf.Peer.StateServerPort = cfg.StateServerPort
	}
	if cfg.NodeType > 0 {
		fmt.Printf("Node type=%d [cmd override]\n", cfg.NodeType)
		grapePeerConf.Peer.NodeType = cfg.NodeType
	}
	if cfg.SnapshotSync {
		fmt.Printf("Snapshot sync is enabled [cmd override]")
		grapePeerConf.Peer.SnapshotSync = true
	}
	fmt.Printf("Node started with type=%d, snapshotSync=%t", grapePeerConf.Peer.NodeType, grapePeerConf.Peer.SnapshotSync)

	// @TODO - temp fix for easy launching without constantly changing yml
	if cfg.Grpc {
		grapePeerConf.Peer.Grpc = cfg.Grpc
	}
	if cfg.Grpcport > 1023 {
		grapePeerConf.Peer.Grpcport = cfg.Grpcport
	}
	if cfg.Leader {
		grapePeerConf.Peer.Leader = cfg.Leader
	}
	if cfg.Apiport > 1023 {
		grapePeerConf.Peer.Apiport = cfg.Apiport
	}
	if cfg.VmServerPort > 1023 && cfg.VmServerPort < 65535 {
		grapePeerConf.Peer.StateServerPort = cfg.VmServerPort
	}

	if cfg.Purge {
		grapePeerConf.Peer.Purge = func() int {
			if cfg.Purge {
				return 1
			}
			return 0
		}()
	}

	return grapePeerConf
}
