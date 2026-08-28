package main

// The provisioning half of grapepeer: everything a wallet application needs to
// do to a machine before that machine can be started as a processing node, and
// nothing that starts one.
//
// Provisioning and running are kept apart on purpose. A wallet application wants
// to write the files, show the operator what it wrote, and then start the node
// as a separate supervised process it can restart; a single command that did
// both would give it no point at which to do either.

import (
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/Grape-Chain/Grape-Dag/config"
	grapecrypto "github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/multiformats/go-multiaddr"
)

//go:embed grapepeer.yml.tmpl
var configTemplate string

const (
	// walletFileName - default name of the wallet file inside the data
	// directory.
	walletFileName = "wallet.json"
	// credentialsFileName - where the node's REST API credentials are written.
	// A file rather than the printed output: it is loaded into the node's
	// environment, and a password that has been on a terminal is a password in
	// somebody's shell history.
	credentialsFileName = "api-credentials.env"
	// ledgerSubdir - where this node keeps its copy of the chain, under the data
	// directory.
	ledgerSubdir = "data/ledger"

	defaultApiPort = 33330
	// defaultNetwork - PRIVATE_TESTNET. A node has to be told to join anything
	// more consequential than that.
	defaultNetwork = 2

	// keyByteLen - Ed25519 seed and public key are both 32 bytes. Checked before
	// the key reaches crypto.LoadWallet, which panics on anything it dislikes.
	keyByteLen = 32

	// filePerm - the config and the wallet hold a private key; the credentials
	// file holds a password. None of them is readable by anyone but the owner.
	filePerm = 0o600
	// dirPerm - and neither is the directory listable.
	dirPerm = 0o700
)

type joinOptions struct {
	walletFile     string
	network        int
	bootstrapNodes string
	dataDir        string
	apiPort        int
	force          bool
}

// joinResult - what provisioning wrote. Returned rather than only printed so
// that a caller, and a test, can check it without parsing prose.
type joinResult struct {
	dataDir         string
	walletFile      string
	walletCreated   bool
	walletAddress   string
	configFile      string
	bootstrapFile   string
	bootstrapPeers  int
	credentialsFile string
	credentialsKept bool
	peerID          string
	apiPort         int
}

func runJoin(args []string, out io.Writer) error {
	opts, err := parseJoinFlags(args, out)
	if err != nil {
		return err
	}
	result, err := provision(opts)
	if err != nil {
		return err
	}
	printJoinSummary(out, result)
	return nil
}

func parseJoinFlags(args []string, out io.Writer) (*joinOptions, error) {
	fs := flag.NewFlagSet("grapepeer join", flag.ContinueOnError)
	fs.SetOutput(out)
	opts := &joinOptions{}
	fs.StringVar(&opts.walletFile, "wallet-file", "",
		"wallet to run this node as; generated and written here when the file does not exist (default <data-dir>/"+walletFileName+")")
	fs.IntVar(&opts.network, "network", defaultNetwork,
		"network to join: 0 - MAINNET, 1 - PUBLIC_TESTNET, 2 - PRIVATE_TESTNET")
	fs.StringVar(&opts.bootstrapNodes, "bootstrap-nodes", "",
		"comma separated multiaddrs of peers to get started from")
	fs.StringVar(&opts.dataDir, "data-dir", "",
		"directory holding this node's configuration and ledger (default $HOME/"+config.GRAPEONE_CFG_PATH+")")
	fs.IntVar(&opts.apiPort, "api-port", defaultApiPort,
		"port the node's REST API and the bundled page listen on")
	fs.BoolVar(&opts.force, "force", false,
		"overwrite an existing configuration; never overwrites the wallet file")
	fs.Usage = func() {
		fmt.Fprintln(out, "Usage: grapepeer join [flags]")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Provision this machine as a processing node. Writes the node's")
		fmt.Fprintln(out, "configuration, bootstrap list and API credentials, and prints the command")
		fmt.Fprintln(out, "that starts it. It does not start anything itself.")
		fmt.Fprintln(out)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() > 0 {
		return nil, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if opts.network < 0 || opts.network > 2 {
		return nil, fmt.Errorf("--network must be 0, 1 or 2, got %d", opts.network)
	}
	// Below 1024 needs privileges this node should not be asking for.
	if opts.apiPort < 1024 || opts.apiPort > 65535 {
		return nil, fmt.Errorf("--api-port must be between 1024 and 65535, got %d", opts.apiPort)
	}
	return opts, nil
}

func provision(opts *joinOptions) (*joinResult, error) {
	dataDir, err := resolveDataDir(opts.dataDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dataDir, dirPerm); err != nil {
		return nil, fmt.Errorf("cannot create the data directory %s: %w", dataDir, err)
	}

	// Parsed before anything is written: a typo in a multiaddr should stop the
	// command, not leave half a configuration behind.
	peers, err := parseBootstrapNodes(opts.bootstrapNodes)
	if err != nil {
		return nil, err
	}

	configFile := filepath.Join(dataDir, config.GRAPEPEER_FILE)
	if err := refuseExisting(configFile, opts.force); err != nil {
		return nil, err
	}
	bootstrapFile := filepath.Join(dataDir, config.BOOTSTRAP_FILE)
	if len(peers) > 0 {
		if err := refuseExisting(bootstrapFile, opts.force); err != nil {
			return nil, err
		}
	}

	walletFile := opts.walletFile
	if walletFile == "" {
		walletFile = filepath.Join(dataDir, walletFileName)
	}
	wallet, walletCreated, err := loadOrCreateWallet(walletFile)
	if err != nil {
		return nil, err
	}

	credentialsFile := filepath.Join(dataDir, credentialsFileName)
	credentialsKept, err := ensureCredentials(credentialsFile, wallet.WalletAddress(), opts.force)
	if err != nil {
		return nil, err
	}

	rendered, err := renderConfig(opts, wallet, dataDir, credentialsFile)
	if err != nil {
		return nil, err
	}
	if err := writeFile(configFile, rendered, opts.force); err != nil {
		return nil, err
	}

	if len(peers) > 0 {
		payload, err := json.MarshalIndent(bootstrapPayload(peers), "", "    ")
		if err != nil {
			return nil, fmt.Errorf("cannot encode the bootstrap list: %w", err)
		}
		if err := writeFile(bootstrapFile, append(payload, '\n'), opts.force); err != nil {
			return nil, err
		}
	}

	return &joinResult{
		dataDir:         dataDir,
		walletFile:      walletFile,
		walletCreated:   walletCreated,
		walletAddress:   wallet.WalletAddress(),
		configFile:      configFile,
		bootstrapFile:   bootstrapFile,
		bootstrapPeers:  len(peers),
		credentialsFile: credentialsFile,
		credentialsKept: credentialsKept,
		peerID:          peerIDFor(wallet.WalletAddress()),
		apiPort:         opts.apiPort,
	}, nil
}

func resolveDataDir(given string) (string, error) {
	if given != "" {
		return filepath.Abs(given)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot resolve the home directory; pass --data-dir: %w", err)
	}
	return filepath.Join(home, config.GRAPEONE_CFG_PATH), nil
}

// refuseExisting - a configuration already on disk is somebody's node. Replacing
// it has to be asked for.
func refuseExisting(path string, force bool) error {
	if force {
		return nil
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists; pass --force to replace it", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("cannot inspect %s: %w", path, err)
	}
	return nil
}

func writeFile(path string, data []byte, force bool) error {
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if force {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, filePerm)
	if err != nil {
		return fmt.Errorf("cannot write %s: %w", path, err)
	}
	// An existing file keeps the mode it was created with, so set it explicitly:
	// --force must not leave a world-readable file holding a private key.
	if err := f.Chmod(filePerm); err != nil {
		f.Close()
		return fmt.Errorf("cannot set the mode of %s: %w", path, err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("cannot write %s: %w", path, err)
	}
	return f.Close()
}

// walletFileContents - the on-disk shape of a wallet, matching what
// crypto.Wallet marshals to.
type walletFileContents struct {
	PrivateKey    string `json:"private_key"`
	PublicKey     string `json:"public_key"`
	WalletAddress string `json:"wallet_address"`
}

// loadOrCreateWallet - read the wallet at path, or generate one and write it
// there.
//
// An existing wallet file is never rewritten, not even with --force: the private
// key in it is the only copy of an account, and a command that provisions a node
// has no business destroying one.
func loadOrCreateWallet(path string) (*grapecrypto.Wallet, bool, error) {
	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
		wallet, err := walletFromFile(path, raw)
		return wallet, false, err
	case !errors.Is(err, os.ErrNotExist):
		return nil, false, fmt.Errorf("cannot read the wallet file %s: %w", path, err)
	}

	wallet := grapecrypto.NewWallet()
	encoded, err := wallet.MarshalJSON()
	if err != nil {
		return nil, false, fmt.Errorf("cannot encode the new wallet: %w", err)
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, dirPerm); err != nil {
			return nil, false, fmt.Errorf("cannot create %s: %w", dir, err)
		}
	}
	// Exclusive, never forced: see above.
	if err := writeFile(path, append(encoded, '\n'), false); err != nil {
		return nil, false, err
	}
	return wallet, true, nil
}

func walletFromFile(path string, raw []byte) (*grapecrypto.Wallet, error) {
	var contents walletFileContents
	if err := json.Unmarshal(raw, &contents); err != nil {
		return nil, fmt.Errorf("%s is not a wallet file: %w", path, err)
	}
	// Validated here because crypto.LoadWallet panics on a malformed key, and a
	// mistyped path should be an error message rather than a stack trace.
	if err := checkKey("public_key", contents.PublicKey); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := checkKey("private_key", contents.PrivateKey); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	wallet := grapecrypto.LoadWallet(contents.PublicKey, contents.PrivateKey)
	if contents.WalletAddress != "" && !strings.EqualFold(contents.WalletAddress, wallet.WalletAddress()) {
		return nil, fmt.Errorf("%s: wallet_address %s does not belong to public_key; refusing to run as an account this file does not own",
			path, contents.WalletAddress)
	}
	return wallet, nil
}

func checkKey(field, value string) error {
	if value == "" {
		return fmt.Errorf("%s is missing", field)
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(value, "0x"))
	if err != nil {
		return fmt.Errorf("%s is not hex: %w", field, err)
	}
	if len(decoded) != keyByteLen {
		return fmt.Errorf("%s must be %d bytes, got %d", field, keyByteLen, len(decoded))
	}
	return nil
}

// ensureCredentials - make sure the node has REST API credentials, returning
// true when it already had some.
//
// Existing credentials are kept even under --force. Rotating them would lock out
// whatever wallet application is already talking to this node, and the operator
// who wants new ones can delete the file.
func ensureCredentials(path, walletAddress string, force bool) (kept bool, err error) {
	if _, statErr := os.Stat(path); statErr == nil {
		return true, nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return false, fmt.Errorf("cannot inspect %s: %w", path, statErr)
	}
	password, err := randomSecret()
	if err != nil {
		return false, err
	}
	body := fmt.Sprintf(`# REST API credentials for this node, written by 'grapepeer join' on %s.
# The node reads them from its environment, so load them before starting it:
#
#   set -a; . %s; set +a
#
# Anyone holding these can start and stop this node's processing. The file is
# %04o for that reason.
GRAPE_REST_API_USERNAME=%s
GRAPE_REST_API_PASSWORD=%s
`, time.Now().UTC().Format(time.RFC3339), path, filePerm, apiUsername(walletAddress), password)
	// Exclusive: reaching here means the file did not exist a moment ago, and if
	// it does now, something else is provisioning this directory.
	if err := writeFile(path, []byte(body), false); err != nil {
		return false, err
	}
	return false, nil
}

// randomSecret - 32 bytes from the system source, base64 without padding. Long
// enough that the API password is not the weak part of the node.
func randomSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("cannot read random bytes for the API password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// apiUsername - a name that identifies which node the credentials belong to. The
// username is not a secret; deriving it from the account makes a credentials
// file recognisable when an operator has several.
func apiUsername(walletAddress string) string {
	return "node-" + shortAddress(walletAddress)
}

// peerIDFor - the -id a node is started with. It has to be stable across
// restarts, because the libp2p key on disk is filed under it.
func peerIDFor(walletAddress string) string {
	return "node-" + shortAddress(walletAddress)
}

func shortAddress(walletAddress string) string {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(walletAddress, "0x"), "0X")
	if len(trimmed) > 8 {
		trimmed = trimmed[:8]
	}
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
}

func parseBootstrapNodes(list string) ([]string, error) {
	var peers []string
	for _, entry := range strings.Split(list, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		// Parsed, not just copied: a node that cannot reach any bootstrap peer
		// sits there looking like a network problem.
		if _, err := multiaddr.NewMultiaddr(entry); err != nil {
			return nil, fmt.Errorf("--bootstrap-nodes: %q is not a multiaddr: %w", entry, err)
		}
		peers = append(peers, entry)
	}
	return peers, nil
}

// bootstrapPayload - the shape config.LoadBootstrap reads: an object whose
// values are multiaddrs. The keys are ignored by the loader and are only there
// to make the file readable.
func bootstrapPayload(peers []string) map[string]string {
	payload := make(map[string]string, len(peers))
	for i, addr := range peers {
		payload[fmt.Sprintf("peer%d", i+1)] = addr
	}
	return payload
}

type configValues struct {
	Network         int
	ApiPort         int
	PublicKey       string
	PrivateKey      string
	WalletAddress   string
	StorePath       string
	CredentialsFile string
}

func renderConfig(opts *joinOptions, wallet *grapecrypto.Wallet, dataDir, credentialsFile string) ([]byte, error) {
	tmpl, err := template.New(config.GRAPEPEER_FILE).Parse(configTemplate)
	if err != nil {
		return nil, fmt.Errorf("the embedded configuration template is broken: %w", err)
	}
	values := configValues{
		Network:       opts.network,
		ApiPort:       opts.apiPort,
		PublicKey:     wallet.PublicKeyStr(),
		PrivateKey:    wallet.PrivateKeyStr(),
		WalletAddress: wallet.WalletAddress(),
		// ToSlash so the rendered YAML never carries a backslash escape.
		StorePath:       filepath.ToSlash(filepath.Join(dataDir, ledgerSubdir)),
		CredentialsFile: filepath.ToSlash(credentialsFile),
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, values); err != nil {
		return nil, fmt.Errorf("cannot render the configuration: %w", err)
	}
	return []byte(buf.String()), nil
}

// printJoinSummary - what was written and what to run next.
//
// The wallet address is printed; the private key and the API password are not.
// Both are in files this command has just made unreadable to anybody else, and
// putting either on a terminal would undo that.
func printJoinSummary(out io.Writer, r *joinResult) {
	fmt.Fprintf(out, "Provisioned a processing node in %s\n\n", r.dataDir)
	fmt.Fprintf(out, "  wallet       %s\n", r.walletAddress)
	if r.walletCreated {
		fmt.Fprintf(out, "               new keys written to %s\n", r.walletFile)
	} else {
		fmt.Fprintf(out, "               keys loaded from %s\n", r.walletFile)
	}
	fmt.Fprintf(out, "  config       %s\n", r.configFile)
	if r.bootstrapPeers > 0 {
		fmt.Fprintf(out, "  bootstrap    %s (%d peer(s))\n", r.bootstrapFile, r.bootstrapPeers)
	} else {
		fmt.Fprintf(out, "  bootstrap    not written; pass --bootstrap-nodes to set one\n")
	}
	if r.credentialsKept {
		fmt.Fprintf(out, "  API auth     %s (kept as it was)\n", r.credentialsFile)
	} else {
		fmt.Fprintf(out, "  API auth     %s (new credentials)\n", r.credentialsFile)
	}

	fmt.Fprintf(out, "\nThis command did not start the node. To start it:\n\n")
	fmt.Fprintf(out, "  set -a; . %s; set +a\n", r.credentialsFile)
	fmt.Fprintf(out, "  %s\n", startCommand(r))
	fmt.Fprintf(out, "\nThen open http://127.0.0.1:%d/node/ to start and stop processing,\n", r.apiPort)
	fmt.Fprintf(out, "or run: grapepeer status --api-port %d\n", r.apiPort)
}

// startCommand - the exact command that runs this node.
//
// The -f is only present when the configuration is somewhere other than the
// single path the loader looks in by itself.
func startCommand(r *joinResult) string {
	cmd := fmt.Sprintf("grapepeer -id %s", r.peerID)
	if home, err := os.UserHomeDir(); err != nil ||
		r.configFile != filepath.Join(home, config.GRAPEONE_CFG_PATH, config.GRAPEPEER_FILE) {
		cmd += fmt.Sprintf(" -f %s", r.configFile)
	}
	return cmd
}

// ---------------------------------------------------------------------------
// grapepeer status - read the node's own /node/status over its REST API.
// ---------------------------------------------------------------------------

func runStatus(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("grapepeer status", flag.ContinueOnError)
	fs.SetOutput(out)
	host := fs.String("host", "127.0.0.1", "host the node's REST API is on")
	apiPort := fs.Int("api-port", defaultApiPort, "port the node's REST API is on")
	dataDir := fs.String("data-dir", "", "directory holding "+credentialsFileName+" (default $HOME/"+config.GRAPEONE_CFG_PATH+")")
	fs.Usage = func() {
		fmt.Fprintln(out, "Usage: grapepeer status [flags]")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Ask a running node what it is doing. Reads the API credentials from the")
		fmt.Fprintln(out, "environment, or from the file grapepeer join wrote.")
		fmt.Fprintln(out)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	dir, err := resolveDataDir(*dataDir)
	if err != nil {
		return err
	}
	user, password := apiCredentials(filepath.Join(dir, credentialsFileName))

	url := fmt.Sprintf("http://%s:%d%s", *host, *apiPort, "/node/status")
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if user != "" {
		req.SetBasicAuth(user, password)
	}
	// A local node either answers promptly or is not running.
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach the node at %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("cannot read the node's answer: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the node answered %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	fmt.Fprintln(out, strings.TrimSpace(string(body)))
	return nil
}

// apiCredentials - the environment first, then the file grapepeer join wrote.
// The environment wins because that is where a supervisor puts them.
func apiCredentials(path string) (user, password string) {
	user = os.Getenv("GRAPE_REST_API_USERNAME")
	password = os.Getenv("GRAPE_REST_API_PASSWORD")
	if user != "" && password != "" {
		return user, password
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return user, password
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		switch strings.TrimSpace(key) {
		case "GRAPE_REST_API_USERNAME":
			if user == "" {
				user = strings.TrimSpace(value)
			}
		case "GRAPE_REST_API_PASSWORD":
			if password == "" {
				password = strings.TrimSpace(value)
			}
		}
	}
	return user, password
}
