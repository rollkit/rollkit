package block

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	coreda "github.com/rollkit/rollkit/core/da"
)

// HeaderSubmissionLoop is responsible for submitting headers to the DA layer.
func (m *Manager) HeaderSubmissionLoop(ctx context.Context) {
	timer := time.NewTicker(m.config.DA.BlockTime.Duration)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if m.pendingHeaders.isEmpty() {
			continue
		}
		err := m.submitHeadersToDA(ctx)
		if err != nil {
			m.logger.Error("error while submitting header to DA", "error", err)
		}
	}
}

func (m *Manager) submitHeadersToDA(ctx context.Context) error {
	submittedAllHeaders := false
	var backoff time.Duration
	headersToSubmit, err := m.pendingHeaders.getPendingHeaders(ctx)
	if len(headersToSubmit) == 0 {
		// There are no pending headers; return because there's nothing to do, but:
		// - it might be caused by error, then err != nil
		// - all pending headers are processed, then err == nil
		// whatever the reason, error information is propagated correctly to the caller
		return err
	}
	if err != nil {
		// There are some pending headers but also an error. It's very unlikely case - probably some error while reading
		// headers from the store.
		// The error is logged and normal processing of pending headers continues.
		m.logger.Error("error while fetching headers pending DA", "err", err)
	}
	numSubmittedHeaders := 0
	attempt := 0
	maxBlobSize, err := m.dalc.MaxBlobSize(ctx)
	if err != nil {
		return err
	}
	initialMaxBlobSize := maxBlobSize
	gasPrice := m.gasPrice
	initialGasPrice := gasPrice

daSubmitRetryLoop:
	for !submittedAllHeaders && attempt < maxSubmitAttempts {
		select {
		case <-ctx.Done():
			break daSubmitRetryLoop
		case <-time.After(backoff):
		}

		headersBz := make([][]byte, len(headersToSubmit))
		for i, header := range headersToSubmit {
			headerPb, err := header.ToProto()
			if err != nil {
				// do we drop the header from attempting to be submitted?
				return fmt.Errorf("failed to transform header to proto: %w", err)
			}
			headersBz[i], err = proto.Marshal(headerPb)
			if err != nil {
				// do we drop the header from attempting to be submitted?
				return fmt.Errorf("failed to marshal header: %w", err)
			}
		}

		ctx, cancel := context.WithTimeout(ctx, 60*time.Second) //TODO: make this configurable
		res := m.dalc.Submit(ctx, headersBz, maxBlobSize, gasPrice)
		cancel()

		switch res.Code {
		case coreda.StatusSuccess:
			m.logger.Info("successfully submitted Rollkit headers to DA layer", "gasPrice", gasPrice, "daHeight", res.Height, "headerCount", res.SubmittedCount)
			if res.SubmittedCount == uint64(len(headersToSubmit)) {
				submittedAllHeaders = true
			}
			submittedHeaders, notSubmittedHeaders := headersToSubmit[:res.SubmittedCount], headersToSubmit[res.SubmittedCount:]
			numSubmittedHeaders += len(submittedHeaders)
			for _, header := range submittedHeaders {
				m.headerCache.SetDAIncluded(header.Hash().String())
				if err != nil {
					return err
				}
			}
			lastSubmittedHeight := uint64(0)
			if l := len(submittedHeaders); l > 0 {
				lastSubmittedHeight = submittedHeaders[l-1].Height()
			}
			m.pendingHeaders.setLastSubmittedHeight(ctx, lastSubmittedHeight)
			headersToSubmit = notSubmittedHeaders
			// reset submission options when successful
			// scale back gasPrice gradually
			backoff = 0
			maxBlobSize = initialMaxBlobSize
			if m.gasMultiplier > 0 && gasPrice != -1 {
				gasPrice = gasPrice / m.gasMultiplier
				gasPrice = max(gasPrice, initialGasPrice)
			}
			m.logger.Debug("resetting DA layer submission options", "backoff", backoff, "gasPrice", gasPrice, "maxBlobSize", maxBlobSize)
		case coreda.StatusNotIncludedInBlock, coreda.StatusAlreadyInMempool:
			m.logger.Error("DA layer submission failed", "error", res.Message, "attempt", attempt)
			backoff = m.config.DA.BlockTime.Duration * time.Duration(m.config.DA.MempoolTTL) //nolint:gosec
			if m.gasMultiplier > 0 && gasPrice != -1 {
				gasPrice = gasPrice * m.gasMultiplier
			}
			m.logger.Info("retrying DA layer submission with", "backoff", backoff, "gasPrice", gasPrice, "maxBlobSize", maxBlobSize)

		case coreda.StatusTooBig:
			maxBlobSize = maxBlobSize / 4
			fallthrough
		default:
			m.logger.Error("DA layer submission failed", "error", res.Message, "attempt", attempt)
			backoff = m.exponentialBackoff(backoff)
		}

		attempt += 1
	}

	if !submittedAllHeaders {
		return fmt.Errorf(
			"failed to submit all headers to DA layer, submitted %d headers (%d left) after %d attempts",
			numSubmittedHeaders,
			len(headersToSubmit),
			attempt,
		)
	}
	return nil
}
