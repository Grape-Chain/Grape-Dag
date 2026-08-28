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
	Peer  PeerConfiguration
	Dag   DagConfiguration
	Tx    TxConfiguration
	Host  HostConfig
	Store StoreConfiguration
}

// StoreConfiguration - where the commit-transaction chain is kept on disk.
type StoreConfiguration struct {
	Enabled bool   // persist the chain, so a restart does not resync from scratch
	Path    string // directory, relative to the config directory unless absolute
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
	// Peeroutboundqueue - how many messages may be queued for one peer before
	// gossipsub starts dropping them. Messages, not bytes; the library's own
	// default is 32.
	//
	// This replaces Qsize, which was documented as "pubsub outbound queue size in
	// mb" and passed to WithPeerOutboundQueueSize, whose unit is messages. At the
	// default of 16 that asked for 16,777,216 queued messages per peer - not an
	// allocation, because the queue grows lazily, but a drop boundary moved far
	// past what the process could survive reaching. Renamed rather than
	// reinterpreted: silently changing what qsize: 16 means would give an
	// operator a 16-message queue, below the library default, and drops instead
	// of a fix.
	Peeroutboundqueue int
	Msize             int  // pubsub max message size in mb
	Qsync             bool // this flag tells the lock free queues to use channel for sync
	SnapshotSync      bool // sync empty node using leader's balance snapshot or not
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
	Versioncollision bool // Enable or disable version collision in DAG, enabling is a very expensive operation!
	// Confirmation - which rule decides a site is confirmed: the technical
	// paper's share-of-tips measure (anything but "legacy"), or the original
	// fixed two-direct-approver count ("legacy"). How large the share has to be
	// is Confirmshare, below.
	Confirmation string
	Tiptimeout   uint32 // Seconds before an unapproved tip stops counting towards the confirmation denominator; 0 disables
	// Walkdepth - the technical paper's W, "depth of throwing of a random walk
	// particle": how many approvals below a tip a tip-selection walk starts.
	// Bounds the per-transaction cost of selection by construction, and is what
	// keeps the walk working on the recent frontier instead of on the oldest
	// part of the unconfirmed region. 0 falls back to the default.
	Walkdepth uint16
	// Confirmshare - share of the live tips that must confirm a site before it
	// is irrevocably confirmed, in permille. 1000 is the technical paper's
	// literal 100%.
	Confirmshare uint16
	// Pinsigners - public keys, hex, comma separated, whose commit transactions
	// this node will apply. Empty means the signer is adopted from the
	// chain-opening statement instead, which trusts whichever peer supplied the
	// chain. See dag/pinauth.go.
	Pinsigners string
	// Consensus - what makes a commit transaction applicable: "leader" (a single
	// authorised signer asserts it, the behaviour so far) or "quorum" (at least
	// two thirds of Validators agreed to it).
	Consensus string
	// Validators - public keys, hex, comma separated, forming the validator set
	// in quorum mode. The quorum is derived from the size of the set.
	Validators string
	Slicing    bool // Move sites settled by a commit transaction out of the live graph into the slice archive
}

type TxConfiguration struct {
	Maxfuellimit uint64
	Maxfuelprice uint64
	Neutrino     float64

	// The fee and reward settings. See docs/economics.md, which is the document
	// to read before changing any of them: they decide what a payment costs and
	// how the proceeds are divided, so a node that disagrees with the network
	// about one of them will reject payments everyone else accepts.

	// Feemode - how a payment's fee is set. "fixed" is the only implemented
	// mode: the fee is a minimum, not a bid, and under congestion transactions
	// queue rather than outbid each other. The setting exists so that adding a
	// fee market later is not a migration.
	Feemode string
	// Minpaymentfee - the least a payment may pay, in neutrinos. Charged as
	// fuel_limit x fuel_price with the limit fixed at 1, so the fee rides on
	// fields a payment already carries and there is one number to reason about.
	Minpaymentfee uint64
	// Feestartpin - the commit-transaction number at which fees begin. Negative
	// means never, which is the default: before it, the fee is zero and every
	// path behaves as it did before fees existed. Must be agreed network-wide
	// before it is set.
	Feestartpin int64
	// Minstake - where the stake bonus starts, in neutrinos. Not a gate: a
	// processor below it still earns for the work it did, at the base weight.
	Minstake uint64
	// Stakecapmilli - ceiling on the stake bonus, in permille. 1000 means stake
	// is worth nothing and rewards are purely work-based; larger tilts rewards
	// toward big holders. The main economic dial.
	Stakecapmilli uint32
}

// FeesActive - whether fees are charged for a commit transaction at this height.
//
// A single place to ask, because the answer is needed on the client path, on the
// peer path and again when the commit transaction is built, and three copies of
// the comparison is three chances to get the boundary wrong.
func (t TxConfiguration) FeesActive(pinNumber int64) bool {
	return t.Feestartpin >= 0 && pinNumber >= t.Feestartpin
}

// MinimumPaymentFee - the least a payment may pay at this height, in neutrinos.
// Zero before fees start.
func (t TxConfiguration) MinimumPaymentFee(pinNumber int64) uint64 {
	if !t.FeesActive(pinNumber) {
		return 0
	}
	return t.Minpaymentfee
}

type HostConfig struct {
	Rendezvous  string
	ProtocolID  string
	PeerID      string
	Bootstrap   bool
	Seed        int64
	Debug       bool
	Grpc        bool
	Leader      bool
	NodeType    int
	WaitConnect bool
	Profile     bool
	// Metricsaddr - where the diagnostics server (pprof and /metrics) listens.
	// Loopback by default: it exposes profiles and internal counters, so opening
	// it to a network has to be a decision someone makes.
	Metricsaddr     string
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
