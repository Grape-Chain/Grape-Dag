package config

import (
	"os"
	"time"

	"github.com/multiformats/go-multiaddr"
)

const (
	PRE_RENDEZVOUS_ID         = "/GrapeOne/43312633-80cf-429d-bc92-a17ca501aa5b"
	PRE_DAGSYNC_ID            = "/dagsync/0.0.1" // identify the subscription topic for synchronization
	APP_NAME                  = "Grape Node"
	LOG_FILE_PREFIX           = "%s.log"
	GOLOG_FILE                = "GOLOG_FILE"
	GOLOG_OUTPUT              = "GOLOG_OUTPUT"
	GOLOG_DEFAULT_OUTPUT_MODE = "file"
	GRAPEONE_CFG_PATH         = ".grap3"
	BOOTSTRAP_FILE            = "bootstrap.json"
	GENERATOR_NAME            = "txgenerator"
	GENERATOR_EXT             = "yml"
	GENERATOR_FILE            = GENERATOR_NAME + "." + GENERATOR_EXT
	GRAPEPEER_FILE            = "grapepeer.yml"
	MAX_CONN_LIMIT            = 100
	DB_CTX_TIMEOUT            = 5
	STATS_DB                  = "mongo"
	TX_WEIGHT_MEAN            = 3.0 // tx weight mean value
	TX_WEIGHT_UPPER_LIMIT     = 6.0 //
	TX_WEIGHT_LOWER_LIMIT     = 0.6 // tx weigth bounds used when generating transactions
	TX_PIN_DEPTH_THRESHOLD    = 100 // threshold value for generating a pinning transaction
	// REST_API_USERNAME and REST_API_PASSWORD are exported as vars below
	// (read from the GRAPE_REST_API_USERNAME and GRAPE_REST_API_PASSWORD
	// environment variables). They are not constants because we want
	// operators to set them at process start.
	NET_CONN_TIMEOUT               = 15 // timeout time in seconds before we stop trying to connect
	PIN_TX_TIMER_DEF               = time.Second * 5
	QUEUE_RELIEF_DELAY             = time.Millisecond * 1000
	QUEUE_RELIEF_SIZE              = 7500
	QUEUE_GEN_SIZE                 = 100
	QUEUE_WARN_SIZE                = 1000
	PUBSUB_DELAY                   = 20. // check if the arriving messages are running past this delay time
	NET_DISCOVERY_CYCLE            = 15  // how frequently node discovery has to run to find new peers
	PUBSUB_QUEUE_YIELD_MIN         = 5
	PUBSUB_QUEUE_YIELD_MAX         = 25
	PUBSUB_QUEUE_YEILD_INCR        = 1 // We will increment queue wait on empty by this value
	ADDR_REGEXP                    = "\\.*((127\\.)|(::1))\\.*"
	ANNOUNCE_TXT_PERIOD            = 30
	WAIT_FOR_STATE_CHANGE_DISPATCH = 100 // time to wait for a state machine state change to DISPATH_END
	KB                             = 1 << 10
	MB                             = KB << 10
	GB                             = MB << 10
	GRAPE                          = "\U0001F347"
	LOGGER_ROOT_ID                 = "grap"
	LOGGER_FN                      = "grape-%s"
	ACTIVATION_OK                  = 0x10
	USE_ACTIVATION                 = false
	GRAPEPEER_ID                   = "GRAPEPEER_ID"
	ROUTING_TABLE_REFRESH          = 30
	STATE_CHANGE_WAIT              = 1000
	RELAY_SRV_1                    = "/dns4/bootstrap1/udp/43431/quic-v1/p2p/12D3KooWLMy6TucfkW55NmYH9FGwfSnLPZPiEEicqyJzfubfLob1"
	RELAY_SRV_2                    = "/dns4/bootstrap2/udp/43431/quic-v1/p2p/12D3KooWQYaGFAHo5zSsJR4Sgdpv9AoCWU9y6aTdAvmVjg6rcmzf"
	// ADDRESS_BYTE_LEN - length in bytes of an account address
	ADDRESS_BYTE_LEN = 20
	// ZERO_ADDRESS - the all-zero account address; used as a burn destination
	ZERO_ADDRESS = "0x0000000000000000000000000000000000000000"
)

type Grapepeer struct {
	Peer PeerConfiguration
	Dag  DagConfiguration
	Tx   TxConfiguration
	Host HostConfig
}

type PeerConfiguration struct {
	Host            string // "0.0.0.0"
	Id              string
	Port            int // 0 - random port
	Console         int // 0 - no console output, 1 - console output
	Bandwidth       int // Bandwidth estimation: 0 - off, 1 - on
	Visualize       int // 0 - no, 1 - yes
	Purge           int
	Mdns            int
	Stats           int
	Grpc            bool // true - run grpc service in order to publish transactions
	Grpcport        int  // grpc service port to listen on
	Network         int  // Network, this node runs on: 0 - MAINNET; 1 - PUBLIC_TESTNET; 2 - PRIVATE_TESTNET
	Leader          bool
	NodeType        int    // 0 - fullnode, 1 - peernode, 2 - light node
	Apiport         int    // rest api port the server bind to
	ApiTlsEnabled   bool   // whether enable TLS on API or not
	Apikey          string //private key location and name for tls on api (if enabled)
	Apicert         string // cert location and name for tls on api (if enabled)
	ApiAuthDisabled bool   // run the REST API with no authentication (local development only)
	Walletdir       string // directory holding the web wallet assets; empty disables the /wallet route when absent
	StateServerPort int    // port of the gRPC server serving state for SMC VM
	VmServerPort    int    // port of the smart contract vm server where send contracts to
	Logging         int    // Log level: -1 Debug (through CLI only), 0 - Info, 1 - Warn, 2 - Error
	Qsize           int    // pubsub outbound queue size in mb
	Msize           int    // pubsub max message size in mb
	Qsync           bool   // this flag tells the lock free queues to use channel for sync
	SnapshotSync    bool   // sync empty node using leader's balance snapshot or not
}

type DagConfiguration struct {
	Algorithm        string  //algorithm to use: "mcmc+", "mcmc++"
	Alpha            float64 // Power coefficient MCMC+
	Approvetx        uint16  // num of transactions to approve in DAG
	Delay            uint16  // extra delay in seconds
	Initialwidth     uint16  // Initial width of the dag - num of nodes from the genesis node
	Lambda           uint32  // Tx generation rate
	Totaltx          uint32  // Total num of transactions to generate; 0 - infinite
	Cummstep         float64 // Cummulative weight increment
	Pinthreshold     uint64  // Depth of DAG before generating a pinning transaction
	Wallet           string
	Privatekey       string
	Publickey        string
	FaucetWallet     string
	FaucetPrivatekey string
	FaucetPublickey  string
	Coinbaseaccount  string
	Versioncollision bool   // Enable or disable version collision in DAG, enabling is a very expensive operation!
	Confirmation     string // Confirmation rule: "share100" (paper section 5.1) or "legacy" (two direct approvers)
	Tiptimeout       uint32 // Seconds before an unapproved tip stops counting towards the confirmation denominator; 0 disables
	Slicing          bool   // Move sites settled by a commit transaction out of the live graph into the slice archive
}

type TxConfiguration struct {
	Maxfuellimit uint64
	Maxfuelprice uint64
	Neutrino     float64
}

type HostConfig struct {
	Rendezvous      string
	ProtocolID      string
	PeerID          string
	Bootstrap       bool
	Seed            int64
	Debug           bool
	Grpc            bool
	Leader          bool
	NodeType        int
	WaitConnect     bool
	Profile         bool
	Bootstrap_peers []multiaddr.Multiaddr
	Port            int
	Grpcport        int
	Apiport         int
	Genesis         bool
	Stats           bool
	Pubsub_tracing  bool
	Wallet          string
	PrivateKey      string
	PublicKey       string
	Single          bool // this is for testing only
	VmServerPort    int
	StateServerPort int
	Home            bool
	Purge           bool
	Verbose         int
	Config          string
	Activation      string
	Secret          string
	SnapshotSync    bool // sync empty node using leader's balance snapshot or not
}

// REST_API_USERNAME and REST_API_PASSWORD are loaded from the environment.
// If unset, the REST API runs without HTTP basic-auth credentials configured;
// the service is expected to require them at startup or short-circuit auth.
var (
	REST_API_USERNAME = os.Getenv("GRAPE_REST_API_USERNAME")
	REST_API_PASSWORD = os.Getenv("GRAPE_REST_API_PASSWORD")
)
