package p2p

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/net/conngater"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/celestiaorg/go-header"
	p2p_pb "github.com/celestiaorg/go-header/p2p/pb"
)

var (
	log = logging.Logger("header/p2p")

	tracerClient = otel.Tracer("header/p2p-client")
)

// minHeadResponses is the minimum number of headers of the same height
// received from peers to determine the network head. If all trusted peers
// will return headers with non-equal height, then the highest header will be
// chosen.
const minHeadResponses = 2

// maxUntrustedHeadRequests is the number of head requests to be made to
// the network in order to determine the network head.
var maxUntrustedHeadRequests = 4

// Exchange enables sending outbound HeaderRequests to the network as well as
// handling inbound HeaderRequests from the network.
type Exchange[H header.Header[H]] struct {
	ctx    context.Context
	cancel context.CancelFunc

	protocolID protocol.ID
	host       host.Host

	trustedPeers func() peer.IDSlice
	peerTracker  *peerTracker
	metrics      *exchangeMetrics

	Params ClientParameters
}

func NewExchange[H header.Header[H]](
	host host.Host,
	peers peer.IDSlice,
	gater *conngater.BasicConnectionGater,
	opts ...Option[ClientParameters],
) (*Exchange[H], error) {
	params := DefaultClientParameters()
	for _, opt := range opts {
		opt(&params)
	}

	err := params.Validate()
	if err != nil {
		return nil, err
	}

	var metrics *exchangeMetrics
	if params.metrics {
		var err error
		metrics, err = newExchangeMetrics()
		if err != nil {
			return nil, err
		}
	}

	ex := &Exchange[H]{
		host:        host,
		protocolID:  protocolID(params.networkID),
		peerTracker: newPeerTracker(host, gater, params.pidstore, metrics),
		Params:      params,
		metrics:     metrics,
	}

	ex.trustedPeers = func() peer.IDSlice {
		return shufflePeers(peers)
	}
	return ex, nil
}

func (ex *Exchange[H]) Start(ctx context.Context) error {
	ex.ctx, ex.cancel = context.WithCancel(context.Background())
	log.Infow("client: starting client", "protocol ID", ex.protocolID)

	go ex.peerTracker.gc()
	go ex.peerTracker.track()

	// bootstrap the peerTracker with trusted peers as well as previously seen
	// peers if provided.
	return ex.peerTracker.bootstrap(ctx, ex.trustedPeers())
}

func (ex *Exchange[H]) Stop(ctx context.Context) error {
	// cancel the session if it exists
	ex.cancel()
	// stop the peerTracker
	err := ex.peerTracker.stop(ctx)
	return errors.Join(err, ex.metrics.Close())
}

// Head requests the latest Header from trusted peers.
//
// The Head must be verified thereafter where possible.
// We request in parallel all the trusted peers, compare their response
// and return the highest one.
func (ex *Exchange[H]) Head(ctx context.Context, opts ...header.HeadOption[H]) (H, error) {
	log.Debug("requesting head")
	ctx, span := tracerClient.Start(ctx, "head")
	defer span.End()

	reqCtx := ctx
	startTime := time.Now()
	if deadline, ok := ctx.Deadline(); ok {
		// allocate 90% of caller's set deadline for requests
		// and give leftover to determine the bestHead from gathered responses
		// this avoids DeadlineExceeded error when any of the peers are unresponsive

		sub := deadline.Sub(startTime) * 9 / 10
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithDeadline(ctx, startTime.Add(sub))
		defer cancel()
	}

	reqParams := header.HeadParams[H]{}
	for _, opt := range opts {
		opt(&reqParams)
	}

	peers := ex.trustedPeers()

	// the TrustedHead field indicates whether the Exchange should use
	// trusted peers for its Head request. If nil, trusted peers will
	// be used. If non-nil, Exchange will ask several peers from its network for
	// their Head and verify against the given trusted header.
	useTrackedPeers := !reqParams.TrustedHead.IsZero()
	if useTrackedPeers {
		trackedPeers := ex.peerTracker.getPeers(maxUntrustedHeadRequests)
		if len(trackedPeers) > 0 {
			peers = trackedPeers
			log.Debugw("requesting head from tracked peers", "amount", len(peers))
		}
	}

	var (
		zero      H
		headerReq = &p2p_pb.HeaderRequest{
			Data:   &p2p_pb.HeaderRequest_Origin{Origin: uint64(0)},
			Amount: 1,
		}
		headerRespCh = make(chan H, len(peers))
	)
	for _, from := range peers {
		go func(from peer.ID) {
			_, newSpan := tracerClient.Start(
				ctx, "requesting peer",
				trace.WithAttributes(attribute.String("peerID", from.String())),
			)
			defer newSpan.End()
			
			headers, err := ex.request(reqCtx, from, headerReq)
			if err != nil {
				newSpan.SetStatus(codes.Error, err.Error())
				log.Errorw("head request to peer failed", "peer", from, "err", err)
				headerRespCh <- zero
				return
			}
			// if tracked (untrusted) peers were requested, verify head
			if useTrackedPeers {
				err = header.Verify[H](reqParams.TrustedHead, headers[0], header.DefaultHeightThreshold)
				if err != nil {
					var verErr *header.VerifyError
					if errors.As(err, &verErr) && verErr.SoftFailure {
						log.Debugw("received head from tracked peer that soft-failed verification",
							"tracked peer", from, "err", err)
						newSpan.SetStatus(codes.Error, err.Error())
						headerRespCh <- headers[0]
						return
					}
					logF := log.Warnw
					if errors.Is(err, header.ErrKnownHeader) {
						logF = log.Debugw
					}
					logF("verifying head received from tracked peer", "tracked peer", from,
						"height", headers[0].Height(), "err", err)
					newSpan.SetStatus(codes.Error, err.Error())
					headerRespCh <- zero
					return
				}
			}
			newSpan.SetStatus(codes.Ok, "")
			// request ensures that the result slice will have at least one Header
			headerRespCh <- headers[0]
		}(from)
	}

	headType := headTypeTrusted
	if useTrackedPeers {
		headType = headTypeUntrusted
	}

	headers := make([]H, 0, len(peers))
	for range peers {
		select {
		case h := <-headerRespCh:
			if !h.IsZero() {
				headers = append(headers, h)
			}
		case <-ctx.Done():
			status := headStatusCanceled
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				status = headStatusTimeout
			}
			span.SetStatus(codes.Error, fmt.Sprintf("head request %s", status))
			ex.metrics.head(ctx, time.Since(startTime), len(headers), headType, status)
			return zero, ctx.Err()
		case <-ex.ctx.Done():
			ex.metrics.head(ctx, time.Since(startTime), len(headers), headType, headStatusCanceled)
			span.SetStatus(codes.Error, "exchange client stopped")
			return zero, ex.ctx.Err()
		}
	}

	head, err := bestHead[H](headers)
	if err != nil {
		ex.metrics.head(ctx, time.Since(startTime), len(headers), headType, headStatusNoHeaders)
		span.SetStatus(codes.Error, headStatusNoHeaders)
		return zero, err
	}

	ex.metrics.head(ctx, time.Since(startTime), len(headers), headType, headStatusOk)
	span.SetStatus(codes.Ok, "")
	return head, nil
}

// GetByHeight performs a request for the Header at the given
// height to the network. Note that the Header must be verified
// thereafter.
func (ex *Exchange[H]) GetByHeight(ctx context.Context, height uint64) (H, error) {
	log.Debugw("requesting header", "height", height)
	ctx, span := tracerClient.Start(ctx, "get-by-height",
		trace.WithAttributes(
			attribute.Int64("height", int64(height)),
		))
	defer span.End()
	var zero H
	// sanity check height
	if height == 0 {
		err := fmt.Errorf("specified request height must be greater than 0")
		span.SetStatus(codes.Error, err.Error())
		return zero, err
	}
	// create request
	req := &p2p_pb.HeaderRequest{
		Data:   &p2p_pb.HeaderRequest_Origin{Origin: height},
		Amount: 1,
	}
	headers, err := ex.performRequest(ctx, req)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return zero, err
	}
	span.SetStatus(codes.Ok, "")
	return headers[0], nil
}

// GetRangeByHeight performs a request for the given range of Headers to the network and
// ensures that returned headers are correct against the passed one.
func (ex *Exchange[H]) GetRangeByHeight(
	ctx context.Context,
	from H,
	to uint64,
) ([]H, error) {
	ctx, span := tracerClient.Start(ctx, "get-range-by-height",
		trace.WithAttributes(
			attribute.Int64("from", int64(from.Height())),
			attribute.Int64("to", int64(to)),
		))
	defer span.End()
	session := newSession[H](
		ex.ctx, ex.host, ex.peerTracker, ex.protocolID, ex.Params.RangeRequestTimeout, ex.metrics, withValidation(from),
	)
	defer session.close()
	// we request the next header height that we don't have: `fromHead`+1
	amount := to - (from.Height() + 1)
	result, err := session.getRangeByHeight(ctx, from.Height()+1, amount, ex.Params.MaxHeadersPerRangeRequest)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	span.SetStatus(codes.Ok, "")
	return result, nil
}

// Get performs a request for the Header by the given hash corresponding
// to the RawHeader. Note that the Header must be verified thereafter.
func (ex *Exchange[H]) Get(ctx context.Context, hash header.Hash) (H, error) {
	log.Debugw("requesting header", "hash", hash.String())
	ctx, span := tracerClient.Start(ctx, "get-by-hash",
		trace.WithAttributes(
			attribute.String("hash", hash.String()),
		))
	defer span.End()
	var zero H
	// create request
	req := &p2p_pb.HeaderRequest{
		Data:   &p2p_pb.HeaderRequest_Hash{Hash: hash},
		Amount: 1,
	}
	headers, err := ex.performRequest(ctx, req)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return zero, err
	}

	if !bytes.Equal(headers[0].Hash(), hash) {
		err = fmt.Errorf("incorrect hash in header: expected %x, got %x", hash, headers[0].Hash())
		span.SetStatus(codes.Error, err.Error())
		return zero, err
	}
	span.SetStatus(codes.Ok, "")
	return headers[0], nil
}

const requestRetry = 3

func (ex *Exchange[H]) performRequest(
	ctx context.Context,
	req *p2p_pb.HeaderRequest,
) ([]H, error) {
	if req.Amount == 0 {
		return make([]H, 0), nil
	}

	trustedPeers := ex.trustedPeers()
	if len(trustedPeers) == 0 {
		return nil, fmt.Errorf("no trusted peers")
	}

	var reqErr error

	for i := 0; i < requestRetry; i++ {
		for _, peer := range trustedPeers {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-ex.ctx.Done():
				return nil, ex.ctx.Err()
			default:
			}

			h, err := ex.request(ctx, peer, req)
			if err != nil {
				reqErr = err
				log.Debugw("requesting header from trustedPeer failed",
					"trustedPeer", peer, "err", err, "try", i)
				continue
			}
			return h, err
		}
	}
	return nil, reqErr
}

// request sends the HeaderRequest to a remote peer.
func (ex *Exchange[H]) request(
	ctx context.Context,
	to peer.ID,
	req *p2p_pb.HeaderRequest,
) ([]H, error) {
	log.Debugw("requesting peer", "peer", to)
	responses, size, duration, err := sendMessage(ctx, ex.host, to, ex.protocolID, req)
	ex.metrics.response(ctx, size, duration, err)
	if err != nil {
		log.Debugw("err sending request", "peer", to, "err", err)
		return nil, err
	}

	hdrs, err := processResponses[H](responses)
	if err != nil {
		return nil, err
	}
	for _, hdr := range hdrs {
		// TODO(@Wondertan): There should be a unified header validation code path
		err = validateChainID(ex.Params.chainID, hdr.ChainID())
		if err != nil {
			return nil, err
		}
	}
	return hdrs, nil
}

// shufflePeers changes the order of trusted peers.
func shufflePeers(peers peer.IDSlice) peer.IDSlice {
	tpeers := make(peer.IDSlice, len(peers))
	copy(tpeers, peers)
	//nolint:gosec // G404: Use of weak random number generator
	rand.New(rand.NewSource(time.Now().UnixNano())).Shuffle(
		len(tpeers),
		func(i, j int) { tpeers[i], tpeers[j] = tpeers[j], tpeers[i] },
	)
	return tpeers
}

// bestHead chooses Header that matches the conditions:
// * should have max height among received;
// * should be received at least from 2 peers;
// If neither condition is met, then latest Header will be returned (header of the highest
// height).
func bestHead[H header.Header[H]](result []H) (H, error) {
	if len(result) == 0 {
		var zero H
		return zero, header.ErrNotFound
	}
	counter := make(map[string]int)
	// go through all of Headers and count the number of headers with a specific hash
	for _, res := range result {
		counter[res.Hash().String()]++
	}
	// sort results in a decreasing order
	sort.Slice(result, func(i, j int) bool {
		return result[i].Height() > result[j].Height()
	})

	// try to find Header with the maximum height that was received at least from 2 peers
	for _, res := range result {
		if counter[res.Hash().String()] >= minHeadResponses {
			return res, nil
		}
	}
	log.Debug("could not find latest header received from at least two peers, returning header with the max height")
	// otherwise return header with the max height
	return result[0], nil
}
