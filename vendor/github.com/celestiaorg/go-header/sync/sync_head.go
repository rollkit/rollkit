package sync

import (
	"context"
	"errors"
	"time"

	"github.com/celestiaorg/go-header"
)

// Head returns the Network Head.
//
// Known subjective head is considered network head if it is recent enough(now-timestamp<=blocktime)
// Otherwise, we attempt to request recent network head from a trusted peer and
// set as the new subjective head, assuming that trusted peer is always fully synced.
//
// The request is limited with 2 seconds and otherwise potentially unrecent header is returned.
func (s *Syncer[H]) Head(ctx context.Context, _ ...header.HeadOption[H]) (H, error) {
	sbjHead, err := s.subjectiveHead(ctx)
	if err != nil {
		return sbjHead, err
	}
	// if subjective header is recent enough (relative to the network's block time) - just use it
	if isRecent(sbjHead, s.Params.blockTime, s.Params.recencyThreshold) {
		return sbjHead, nil
	}
	// otherwise, request head from the network
	// TODO: Besides requesting we should listen for new gossiped headers and cancel request if so
	//
	// single-flight protection
	// ensure only one Head is requested at the time
	if !s.getter.Lock() {
		// means that other routine held the lock and set the subjective head for us,
		// so just recursively get it
		return s.Head(ctx)
	}
	defer s.getter.Unlock()
	// limit time to get a recent header
	// if we can't get it - give what we have
	reqCtx, cancel := context.WithTimeout(ctx, time.Second*2) // TODO(@vgonkivs): make timeout configurable
	defer cancel()
	s.metrics.unrecentHead(s.ctx)
	netHead, err := s.getter.Head(reqCtx, header.WithTrustedHead[H](sbjHead))
	if err != nil {
		log.Warnw("failed to get recent head, returning current subjective", "sbjHead", sbjHead.Height(), "err", err)
		return s.subjectiveHead(ctx)
	}
	// process and validate netHead fetched from trusted peers
	// NOTE: We could trust the netHead like we do during 'automatic subjective initialization'
	// but in this case our subjective head is not expired, so we should verify netHead
	// and only if it is valid, set it as new head
	_ = s.incomingNetworkHead(ctx, netHead)
	// netHead was either accepted or rejected as the new subjective
	// anyway return most current known subjective head
	return s.subjectiveHead(ctx)
}

// subjectiveHead returns the latest known local header that is not expired(within trusting period).
// If the header is expired, it is retrieved from a trusted peer without validation;
// in other words, an automatic subjective initialization is performed.
func (s *Syncer[H]) subjectiveHead(ctx context.Context) (H, error) {
	// pending head is the latest known subjective head and sync target, so try to get it
	// NOTES:
	// * Empty when no sync is in progress
	// * Pending cannot be expired, guaranteed
	pendHead := s.pending.Head()
	if !pendHead.IsZero() {
		return pendHead, nil
	}
	// if pending is empty - get the latest stored/synced head
	storeHead, err := s.store.Head(ctx)
	if err != nil {
		return storeHead, err
	}
	// check if the stored header is not expired and use it
	if !isExpired(storeHead, s.Params.TrustingPeriod) {
		return storeHead, nil
	}
	// otherwise, request head from a trusted peer
	log.Infow("stored head header expired", "height", storeHead.Height())
	// single-flight protection
	// ensure only one Head is requested at the time
	if !s.getter.Lock() {
		// means that other routine held the lock and set the subjective head for us,
		// so just recursively get it
		return s.subjectiveHead(ctx)
	}
	defer s.getter.Unlock()

	trustHead, err := s.getter.Head(ctx)
	if err != nil {
		return trustHead, err
	}
	s.metrics.subjectiveInitialization(s.ctx)
	// and set it as the new subjective head without validation,
	// or, in other words, do 'automatic subjective initialization'
	// NOTE: we avoid validation as the head expired to prevent possibility of the Long-Range Attack
	s.setSubjectiveHead(ctx, trustHead)
	switch {
	default:
		log.Infow("subjective initialization finished", "height", trustHead.Height())
		return trustHead, nil
	case isExpired(trustHead, s.Params.TrustingPeriod):
		log.Warnw("subjective initialization with an expired header", "height", trustHead.Height())
	case !isRecent(trustHead, s.Params.blockTime, s.Params.recencyThreshold):
		log.Warnw("subjective initialization with an old header", "height", trustHead.Height())
	}
	log.Warn("trusted peer is out of sync")
	s.metrics.trustedPeersOutOufSync(s.ctx)
	return trustHead, nil
}

// setSubjectiveHead takes already validated head and sets it as the new sync target.
func (s *Syncer[H]) setSubjectiveHead(ctx context.Context, netHead H) {
	// TODO(@Wondertan): Right now, we can only store adjacent headers, instead we should:
	//  * Allow storing any valid header here in Store
	//  * Remove ErrNonAdjacent
	//  * Remove writeHead from the canonical store implementation
	err := s.storeHeaders(ctx, netHead)
	var nonAdj *header.ErrNonAdjacent
	if err != nil && !errors.As(err, &nonAdj) {
		// might be a storage error or something else, but we can still try to continue processing netHead
		log.Errorw("storing new network header",
			"height", netHead.Height(),
			"hash", netHead.Hash().String(),
			"err", err)
	}
	s.metrics.newSubjectiveHead(s.ctx, netHead.Height(), netHead.Time())

	storeHead, err := s.store.Head(ctx)
	if err == nil && storeHead.Height() >= netHead.Height() {
		// we already synced it up - do nothing
		return
	}
	// and if valid, set it as new subjective head
	s.pending.Add(netHead)
	s.wantSync()
	log.Infow("new network head", "height", netHead.Height(), "hash", netHead.Hash())
}

// incomingNetworkHead processes new potential network headers.
// If the header valid, sets as new subjective header.
func (s *Syncer[H]) incomingNetworkHead(ctx context.Context, head H) error {
	// ensure there is no racing between network head candidates
	s.incomingMu.Lock()
	defer s.incomingMu.Unlock()

	softFailure, err := s.verify(ctx, head)
	if err != nil && !softFailure {
		return err
	}

	// TODO(@Wondertan):
	//  Implement setSyncTarget and use it for soft failures
	s.setSubjectiveHead(ctx, head)
	return err
}

// verify verifies given network head candidate.
func (s *Syncer[H]) verify(ctx context.Context, newHead H) (bool, error) {
	sbjHead, err := s.subjectiveHead(ctx)
	if err != nil {
		log.Errorw("getting subjective head during validation", "err", err)
		// local error, so soft
		return true, &header.VerifyError{Reason: err, SoftFailure: true}
	}

	var heightThreshold uint64
	if s.Params.TrustingPeriod != 0 && s.Params.blockTime != 0 {
		buffer := time.Hour * 48 / s.Params.blockTime // generous buffer to account for variable block time
		heightThreshold = uint64(s.Params.TrustingPeriod/s.Params.blockTime + buffer)
	}

	err = header.Verify(sbjHead, newHead, heightThreshold)
	if err == nil {
		return false, nil
	}

	var verErr *header.VerifyError
	if errors.As(err, &verErr) && !verErr.SoftFailure {
		logF := log.Warnw
		if errors.Is(err, header.ErrKnownHeader) {
			logF = log.Debugw
		}
		logF("invalid network header",
			"height_of_invalid", newHead.Height(),
			"hash_of_invalid", newHead.Hash(),
			"height_of_subjective", sbjHead.Height(),
			"hash_of_subjective", sbjHead.Hash(),
			"reason", verErr.Reason)
	}

	return verErr.SoftFailure, err
}

// isExpired checks if header is expired against trusting period.
func isExpired[H header.Header[H]](header H, period time.Duration) bool {
	expirationTime := header.Time().Add(period)
	return !expirationTime.After(time.Now())
}

// isRecent checks if header is recent against the given recency threshold.
func isRecent[H header.Header[H]](header H, blockTime, recencyThreshold time.Duration) bool {
	if recencyThreshold == 0 {
		recencyThreshold = blockTime * 2 // allow some drift by adding additional buffer of 2 blocks
	}
	return time.Since(header.Time()) <= recencyThreshold
}
