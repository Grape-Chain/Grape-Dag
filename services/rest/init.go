package rest

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/services"
	"github.com/Grape-Chain/Grape-Dag/services/eth/rpc"
	"github.com/Grape-Chain/Grape-Dag/services/rest/api"
	"github.com/Grape-Chain/Grape-Dag/services/ws"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	golog "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
)

type RestAPIConfig struct {
	ctx context.Context // this context is needed to access peer discovery
	srv *http.Server
	rd  *routing.RoutingDiscovery
}

var logger golog.EventLogger = golog.Logger("node-api")

var apiConfig *RestAPIConfig

// This is a temporary solution to protect our rest api hosted in the cloud
// until we decide which api access model to use for production.
// Credentials come from GRAPE_REST_API_USERNAME / GRAPE_REST_API_PASSWORD; when
// they are unset the API refuses to serve unless peer.apiauthdisabled is set
// explicitly (see authCredentials).
var users map[string]string = map[string]string{config.REST_API_USERNAME: config.REST_API_PASSWORD}

// authCredentialsConfigured - true when a non-empty user and password are set.
// An empty pair would otherwise build the map {"": ""}, which makes
// `Authorization: Basic Og==` (a bare ":") a valid credential for every route.
func authCredentialsConfigured() bool {
	return config.REST_API_USERNAME != "" && config.REST_API_PASSWORD != ""
}

// Account service required for account querying operations from state
var accService services.AccountService

// Transaction service required for sending transactions and querying them from DAG
var txService services.TransactionService

// Call StartRestAPISrv to start the rest api server
// Pass in the context with cancel fn, routing discovery (context discovery)
func StartRestAPISrv(ctx context.Context, rd *routing.RoutingDiscovery) {

	cfg := config.GetConfig().Peer
	if !authCredentialsConfigured() {
		if !cfg.ApiAuthDisabled {
			logger.Fatalf("[rest api] Refusing to start: no API credentials configured. " +
				"Set GRAPE_REST_API_USERNAME and GRAPE_REST_API_PASSWORD, or set " +
				"peer.apiauthdisabled: true to run the API unauthenticated (local development only).")
			return
		}
		logger.Warnf("[rest api] %s API AUTHENTICATION IS DISABLED - every endpoint is open. "+
			"Do not use this configuration outside local development.", "⚠")
	}
	var tlsConfig *tls.Config
	if cfg.ApiTlsEnabled {
		logger.Infof("Trying to load TLS cert=%s and key=%s", cfg.Apicert, cfg.Apikey)
		cert, err := tls.LoadX509KeyPair(cfg.Apicert, cfg.Apikey)
		if err != nil {
			logger.Fatalf("[rest api] Failed to load x.509 certificate: %s", err.Error())
			os.Exit(-1)
		} else {
			logger.Infof("TLS loaded successfully")
			tlsConfig = &tls.Config{
				Certificates: []tls.Certificate{cert}}
		}
	}

	// Create services
	accService = services.NewAccountService()
	txService = services.NewTransactionService()
	apiConfig = &RestAPIConfig{
		ctx: ctx,
		srv: &http.Server{
			Handler:   nil,
			TLSConfig: tlsConfig,
		},
		rd: rd,
	}
	lsnr, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Apiport))
	if err != nil {
		fmt.Println("Error listening:", err)
		os.Exit(1)
	}
	apiConfig.srv.Handler = apiConfig.routes()

	// the server runs on a separate routine until we shut down the server
	go func() {
		var err error
		if cfg.ApiTlsEnabled {
			err = apiConfig.srv.ServeTLS(lsnr, cfg.Apicert, cfg.Apikey)
		} else {
			err = apiConfig.srv.Serve(lsnr)
		}
		logger.Infof("[rest api] rest api server: %s", err.Error())
	}()
	logger.Infof("[rest api] Successfully started the rest api server on %s", lsnr.Addr().String())
}

func StopRestAPISrv() {
	if apiConfig != nil {
		if apiConfig.srv != nil {
			err := apiConfig.srv.Shutdown(apiConfig.ctx)
			if err != nil {
				logger.Errorf("[rest api] Shutting down the rest api server: %s", err.Error())
			} else {
				logger.Info("[rest api] Successfully shut down the rest api server")
			}
		}
	}
}

func (app *RestAPIConfig) routes() http.Handler {
	s := NodeApiServer{config: app}
	mux := chi.NewRouter()
	mux.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "WWW-Authenticate", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	if authCredentialsConfigured() {
		mux.Use(middleware.BasicAuth("grape-1", users))
	}
	mux.Use(middleware.Heartbeat("/heartbeat"))
	mux.Use(func(h http.Handler) http.Handler { // configure general error handling on API endpoints
		return &ErrorRecovery{h}
	})
	// @Note: these share the middleware chain above (chi applies mux-level
	// middleware to every route registered on the mux, groups included).
	mux.HandleFunc("/faucet", Faucet)
	mux.HandleFunc("/events", ws.EventsEndpoint)
	mux.HandleFunc("/eth/rpc", rpc.RpcHandler())

	// the bundled testnet wallet, when its assets have been built
	if dir := walletDir(); dir != "" {
		if index, wasm := walletAssetsPresent(dir); index {
			if !wasm {
				logger.Warnf("[rest api] Wallet assets in %s are missing wallet.wasm - run 'make wallet'", dir)
			}
			mux.Handle("/wallet", http.RedirectHandler("/wallet/", http.StatusMovedPermanently))
			mux.Handle("/wallet/*", http.StripPrefix("/wallet/", walletHandler(dir)))
			logger.Infof("[rest api] Serving the web wallet from %s at /wallet/", dir)
		} else {
			logger.Infof("[rest api] No web wallet assets in %s - skipping /wallet (run 'make wallet' to build them)", dir)
		}
	}

	finalHandler := api.HandlerFromMuxWithBaseURL(s, mux, "/api/rest")
	logger.Info("[rest api] Successfully configured route handlers")
	return finalHandler
}
