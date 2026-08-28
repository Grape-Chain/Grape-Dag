package run

import (
	"flag"
	"fmt"
	"os"

	config "github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/stats"
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
		// Fatal, and said in full. There is nothing sensible to do without a
		// configuration file - every path below reads it - and the previous
		// behaviour was to warn and carry on, which produced a nil dereference a
		// few frames later. It also names the command that writes the file,
		// because "create this yaml" is not an instruction anyone can follow
		// without knowing what goes in it.
		where := "~/" + config.GRAPEONE_CFG_PATH + "/" + config.GRAPEPEER_FILE
		if len(cfg.Config) > 0 {
			where = cfg.Config
		}
		// Written straight to stderr rather than through the logger, because the
		// logger does not exist yet - ProcessLogInit runs after this, so the
		// obvious utils.ColorizeError(logger, ...) here segfaults on a nil
		// EventLogger and replaces one unhelpful stack trace with another.
		fmt.Fprintf(os.Stderr, "\nNo configuration file at %s\n\n", where)
		fmt.Fprintf(os.Stderr, "Write one with:\n    grapepeer join --wallet-file <path>\n\n")
		fmt.Fprintf(os.Stderr, "Or point at an existing one:\n    grapepeer -config <path>\n\n")
		os.Exit(2)
	}

	grapePeerConf.Peer.Id = cfg.PeerID

	if cfg.Profile {
		stats.StartDiagnosticsServer(cfg.Metricsaddr)
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

	// The libp2p listen port. Parsed into HostConfig and, until now, never copied
	// into the peer configuration - so -port was accepted, documented, and had no
	// effect whatever, while peer.go read only the yml value. gearup's own
	// validation rule ("when <home> is given, <port> must also be given") assumed
	// it worked.
	//
	// Above 1023 because that is the same guard the gRPC port uses, and because
	// binding a privileged port is not something a flag should make easy.
	if cfg.Port > 1023 {
		grapePeerConf.Peer.Port = cfg.Port
	}

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
