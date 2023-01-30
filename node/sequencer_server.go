package node

import (
	//"context"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/rpc/v2/json2"

	//"time"

	//"github.com/rs/cors"
	"github.com/rs/cors"
	"github.com/tendermint/tendermint/config"
	tmjson "github.com/tendermint/tendermint/libs/json"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/types"
	"golang.org/x/net/netutil"
	//"github.com/tendermint/tendermint/libs/service"
	/*rpcclient "github.com/tendermint/tendermint/rpc/client"
	"golang.org/x/net/netutil"*///"github.com/rollkit/rollkit/rpc/json"
)

// Nodes of centralized roll-ups may connect directly to the sequencer
// p2p gossip is unnecessary and inefficient if you always know the sequencer
// this server will expose an RPC endpoint for receiving transactions directly
type SequencerServer struct {
	server http.Server
	node   *FullNode
	Logger log.Logger
	config *config.RPCConfig
}

func NewSequencerServer(node *FullNode, conf *config.RPCConfig, logger log.Logger) *SequencerServer {
	srv := &SequencerServer{
		node:   node,
		config: conf,
		Logger: logger,
	}
	return srv
}

func (s *SequencerServer) Start() error {
	if s.node.conf.SequencerListenAddress == "" {
		s.Logger.Info("SequencerListenAddress not specified -  sequencer server will not be started")
		return nil
	}
	parts := strings.SplitN(s.node.conf.SequencerListenAddress, "://", 2)
	if len(parts) != 2 {
		return errors.New("invalid RPC listen address: expecting tcp://host:port")
	}
	proto := parts[0]
	addr := parts[1]

	listener, err := net.Listen(proto, addr)
	if err != nil {
		return err
	}

	if s.node.conf.SequencerMaxOpenConnections != 0 {
		s.Logger.Debug("limiting number of connections", "limit", s.node.conf.SequencerMaxOpenConnections)
		listener = netutil.LimitListener(listener, int(s.node.conf.SequencerMaxOpenConnections))
	}

	handler, err := newHandler(s)
	if err != nil {
		return err
	}

	if s.config.IsCorsEnabled() {
		s.Logger.Debug("Sequencer CORS enabled",
			"origins", s.config.CORSAllowedOrigins,
			"methods", s.config.CORSAllowedMethods,
			"headers", s.config.CORSAllowedHeaders,
		)
		c := cors.New(cors.Options{
			AllowedOrigins: s.config.CORSAllowedOrigins,
			AllowedMethods: s.config.CORSAllowedMethods,
			AllowedHeaders: s.config.CORSAllowedHeaders,
		})
		handler = c.Handler(handler)
	}

	go func() {
		err := s.serve(listener, handler)
		if err != http.ErrServerClosed {
			s.Logger.Error("error while serving HTTP", "error", err)
		}
	}()

	return nil
}

func (s *SequencerServer) serve(listener net.Listener, handler http.Handler) error {
	s.Logger.Info("serving HTTP", "listen address", listener.Addr())
	s.server = http.Server{Handler: handler}
	if s.config.TLSCertFile != "" && s.config.TLSKeyFile != "" {
		return s.server.ServeTLS(listener, s.config.CertFile(), s.config.KeyFile())
	}
	return s.server.Serve(listener)
}

type handler struct {
	mux *http.ServeMux
}

func newHandler(s *SequencerServer) (http.Handler, error) {
	mux := http.NewServeMux()
	h := handler{
		mux: mux,
	}
	mux.HandleFunc("/tx", s.handleDirectTx)
	return h, nil
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (s *SequencerServer) handleDirectTx(w http.ResponseWriter, r *http.Request) {
	s.Logger.Info("HELLO")
	/*values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		s.encodeAndWriteResponse(w, nil, err, int(json2.E_PARSE))
		return
	}*/
	r.ParseForm()
	values := r.PostForm
	s.Logger.Info("Parsed")
	s.Logger.Info(r.RequestURI)
	body := r.Body
	bodyData := make([]byte, 1024)
	body.Read(bodyData)
	s.Logger.Info(string(bodyData))
	marsh, err := json.Marshal(values)
	if err != nil {
		return
	}
	s.Logger.Info(string(marsh))
	if len(values["tx"]) == 0 {
		s.Logger.Info("Empty")
		s.encodeAndWriteResponse(w, "RESPONSE:)", nil, 200)
	} else {
		s.Logger.Info("Got tx. Processing...")
		tx := []byte(values["tx"][0])
		s.node.ReceiveDirectTx(tx)
		ctx, cancel := context.WithCancel(context.TODO())
		go func() {
			time.Sleep(2 * time.Second)
			cancel()
		}()
		for {
			select {
			case <-s.node.DoneBuildingBlock:
				s.Logger.Info("server -done building block")
				s.Logger.Info("Trying to respond")
				s.encodeAndWriteResponse(w, fmt.Sprintf("includd in block %d", s.node.Store.Height()), nil, 200)
				return
			case <-ctx.Done():
				s.Logger.Info("Handler timed out waiting for block to be build")
				s.Logger.Info("Trying to respond")
				s.encodeAndWriteResponse(w, "TIMED OUT :(", nil, 200)
				return
			}
		}
	}
}

func (s *SequencerServer) encodeAndWriteResponse(w http.ResponseWriter, result interface{}, errResult error, statusCode int) {
	w.Header().Set("x-content-type-options", "nosniff")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	resp := response{
		Version: "2.0",
		ID:      []byte("-1"),
	}

	if errResult != nil {
		resp.Error = &json2.Error{Code: json2.ErrorCode(statusCode), Data: errResult.Error()}
	} else {
		bytes, err := tmjson.Marshal(result)
		if err != nil {
			resp.Error = &json2.Error{Code: json2.ErrorCode(json2.E_INTERNAL), Data: err.Error()}
		} else {
			resp.Result = bytes
		}
	}

	encoder := json.NewEncoder(w)
	err := encoder.Encode(resp)
	if err != nil {
		s.Logger.Error("failed to encode RPC response", "error", err)
	}
}

type response struct {
	Version string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *json2.Error    `json:"error,omitempty"`
	ID      json.RawMessage `json:"id"`
}
type receiveTxDirectArgs struct {
	Tx types.Tx `json:"tx"`
}
