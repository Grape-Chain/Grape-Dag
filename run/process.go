package run

import (
	"fmt"

	config "github.com/Grape-Chain/Grape-Dag/config"
	grapelog "github.com/Grape-Chain/Grape-Dag/logger"
	grapepeer "github.com/Grape-Chain/Grape-Dag/peer"
	golog "github.com/ipfs/go-log/v2"
)

type ProcessConfig interface {
	process(*config.Grapepeer) error
}

type ProcessPeerstorePurge struct{}

func (p *ProcessPeerstorePurge) process(c *config.Grapepeer) error {
	// Did we request a peerstore purge?
	if c.Peer.Purge > 0 {
		purgeStore(grapepeer.GetHost())
	}
	return nil
}

type ProcessLogInit struct{}

func (p *ProcessLogInit) process(c *config.Grapepeer) error {
	// Set log level for the node
	logLevel := detectlogLevel(golog.LogLevel(config.GetConfig().Peer.Logging), cfg.Debug)
	logFile := fmt.Sprintf(config.LOGGER_FN, cfg.PeerID)
	grapelog.InitLogging(logFile, cfg.Debug || config.GetConfig().Peer.Console > 0, logLevel)
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

func (cp *ConfigProcessor) Process(lc *config.Grapepeer) {
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
