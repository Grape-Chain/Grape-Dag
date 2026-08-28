package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/spf13/viper"
)

// join - run the subcommand against a directory of its own, returning what it
// printed. Every test passes --data-dir so that nothing writes to the home
// directory of whoever is running the suite.
func join(t *testing.T, dir string, extra ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	args := append([]string{"--data-dir", dir}, extra...)
	err := runJoin(args, &out)
	return out.String(), err
}

func configPath(dir string) string    { return filepath.Join(dir, config.GRAPEPEER_FILE) }
func bootstrapPath(dir string) string { return filepath.Join(dir, config.BOOTSTRAP_FILE) }
func walletPath(dir string) string    { return filepath.Join(dir, walletFileName) }
func credsPath(dir string) string     { return filepath.Join(dir, credentialsFileName) }

// readConfig - the generated file as viper sees it, which is the only reading
// that matters: viper is what the node's own loader uses.
func readConfig(t *testing.T, path string) *viper.Viper {
	t.Helper()
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yml")
	if err := v.ReadInConfig(); err != nil {
		t.Fatalf("viper cannot read the generated %s: %s", path, err)
	}
	return v
}

func TestJoinWritesAConfigViperCanParseBackWithTheExpectedKeys(t *testing.T) {
	dir := t.TempDir()
	if _, err := join(t, dir, "--network", "1", "--api-port", "34567"); err != nil {
		t.Fatalf("join: %s", err)
	}

	v := readConfig(t, configPath(dir))

	cases := []struct {
		key  string
		want any
		got  func() any
	}{
		{"peer.nodetype", 0, func() any { return v.GetInt("peer.nodetype") }},
		{"peer.network", 1, func() any { return v.GetInt("peer.network") }},
		{"peer.apiport", 34567, func() any { return v.GetInt("peer.apiport") }},
		{"peer.apiauthdisabled", false, func() any { return v.GetBool("peer.apiauthdisabled") }},
		{"peer.apitlsenabled", false, func() any { return v.GetBool("peer.apitlsenabled") }},
		{"dag.algorithm", "mcmc+", func() any { return v.GetString("dag.algorithm") }},
		{"dag.approvetx", 2, func() any { return v.GetInt("dag.approvetx") }},
		{"store.enabled", true, func() any { return v.GetBool("store.enabled") }},
	}
	for _, c := range cases {
		if got := c.got(); got != c.want {
			t.Errorf("%s = %v, want %v", c.key, got, c.want)
		}
	}
	// Present at all, as opposed to merely reading as a zero value.
	for _, key := range []string{
		"peer.nodetype", "peer.apiauthdisabled", "dag.publickey", "dag.privatekey",
		"dag.wallet", "dag.coinbaseaccount", "store.path", "tx.neutrino",
	} {
		if !v.IsSet(key) {
			t.Errorf("the generated config has no %s key", key)
		}
	}
}

func TestTheGeneratedConfigUnmarshalsIntoTheNodesOwnConfigStruct(t *testing.T) {
	dir := t.TempDir()
	if _, err := join(t, dir); err != nil {
		t.Fatalf("join: %s", err)
	}
	v := readConfig(t, configPath(dir))

	var parsed config.Grapepeer
	if err := v.Unmarshal(&parsed); err != nil {
		t.Fatalf("the generated config does not decode into config.Grapepeer: %s", err)
	}
	if parsed.Peer.NodeType != 0 {
		t.Errorf("Peer.NodeType = %d, want 0 - a processing node", parsed.Peer.NodeType)
	}
	if parsed.Peer.ApiAuthDisabled {
		t.Error("Peer.ApiAuthDisabled is true; join must not leave the API open")
	}
	if parsed.Peer.Apiport != defaultApiPort {
		t.Errorf("Peer.Apiport = %d, want %d", parsed.Peer.Apiport, defaultApiPort)
	}
	if parsed.Dag.Wallet == "" || parsed.Dag.Wallet != parsed.Dag.Coinbaseaccount {
		t.Errorf("dag.wallet %q and dag.coinbaseaccount %q must be the same account, or the node earns for somebody else",
			parsed.Dag.Wallet, parsed.Dag.Coinbaseaccount)
	}
	if !filepath.IsAbs(parsed.Store.Path) {
		t.Errorf("Store.Path = %q, want an absolute path inside the data directory", parsed.Store.Path)
	}
	if !strings.HasPrefix(filepath.Clean(parsed.Store.Path), filepath.Clean(dir)) {
		t.Errorf("Store.Path = %q, want it under the data directory %q", parsed.Store.Path, dir)
	}
}

func TestTheGeneratedConfigCarriesTheWalletFromTheWalletFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := join(t, dir); err != nil {
		t.Fatalf("join: %s", err)
	}
	var wallet walletFileContents
	raw, err := os.ReadFile(walletPath(dir))
	if err != nil {
		t.Fatalf("reading the wallet file: %s", err)
	}
	if err := json.Unmarshal(raw, &wallet); err != nil {
		t.Fatalf("the wallet file is not JSON: %s", err)
	}

	v := readConfig(t, configPath(dir))
	if got := v.GetString("dag.publickey"); got != wallet.PublicKey {
		t.Errorf("dag.publickey = %q, want the wallet's %q", got, wallet.PublicKey)
	}
	if got := v.GetString("dag.privatekey"); got != wallet.PrivateKey {
		t.Errorf("dag.privatekey does not match the wallet file")
	}
	if got := v.GetString("dag.wallet"); got != wallet.WalletAddress {
		t.Errorf("dag.wallet = %q, want %q", got, wallet.WalletAddress)
	}
}

func TestJoinGeneratesAWalletWhenTheWalletFileIsAbsent(t *testing.T) {
	dir := t.TempDir()
	out, err := join(t, dir)
	if err != nil {
		t.Fatalf("join: %s", err)
	}
	if _, err := os.Stat(walletPath(dir)); err != nil {
		t.Fatalf("no wallet file was written: %s", err)
	}
	if !strings.Contains(out, "new keys written to") {
		t.Errorf("join did not report that it generated a wallet:\n%s", out)
	}
}

func TestJoinLoadsAnExistingWalletRatherThanReplacingIt(t *testing.T) {
	dir := t.TempDir()
	if _, err := join(t, dir); err != nil {
		t.Fatalf("first join: %s", err)
	}
	before, err := os.ReadFile(walletPath(dir))
	if err != nil {
		t.Fatalf("reading the wallet file: %s", err)
	}

	out, err := join(t, dir, "--force")
	if err != nil {
		t.Fatalf("second join: %s", err)
	}
	after, err := os.ReadFile(walletPath(dir))
	if err != nil {
		t.Fatalf("reading the wallet file: %s", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("--force rewrote the wallet file; a private key is the one thing join must never replace")
	}
	if !strings.Contains(out, "keys loaded from") {
		t.Errorf("join did not report that it reused the wallet:\n%s", out)
	}
}

func TestJoinRunsAgainstAWalletFileGivenOutsideTheDataDirectory(t *testing.T) {
	dir := t.TempDir()
	elsewhere := filepath.Join(t.TempDir(), "keys", "operator.json")

	if _, err := join(t, dir, "--wallet-file", elsewhere); err != nil {
		t.Fatalf("join: %s", err)
	}
	if _, err := os.Stat(elsewhere); err != nil {
		t.Fatalf("the wallet was not written where it was asked for: %s", err)
	}
	if _, err := os.Stat(walletPath(dir)); err == nil {
		t.Error("join also wrote a wallet into the data directory")
	}
}

func TestJoinRefusesToOverwriteAnExistingConfigWithoutForce(t *testing.T) {
	dir := t.TempDir()
	if _, err := join(t, dir); err != nil {
		t.Fatalf("first join: %s", err)
	}
	before, err := os.ReadFile(configPath(dir))
	if err != nil {
		t.Fatalf("reading the config: %s", err)
	}

	_, err = join(t, dir, "--api-port", "40001")
	if err == nil {
		t.Fatal("a second join without --force succeeded")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("the error does not say how to proceed: %s", err)
	}
	after, _ := os.ReadFile(configPath(dir))
	if !bytes.Equal(before, after) {
		t.Error("the refused join changed the config anyway")
	}
}

func TestJoinReplacesAnExistingConfigWithForce(t *testing.T) {
	dir := t.TempDir()
	if _, err := join(t, dir); err != nil {
		t.Fatalf("first join: %s", err)
	}
	if _, err := join(t, dir, "--force", "--api-port", "40001"); err != nil {
		t.Fatalf("join --force: %s", err)
	}
	if got := readConfig(t, configPath(dir)).GetInt("peer.apiport"); got != 40001 {
		t.Fatalf("peer.apiport = %d, want 40001", got)
	}
}

func TestJoinWritesBootstrapNodesInTheShapeTheLoaderReads(t *testing.T) {
	dir := t.TempDir()
	addrs := []string{
		"/ip4/51.15.247.49/tcp/33331/p2p/QmfM2drCBsVihmnKa7GkVBiDEob2ebZo1MRXJpgmw9biPo",
		"/ip4/51.15.139.232/tcp/33331/p2p/QmX5DbgiKhfvByvtoMNe1VVv4RdDwMogzfU6W415JBTRL3",
	}
	if _, err := join(t, dir, "--bootstrap-nodes", strings.Join(addrs, ",")); err != nil {
		t.Fatalf("join: %s", err)
	}

	raw, err := os.ReadFile(bootstrapPath(dir))
	if err != nil {
		t.Fatalf("no bootstrap file: %s", err)
	}
	// config.LoadBootstrap reads an object and takes the values, ignoring keys.
	var decoded map[string]string
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("the bootstrap file is not an object of strings: %s", err)
	}
	if len(decoded) != len(addrs) {
		t.Fatalf("the bootstrap file holds %d peers, want %d", len(decoded), len(addrs))
	}
	for _, want := range addrs {
		found := false
		for _, got := range decoded {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Errorf("the bootstrap file does not carry %s", want)
		}
	}
}

func TestJoinDoesNotWriteABootstrapFileWhenNoNodesAreGiven(t *testing.T) {
	dir := t.TempDir()
	out, err := join(t, dir)
	if err != nil {
		t.Fatalf("join: %s", err)
	}
	if _, err := os.Stat(bootstrapPath(dir)); err == nil {
		t.Error("join wrote a bootstrap file with no peers in it")
	}
	if !strings.Contains(out, "--bootstrap-nodes") {
		t.Errorf("join did not say how to set a bootstrap list:\n%s", out)
	}
}

func TestJoinRejectsAMalformedBootstrapAddressBeforeWritingAnything(t *testing.T) {
	dir := t.TempDir()
	_, err := join(t, dir, "--bootstrap-nodes", "not-a-multiaddr")
	if err == nil {
		t.Fatal("join accepted an address that is not a multiaddr")
	}
	if !strings.Contains(err.Error(), "not-a-multiaddr") {
		t.Errorf("the error does not name the offending address: %s", err)
	}
	// Nothing half-written: the parse happens before the first file is opened.
	for _, path := range []string{configPath(dir), bootstrapPath(dir), walletPath(dir)} {
		if _, err := os.Stat(path); err == nil {
			t.Errorf("%s was written despite the failure", path)
		}
	}
}

func TestJoinWritesApiCredentialsAndLeavesAuthenticationEnabled(t *testing.T) {
	dir := t.TempDir()
	if _, err := join(t, dir); err != nil {
		t.Fatalf("join: %s", err)
	}
	raw, err := os.ReadFile(credsPath(dir))
	if err != nil {
		t.Fatalf("no credentials file: %s", err)
	}
	user, password := parseEnvFile(t, string(raw))
	if user == "" {
		t.Error("GRAPE_REST_API_USERNAME is empty")
	}
	if len(password) < 32 {
		t.Errorf("GRAPE_REST_API_PASSWORD is %d characters; too short to be a generated secret", len(password))
	}
	// The names the node reads. A rename in config/param.go has to break here.
	if !strings.Contains(string(raw), "GRAPE_REST_API_USERNAME=") ||
		!strings.Contains(string(raw), "GRAPE_REST_API_PASSWORD=") {
		t.Error("the credentials file does not use the variable names the node reads")
	}
	if readConfig(t, configPath(dir)).GetBool("peer.apiauthdisabled") {
		t.Error("join left peer.apiauthdisabled true")
	}
}

func TestJoinKeepsExistingApiCredentialsEvenWithForce(t *testing.T) {
	dir := t.TempDir()
	if _, err := join(t, dir); err != nil {
		t.Fatalf("first join: %s", err)
	}
	before, err := os.ReadFile(credsPath(dir))
	if err != nil {
		t.Fatalf("reading the credentials: %s", err)
	}
	out, err := join(t, dir, "--force")
	if err != nil {
		t.Fatalf("join --force: %s", err)
	}
	after, _ := os.ReadFile(credsPath(dir))
	if !bytes.Equal(before, after) {
		t.Error("--force rotated the API credentials and would have locked out whatever is already connected")
	}
	if !strings.Contains(out, "kept as it was") {
		t.Errorf("join did not report that it kept the credentials:\n%s", out)
	}
}

func TestJoinWritesEverySecretFileReadableByItsOwnerAlone(t *testing.T) {
	dir := t.TempDir()
	if _, err := join(t, dir, "--bootstrap-nodes", "/ip4/127.0.0.1/tcp/33331"); err != nil {
		t.Fatalf("join: %s", err)
	}
	for _, path := range []string{configPath(dir), walletPath(dir), credsPath(dir)} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %s", path, err)
		}
		if perm := info.Mode().Perm(); perm != filePerm {
			t.Errorf("%s has mode %04o, want %04o", path, perm, filePerm)
		}
	}
}

func TestJoinNeverPrintsThePrivateKeyOrTheApiPassword(t *testing.T) {
	dir := t.TempDir()
	out, err := join(t, dir)
	if err != nil {
		t.Fatalf("join: %s", err)
	}

	var wallet walletFileContents
	raw, _ := os.ReadFile(walletPath(dir))
	if err := json.Unmarshal(raw, &wallet); err != nil {
		t.Fatalf("the wallet file is not JSON: %s", err)
	}
	if strings.Contains(out, wallet.PrivateKey) {
		t.Error("join printed the private key")
	}
	creds, _ := os.ReadFile(credsPath(dir))
	_, password := parseEnvFile(t, string(creds))
	if password != "" && strings.Contains(out, password) {
		t.Error("join printed the API password")
	}
	// The address, on the other hand, is what the operator needs to see.
	if !strings.Contains(out, wallet.WalletAddress) {
		t.Errorf("join did not print the wallet address:\n%s", out)
	}
}

func TestJoinDoesNotStartTheNodeAndPrintsTheCommandThatWould(t *testing.T) {
	dir := t.TempDir()
	out, err := join(t, dir, "--api-port", "34999")
	if err != nil {
		t.Fatalf("join: %s", err)
	}
	if !strings.Contains(out, "did not start the node") {
		t.Errorf("join does not say that it started nothing:\n%s", out)
	}
	// The config is not in the one place the loader looks by itself, so the
	// printed command has to point at it.
	if !strings.Contains(out, "grapepeer -id node-") {
		t.Errorf("join does not print a start command with a peer id:\n%s", out)
	}
	if !strings.Contains(out, "-f "+configPath(dir)) {
		t.Errorf("join does not point the start command at %s:\n%s", configPath(dir), out)
	}
	if !strings.Contains(out, credsPath(dir)) {
		t.Errorf("join does not say to load the credentials:\n%s", out)
	}
	if !strings.Contains(out, ":34999/node/") {
		t.Errorf("join does not name the page that starts and stops processing:\n%s", out)
	}
}

func TestJoinRejectsSettingsAMachineCannotHonour(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"a network that does not exist", []string{"--network", "7"}, "--network"},
		{"a negative network", []string{"--network", "-1"}, "--network"},
		{"a privileged API port", []string{"--api-port", "80"}, "--api-port"},
		{"an API port above the range", []string{"--api-port", "70000"}, "--api-port"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			_, err := join(t, dir, c.args...)
			if err == nil {
				t.Fatalf("join %v succeeded", c.args)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("the error does not mention %s: %s", c.want, err)
			}
			if _, err := os.Stat(configPath(dir)); err == nil {
				t.Error("a config was written despite the invalid setting")
			}
		})
	}
}

func TestJoinRejectsAWalletFileThatIsNotOne(t *testing.T) {
	cases := []struct {
		name     string
		contents string
		want     string
	}{
		{"not JSON at all", "hello", "not a wallet file"},
		{"no keys", `{}`, "public_key is missing"},
		{"a public key that is not hex", `{"public_key":"zz","private_key":"00"}`, "not hex"},
		{"a key of the wrong length", `{"public_key":"00","private_key":"00"}`, "must be 32 bytes"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "given.json")
			if err := os.WriteFile(path, []byte(c.contents), 0o600); err != nil {
				t.Fatalf("writing the wallet file: %s", err)
			}
			_, err := join(t, dir, "--wallet-file", path)
			if err == nil {
				t.Fatalf("join accepted %s as a wallet", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("the error is %q, want it to mention %q", err, c.want)
			}
		})
	}
}

func TestJoinRefusesAWalletFileWhoseAddressDoesNotMatchItsKey(t *testing.T) {
	dir := t.TempDir()
	// Provision once to get a valid pair, then claim a different account.
	if _, err := join(t, dir); err != nil {
		t.Fatalf("join: %s", err)
	}
	var wallet walletFileContents
	raw, _ := os.ReadFile(walletPath(dir))
	if err := json.Unmarshal(raw, &wallet); err != nil {
		t.Fatalf("the wallet file is not JSON: %s", err)
	}
	wallet.WalletAddress = "0x0000000000000000000000000000000000000001"
	tampered := filepath.Join(t.TempDir(), "tampered.json")
	encoded, _ := json.Marshal(wallet)
	if err := os.WriteFile(tampered, encoded, 0o600); err != nil {
		t.Fatalf("writing the tampered wallet: %s", err)
	}

	_, err := join(t, t.TempDir(), "--wallet-file", tampered)
	if err == nil {
		t.Fatal("join accepted a wallet file whose address does not belong to its key")
	}
	if !strings.Contains(err.Error(), "does not belong to public_key") {
		t.Errorf("the error does not explain the mismatch: %s", err)
	}
}

func TestJoinRejectsAnUnexpectedArgument(t *testing.T) {
	var out bytes.Buffer
	err := runJoin([]string{"--data-dir", t.TempDir(), "start"}, &out)
	if err == nil {
		t.Fatal("join accepted a bare argument")
	}
	if !strings.Contains(err.Error(), "start") {
		t.Errorf("the error does not name the argument: %s", err)
	}
}

func TestThePeerIdJoinPrintsIsStableAcrossRuns(t *testing.T) {
	// The libp2p key on disk is filed under the peer id, so a node that came
	// back with a different one would come back as a different peer.
	dir := t.TempDir()
	first, err := join(t, dir)
	if err != nil {
		t.Fatalf("first join: %s", err)
	}
	second, err := join(t, dir, "--force")
	if err != nil {
		t.Fatalf("second join: %s", err)
	}
	if id := peerIDLine(first); id == "" || id != peerIDLine(second) {
		t.Fatalf("the peer id changed between runs: %q then %q", peerIDLine(first), peerIDLine(second))
	}
}

func peerIDLine(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if i := strings.Index(line, "grapepeer -id "); i >= 0 {
			return strings.TrimSpace(line[i:])
		}
	}
	return ""
}

func parseEnvFile(t *testing.T, body string) (user, password string) {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		switch key {
		case "GRAPE_REST_API_USERNAME":
			user = value
		case "GRAPE_REST_API_PASSWORD":
			password = value
		}
	}
	return user, password
}

func TestStatusPrefersApiCredentialsFromTheEnvironment(t *testing.T) {
	// A supervisor puts them in the environment; the file is the fallback for
	// somebody typing the command by hand.
	dir := t.TempDir()
	if _, err := join(t, dir); err != nil {
		t.Fatalf("join: %s", err)
	}
	t.Setenv("GRAPE_REST_API_USERNAME", "from-env")
	t.Setenv("GRAPE_REST_API_PASSWORD", "also-from-env")

	user, password := apiCredentials(credsPath(dir))
	if user != "from-env" || password != "also-from-env" {
		t.Fatalf("apiCredentials = %q/%q, want the environment's values", user, password)
	}
}

func TestStatusFallsBackToTheCredentialsFileJoinWrote(t *testing.T) {
	dir := t.TempDir()
	if _, err := join(t, dir); err != nil {
		t.Fatalf("join: %s", err)
	}
	t.Setenv("GRAPE_REST_API_USERNAME", "")
	t.Setenv("GRAPE_REST_API_PASSWORD", "")

	raw, err := os.ReadFile(credsPath(dir))
	if err != nil {
		t.Fatalf("reading the credentials: %s", err)
	}
	wantUser, wantPassword := parseEnvFile(t, string(raw))

	user, password := apiCredentials(credsPath(dir))
	if user != wantUser || password != wantPassword {
		t.Fatalf("apiCredentials = %q/%q, want the file's values", user, password)
	}
	if user == "" || password == "" {
		t.Fatal("apiCredentials found nothing in the file join had just written")
	}
}

func TestStatusReportsNoCredentialsRatherThanInventingSome(t *testing.T) {
	t.Setenv("GRAPE_REST_API_USERNAME", "")
	t.Setenv("GRAPE_REST_API_PASSWORD", "")
	user, password := apiCredentials(filepath.Join(t.TempDir(), "absent.env"))
	if user != "" || password != "" {
		t.Fatalf("apiCredentials = %q/%q with no source, want empty", user, password)
	}
}
