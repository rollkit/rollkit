package block

import (
	"context"
	"fmt"
	"time"

	coreda "github.com/rollkit/rollkit/core/da"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/types"
	"google.golang.org/protobuf/proto"
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

	gasPrice := m.gasPrice
	initialGasPrice := gasPrice

	for !submittedAllHeaders && attempt < maxSubmitAttempts {
		select {
		case <-ctx.Done():
			m.logger.Info("context done, stopping header submission loop")
			return nil
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

		submitctx, submitCtxCancel := context.WithTimeout(ctx, 60*time.Second) // TODO: make this configurable
		res := types.SubmitWithHelpers(submitctx, m.da, m.logger, headersBz, gasPrice, nil)
		submitCtxCancel()

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
			}
			lastSubmittedHeight := uint64(0)
			if l := len(submittedHeaders); l > 0 {
				lastSubmittedHeight = submittedHeaders[l-1].Height()
			}
			m.pendingHeaders.setLastSubmittedHeight(ctx, lastSubmittedHeight)
			headersToSubmit = notSubmittedHeaders
			m.sendNonBlockingSignalToDAIncluderCh()
			// reset submission options when successful
			// scale back gasPrice gradually
			backoff = 0
			if m.gasMultiplier > 0 && gasPrice != -1 {
				gasPrice = gasPrice / m.gasMultiplier
				gasPrice = max(gasPrice, initialGasPrice)
			}
			m.logger.Debug("resetting DA layer submission options", "backoff", backoff, "gasPrice", gasPrice)
		case coreda.StatusNotIncludedInBlock, coreda.StatusAlreadyInMempool:
			m.logger.Error("DA layer submission failed", "error", res.Message, "attempt", attempt)
			backoff = m.config.DA.BlockTime.Duration * time.Duration(m.config.DA.MempoolTTL) //nolint:gosec
			if m.gasMultiplier > 0 && gasPrice != -1 {
				gasPrice = gasPrice * m.gasMultiplier
			}
			m.logger.Info("retrying DA layer submission with", "backoff", backoff, "gasPrice", gasPrice)
		case coreda.StatusContextCanceled:
			m.logger.Info("DA layer submission canceled", "attempt", attempt)
			return nil
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

// DataSubmissionLoop is responsible for submitting data to the DA layer.
func (m *Manager) DataSubmissionLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			m.logger.Info("data submission loop stopped")
			return
		case batch := <-m.batchSubmissionChan:
			signedData, err := m.createSignedDataFromBatch(&batch)
			if err != nil {
				m.logger.Error("failed to create signed data from batch", "error", err)
				continue
			}

			err = m.submitDataToDA(ctx, signedData)
			if err != nil {
				m.logger.Error("failed to submit data to DA", "error", err)
			}
		}
	}
}

// createSignedDataFromBatch converts a batch to a SignedData, including signing.
func (m *Manager) createSignedDataFromBatch(batch *coresequencer.Batch) (*types.SignedData, error) {
	data := types.Data{
		Txs: make(types.Txs, len(batch.Transactions)),
	}
	for i, tx := range batch.Transactions {
		data.Txs[i] = types.Tx(tx)
	}

	signature, err := m.getDataSignature(&data)
	if err != nil {
		return nil, err
	}

	pubKey, err := m.signer.GetPublic()
	if err != nil {
		return nil, err
	}

	signer := types.Signer{
		PubKey:  pubKey,
		Address: m.genesis.ProposerAddress,
	}

	return &types.SignedData{
		Data:      data,
		Signature: signature,
		Signer:    signer,
	}, nil
}

// submitDataToDA submits signed data to the Data Availability (DA) layer.
// It implements a retry mechanism with exponential backoff and gas price adjustments to handle various failure scenarios.
// The function attempts to submit data multiple times (up to maxSubmitAttempts).

// Different strategies are used based on the response from the DA layer:
// - On success: Reduces gas price gradually (but not below initial price)
// - On mempool issues: Increases gas price and uses a longer backoff
// - On size issues: Reduces the blob size and uses exponential backoff
// - On other errors: Uses exponential backoff
//
// It returns an error if not all transactions could be submitted after all attempts.
func (m *Manager) submitDataToDA(ctx context.Context, signedData *types.SignedData) error {
	var backoff time.Duration
	attempt := 0

	// Store initial values to be able to reset or compare later
	initialGasPrice := m.gasPrice
	gasPrice := initialGasPrice

	for attempt < maxSubmitAttempts {
		// Wait for backoff duration or exit if context is done
		select {
		case <-ctx.Done():
			m.logger.Info("context done, stopping batch submission loop")
			return nil
		case <-time.After(backoff):
		}

		signedDataBz, err := signedData.MarshalBinary()
		if err != nil {
			return fmt.Errorf("failed to marshal signed data: %w", err)
		}

		// Attempt to submit the signed data to the DA layer using the helper function
		res := types.SubmitWithHelpers(ctx, m.da, m.logger, [][]byte{signedDataBz}, gasPrice, nil)

		gasMultiplier, multErr := m.da.GasMultiplier(ctx)
		if multErr != nil {
			m.logger.Error("failed to get gas multiplier", "error", multErr)
			gasMultiplier = 0
		}

		switch res.Code {
		case coreda.StatusSuccess:
			m.logger.Info("successfully submitted data to DA layer",
				"gasPrice", gasPrice,
				"height", res.Height)

			// Reset submission parameters after success
			backoff = 0

			// Gradually reduce gas price on success, but not below initial price
			if gasMultiplier > 0 && gasPrice != 0 {
				gasPrice = gasPrice / gasMultiplier
				if gasPrice < initialGasPrice {
					gasPrice = initialGasPrice
				}
			}
			m.logger.Debug("resetting DA layer submission options", "backoff", backoff, "gasPrice", gasPrice)

			m.DataCache().SetDAIncluded(signedData.DACommitment().String())
			m.sendNonBlockingSignalToDAIncluderCh()
			return nil

		case coreda.StatusNotIncludedInBlock, coreda.StatusAlreadyInMempool:
			m.logger.Error("DA layer submission failed", "error", res.Message, "attempt", attempt)
			backoff = m.config.DA.BlockTime.Duration * time.Duration(m.config.DA.MempoolTTL)
			if gasMultiplier > 0 && gasPrice != 0 {
				gasPrice = gasPrice * gasMultiplier
			}
			m.logger.Info("retrying DA layer submission with", "backoff", backoff, "gasPrice", gasPrice)
		case coreda.StatusContextCanceled:
			m.logger.Info("DA layer submission canceled due to context cancellation", "attempt", attempt)
			return nil
		case coreda.StatusTooBig:
			// Blob size adjustment is handled within DA impl or SubmitWithOptions call
			// fallthrough to default exponential backoff
			fallthrough
		default:
			m.logger.Error("DA layer submission failed", "error", res.Message, "attempt", attempt)
			backoff = m.exponentialBackoff(backoff)
		}

		attempt++
	}

	// Return error if not all transactions were submitted after all attempts
	return fmt.Errorf("failed to submit data to DA layer after %d attempts", attempt)
}
