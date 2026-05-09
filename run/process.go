package run

import (
	"fmt"

	config "github.com/VG-Grape/luna/config"
	lunalog "github.com/VG-Grape/luna/logger"
	lunapeer "github.com/VG-Grape/luna/peer"
	golog "github.com/ipfs/go-log/v2"
)

type ProcessConfig interface {
	process(*config.Lunapeer) error
}

type ProcessPeerstorePurge struct{}

func (p *ProcessPeerstorePurge) process(c *config.Lunapeer) error {
	// Did we request a peerstore purge?
	if c.Peer.Purge > 0 {
		purgeStore(lunapeer.GetHost())
	}
	return nil
}

type ProcessLogInit struct{}

func (p *ProcessLogInit) process(c *config.Lunapeer) error {
	// Set log level for the node
	logLevel := detectlogLevel(golog.LogLevel(config.GetConfig().Peer.Logging), cfg.Debug)
	logFile := fmt.Sprintf(config.LOGGER_FN, cfg.PeerID)
	lunalog.InitLogging(logFile, cfg.Debug || config.GetConfig().Peer.Console > 0, logLevel)
	logger = golog.Logger(config.LOGGER_ROOT_ID)
	return nil
}

type ConfigProcessor struct {
	handlers []ProcessConfig
}

func (cp *ConfigProcessor) AddProcessor(i ProcessConfig) *ConfigProcessor {
	cp.handlers = append(cp.handlers, i)
	return cp
}

func (cp *ConfigProcessor) Process(lc *config.Lunapeer) {
	for _, h := range cp.handlers {
		if e := h.process(lc); e != nil {
			panic(e.Error())
		}
	}
}

func ConfUp() {
	cPross := ConfigProcessor{}
	cPross.
		AddProcessor(&ProcessActivation{}).
		AddProcessor(&ProcessPeerstorePurge{}).
		AddProcessor(&ProcessLogInit{})
	cPross.Process(GearUpConfig())
}
