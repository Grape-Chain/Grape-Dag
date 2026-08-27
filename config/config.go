package config

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/enescakir/emoji"
	golog "github.com/ipfs/go-log/v2"
	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/viper"
)

var logger golog.EventLogger

func init() {
	logger = golog.Logger("p2p-config")
}

// validateAddress - check that addr is a full-length hex account address.
// Kept local to avoid a config -> crypto dependency.
func validateAddress(addr string) error {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(addr, "0x"), "0X")
	b, err := hex.DecodeString(trimmed)
	if err != nil {
		return fmt.Errorf("not a hex address: %s", err.Error())
	}
	if len(b) != ADDRESS_BYTE_LEN {
		return fmt.Errorf("address must be %d bytes, got %d", ADDRESS_BYTE_LEN, len(b))
	}
	return nil
}

var grapepeer *Grapepeer

func LoadGrapePeerFromConfig(hc *HostConfig) *Grapepeer {
	if grapepeer == nil {
		grapepeer = LoadGrapepeer(hc)
	}
	grapepeer.Host = *hc
	return grapepeer
}

func GetConfig() *Grapepeer {
	return grapepeer
}

var (
	RENDEZVOUS []string = []string{} //[]string{RENDEZVOUS_ID, DAGSYNC_ID}
)

func ParseCliArgs() *HostConfig {
	conf := &HostConfig{}

	flag.StringVar(&conf.Rendezvous, "rendezvous", "grapeone", "Unique string to identify group of nodes.")
	flag.StringVar(&conf.ProtocolID, "protocol", "/grapeone/0.0.1", "Sets a protocol id for stream headers")
	flag.Int64Var(&conf.Seed, "seed", 0, "Random num gen seed value for consistent rand sequence")
	flag.BoolVar(&conf.Bootstrap, "bootstrap", false, "this is a bootstrap node")
	flag.BoolVar(&conf.Debug, "d", false, "Enable debug level logging")
	flag.StringVar(&conf.PeerID, "id", "", "This peer id. Must be unique")
	flag.BoolVar(&conf.Grpc, "grpc", false, "Enable grpc")
	flag.BoolVar(&conf.Leader, "leader", false, "This node is a leader")
	flag.IntVar(&conf.NodeType, "node_type", 0, "Node type to use")
	flag.BoolVar(&conf.WaitConnect, "wait", false, "Wait for a peer connection before proceeding")
	flag.BoolVar(&conf.Profile, "profile", false, "Enable http profiler server")
	flag.IntVar(&conf.Port, "port", 0, "Port to use")
	flag.IntVar(&conf.Grpcport, "grpc_port", 0, "GRPC port to use")
	flag.IntVar(&conf.Apiport, "api_port", 0, "REST API port to use")
	flag.BoolVar(&conf.Genesis, "genesis", false, "Launch genesis peer")
	flag.BoolVar(&conf.Stats, "stats", false, "Enable tx stats collection (one node only)")
	flag.BoolVar(&conf.Single, "single", false, "Enable single node testing")
	flag.IntVar(&conf.VmServerPort, "vmserver_port", -1, "Port of the local SC VM server to send smart contract transactions for execution")
	flag.IntVar(&conf.StateServerPort, "stateserver_port", -1, "Port of the state server maintained by node for vm state calls")
	flag.BoolVar(&conf.Home, "home", false, "when on a home network, try port mappings using UPnP for higher connectivity success rate")
	flag.BoolVar(&conf.Pubsub_tracing, "trace", false, "enable/disable pubsub tracing")
	flag.BoolVar(&conf.Purge, "purge", false, "Purge the peer store")
	flag.IntVar(&conf.Verbose, "verbose", 0, "Add verbosity 1-5")
	flag.StringVar(&conf.Config, "f", "", "Configuration file to use instead of the default grapepeer.yml")
	flag.StringVar(&conf.Activation, "activation", "", "Activation file to activate the leader")
	flag.StringVar(&conf.Secret, "secret", "", "Secret activation key")
	flag.BoolVar(&conf.SnapshotSync, "snapshot_sync", false, "Enable snapshot sync from leader's balances")
	bs_peers := flag.String("bootstrap_nodes", "", "a comma separated list of bootstrap peers to get started")

	flag.Parse()

	if conf.Verbose > 2 {
		fmt.Printf("\t%s  %s AREA 51 %s - PROCEED WITH CAUTION!\n", emoji.Warning, emoji.Alien, emoji.Alien)
	}

	if bs_peers != nil && len(*bs_peers) > 0 {
		for _, p := range strings.Split(*bs_peers, ",") {
			next_peer := strings.Trim(p, " \t\n")
			maddr, err := multiaddr.NewMultiaddr(next_peer)
			if err != nil {
				logger.Errorf("Parsing multiaddr of '%s'. Skipping...", next_peer)
				continue
			}
			conf.Bootstrap_peers = append(conf.Bootstrap_peers, maddr)
		}
	}
	conf.Config = strings.Trim(conf.Config, "\"")
	fmt.Printf("Config file supplied is %s\n", conf.Config)

	RENDEZVOUS = []string{
		fmt.Sprintf("%s/%s", PRE_RENDEZVOUS_ID, conf.Rendezvous),
		fmt.Sprintf("%s/%s", PRE_DAGSYNC_ID, conf.Rendezvous),
	}
	return conf
}

func LoadBootstrap() []multiaddr.Multiaddr {
	var bootstrap_list []multiaddr.Multiaddr
	hd, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	fd, err := os.Open(filepath.Join(hd, GRAPEONE_CFG_PATH, BOOTSTRAP_FILE))
	if err != nil {
		if os.IsNotExist(err) {
			// Configuration file does not exist
			return nil
		}
	}
	defer fd.Close()
	rr := bufio.NewReader(fd)
	jsondata, err := ioutil.ReadAll(rr)
	if err != nil {
		return nil
	}
	var payload map[string]interface{} = make(map[string]interface{})
	err = json.Unmarshal(jsondata, &payload)
	if err != nil {
		return nil
	}
	for _, peer_addr := range payload {
		v, ok := peer_addr.(string)
		if ok {
			addr, err := multiaddr.NewMultiaddr(v)
			if err == nil {
				bootstrap_list = append(bootstrap_list, addr)
			}
		}
	}
	return bootstrap_list
}

func LoadGrapepeer(hc *HostConfig) *Grapepeer {

	// Set the path to look for the configurations file
	hd, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	configPath := filepath.Join(hd, GRAPEONE_CFG_PATH)
	var fd *os.File = nil
	configFile := filepath.Join(hd, GRAPEONE_CFG_PATH, GRAPEPEER_FILE)
	if len(hc.Config) > 0 {
		configFile = hc.Config
		fd, err = os.Open(hc.Config)
	} else {
		fd, err = os.Open(filepath.Join(hd, GRAPEONE_CFG_PATH, GRAPEPEER_FILE))
	}
	if err != nil {
		if os.IsNotExist(err) {
			// Configuration file does not exist
			return nil
		}
	}
	fd.Close()

	// Enable VIPER to read Environment Variables
	viper.AutomaticEnv()
	viper.SetConfigFile(configFile)
	viper.SetConfigType("yml")
	var peerConfig *Grapepeer = &Grapepeer{
		Host: *hc,
	}

	if err := viper.ReadInConfig(); err != nil {
		logger.Errorf("Error reading %s file, %s", GRAPEPEER_FILE, err)
		return nil
	}

	// Set undefined variables
	viper.SetDefault("peer.host", "0.0.0.0")
	viper.SetDefault("peer.port", 0)
	viper.SetDefault("peer.console", 1)
	viper.SetDefault("peer.logging", 5)
	viper.SetDefault("peer.bandwidth", 0)
	viper.SetDefault("peer.visualize", 0)
	viper.SetDefault("peer.purge", 0)
	viper.SetDefault("peer.stats", 0)
	viper.SetDefault("peer.qsize", 16)
	viper.SetDefault("peer.msize", 16)
	viper.SetDefault("peer.qsync", true)
	viper.SetDefault("peer.network", 2)
	viper.SetDefault("peer.snapshotsync", true)
	viper.SetDefault("peer.apiauthdisabled", false)
	viper.SetDefault("dag.algorithm", "default")
	viper.SetDefault("dag.alpha", 0.5)
	viper.SetDefault("dag.approvetx", 2)
	viper.SetDefault("dag.initialwidth", 5)
	viper.SetDefault("dag.lambda", 1000)
	viper.SetDefault("dag.totaltx", 0)
	viper.SetDefault("dag.cummstep", 0.1)
	viper.SetDefault("dag.wallet", "0x1a6c77929698e36981b9b0e0486a253ae33185e6")
	viper.SetDefault("dag.publickey", "4608a6e0c0c512c7d3a00a7c7dc54202a7c3965cdad16f010eca4647aebe1c28")
	viper.SetDefault("dag.privatekey", "f43e4ede273453d86183aed3442d69e0052bbe5776b4200f1354277da9f6be29")
	// Must be a full 20-byte address: the smart-contract stage parses it as one
	// when building every pin. Defaults to the zero address, i.e. fees collected
	// by the VM are burned until an operator configures a real account.
	viper.SetDefault("dag.coinbaseaccount", ZERO_ADDRESS)
	viper.SetDefault("dag.pinthreshold", TX_PIN_DEPTH_THRESHOLD)
	viper.SetDefault("dag.versioncollision", false)
	viper.SetDefault("tx.maxfuellimit", 10000000)
	viper.SetDefault("tx.maxfuelprice", 10000000)
	viper.SetDefault("tx.neutrino", 0.00000001)

	err = viper.Unmarshal(&peerConfig)
	if err != nil {
		logger.Errorf("Unable to decode into Grapepeer, %v", err)
		return nil
	}
	if err := validateAddress(peerConfig.Dag.Coinbaseaccount); err != nil {
		logger.Errorf("Invalid dag.coinbaseaccount %q: %s", peerConfig.Dag.Coinbaseaccount, err.Error())
		return nil
	}
	peerConfig.Peer.Apikey = filepath.Join(configPath, peerConfig.Peer.Apikey)
	peerConfig.Peer.Apicert = filepath.Join(configPath, peerConfig.Peer.Apicert)
	// Overwrite the num of approved tx - this is the optimal value already
	// peerConfig.Dag.Approvetx = 2
	return peerConfig
}

func (dag *DagConfiguration) String() string {
	return fmt.Sprintf("[DAG Config] Algorithm:%s, Alpha:%f, ApproveTx:%d, InitialWidth:%d, Lambda:%d, TotalTx:%d",
		dag.Algorithm, dag.Alpha, dag.Approvetx, dag.Initialwidth, dag.Lambda, dag.Totaltx,
	)
}
