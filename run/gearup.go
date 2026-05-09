package run

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	config "github.com/VG-Grape/luna/config"
	utils "github.com/VG-Grape/luna/utils"
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

func GearUpConfig() *config.Lunapeer {
	help := flag.Bool("help", false, "Show usage")
	cfg = config.ParseCliArgs()
	if *help || len(cfg.PeerID) == 0 {
		utils.ColorizePrint(config.APP_NAME)
		utils.ColorizePrint("Usage: \n   Run lunaone -id <peerID>'\n")
		os.Exit(0)
	}
	if cfg.Home && (cfg.Port < 1023) {
		utils.ColorizePrint("When option <home> is given, option <port> must also be given")
		os.Exit(0)
	}

	// Load peer and dag configs
	lunaPeerConf := config.LoadLunaPeerFromConfig(cfg)
	if lunaPeerConf == nil {
		logger.Warnf("Failed to load %s. To better customize please create %s/%s",
			config.LUNAPEER_FILE, config.LUNAONE_CFG_PATH, config.LUNAPEER_FILE)
	}

	lunaPeerConf.Peer.Id = cfg.PeerID

	if cfg.Profile {
		fmt.Println("* Profiling is enabled")
		go func() {
			fmt.Println(http.ListenAndServe("localhost:6060", nil))
		}()
	}

	if cfg.VmServerPort > 1023 && cfg.VmServerPort <= 65535 {
		fmt.Printf("Use VmServerPort supplied via CMD (override yml file config) %d\n", cfg.VmServerPort)
		lunaPeerConf.Peer.VmServerPort = cfg.VmServerPort
	}
	if cfg.StateServerPort > 1023 && cfg.StateServerPort <= 65535 {
		fmt.Printf("Use StateServerPort supplied via CMD (override yml file config) %d\n", cfg.StateServerPort)
		lunaPeerConf.Peer.StateServerPort = cfg.StateServerPort
	}
	if cfg.NodeType > 0 {
		fmt.Printf("Node type=%d [cmd override]\n", cfg.NodeType)
		lunaPeerConf.Peer.NodeType = cfg.NodeType
	}
	if cfg.SnapshotSync {
		fmt.Printf("Snapshot sync is enabled [cmd override]")
		lunaPeerConf.Peer.SnapshotSync = true
	}
	fmt.Printf("Node started with type=%d, snapshotSync=%t", lunaPeerConf.Peer.NodeType, lunaPeerConf.Peer.SnapshotSync)

	// @TODO - temp fix for easy launching without constantly changing yml
	if cfg.Grpc {
		lunaPeerConf.Peer.Grpc = cfg.Grpc
	}
	if cfg.Grpcport > 1023 {
		lunaPeerConf.Peer.Grpcport = cfg.Grpcport
	}
	if cfg.Leader {
		lunaPeerConf.Peer.Leader = cfg.Leader
	}
	if cfg.Apiport > 1023 {
		lunaPeerConf.Peer.Apiport = cfg.Apiport
	}
	if cfg.VmServerPort > 1023 && cfg.VmServerPort < 65535 {
		lunaPeerConf.Peer.StateServerPort = cfg.VmServerPort
	}

	if cfg.Purge {
		lunaPeerConf.Peer.Purge = func() int {
			if cfg.Purge {
				return 1
			}
			return 0
		}()
	}

	return lunaPeerConf
}
