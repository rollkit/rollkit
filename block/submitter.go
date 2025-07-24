package block

import (
	"context"
	"fmt"
	"time"

	coreda "github.com/rollkit/rollkit/core/da"
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
			m.logger.Info("header submission loop stopped")
			return
		case <-timer.C:
		}
		if m.pendingHeaders.isEmpty() {
			continue
		}
		headersToSubmit, err := m.pendingHeaders.getPendingHeaders(ctx)
		if err != nil {
			m.logger.Error("error while fetching headers pending DA", "err", err)
			continue
		}
		if len(headersToSubmit) == 0 {
			continue
		}
		err = m.submitHeadersToDA(ctx, headersToSubmit)
		if err != nil {
			m.logger.Error("error while submitting header to DA", "error", err)
		}
	}
}

// DataSubmissionLoop is responsible for submitting data to the DA layer.
func (m *Manager) DataSubmissionLoop(ctx context.Context) {
	timer := time.NewTicker(m.config.DA.BlockTime.Duration)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			m.logger.Info("data submission loop stopped")
			return
		case <-timer.C:
		}
		if m.pendingData.isEmpty() {
			continue
		}

		signedDataToSubmit, err := m.createSignedDataToSubmit(ctx)
		if err != nil {
			m.logger.Error("failed to create signed data to submit", "error", err)
			continue
		}
		if len(signedDataToSubmit) == 0 {
			continue
		}

		err = m.submitDataToDA(ctx, signedDataToSubmit)
		if err != nil {
			m.logger.Error("failed to submit data to DA", "error", err)
		}
	}
}

// submitToDA is a helper for submitting marshaled items to the DA layer with retry, backoff, and gas price logic.
// postSubmit is called after a successful submission to update caches, pending lists, etc.
func submitToDA(
	m *Manager,
	ctx context.Context,
	marshaled [][]byte,
	onSuccessfulSubmission func(int, *coreda.ResultSubmit, float64),
	itemType string,
) error {
	submittedAll := false
	var backoff time.Duration
	attempt := 0
	initialGasPrice := m.gasPrice
	gasPrice := initialGasPrice
	remaining := marshaled
	numSubmitted := 0
	var remainingLen int
	for !submittedAll && attempt < maxSubmitAttempts {
		select {
		case <-ctx.Done():
			m.logger.Info("context done, stopping submission loop")
			return nil
		case <-time.After(backoff):
		}

		remainingLen = len(remaining)

		submitctx, submitCtxCancel := context.WithTimeout(ctx, 60*time.Second)

		// Record DA submission retry attempt
		m.recordDAMetrics("submission", DAModeRetry)

		res := types.SubmitWithHelpers(submitctx, m.da, m.logger, remaining, gasPrice, nil)
		submitCtxCancel()

		switch res.Code {
		case coreda.StatusSuccess:
			// Record successful DA submission
			m.recordDAMetrics("submission", DAModeSuccess)

			m.logger.Info(fmt.Sprintf("successfully submitted %s to DA layer", itemType), "gasPrice", gasPrice, "count", res.SubmittedCount)
			if res.SubmittedCount == uint64(remainingLen) {
				submittedAll = true
			}

			// submittedCount represents both:
			// 1. The number of items successfully submitted in this batch
			// 2. The index range [submissionOffset : submissionOffset+submittedCount) of submitted items
			submittedCount := int(res.SubmittedCount)
			numSubmitted += submittedCount
			onSuccessfulSubmission(submittedCount, &res, gasPrice)

			remaining = remaining[res.SubmittedCount:]
			backoff = 0
			if m.gasMultiplier > 0 && gasPrice != -1 {
				gasPrice = gasPrice / m.gasMultiplier
				gasPrice = max(gasPrice, initialGasPrice)
			}
			m.logger.Debug("resetting DA layer submission options", "backoff", backoff, "gasPrice", gasPrice)
		case coreda.StatusNotIncludedInBlock, coreda.StatusAlreadyInMempool:
			m.logger.Error("DA layer submission failed", "error", res.Message, "attempt", attempt)
			// Record failed DA submission (will retry)
			m.recordDAMetrics("submission", DAModeFail)
			backoff = m.config.DA.BlockTime.Duration * time.Duration(m.config.DA.MempoolTTL)
			if m.gasMultiplier > 0 && gasPrice != -1 {
				gasPrice = gasPrice * m.gasMultiplier
			}
			m.logger.Info("retrying DA layer submission with", "backoff", backoff, "gasPrice", gasPrice)
		case coreda.StatusContextCanceled:
			m.logger.Info("DA layer submission canceled due to context cancellation", "attempt", attempt)
			return nil
		case coreda.StatusTooBig:
			fallthrough
		default:
			m.logger.Error("DA layer submission failed", "error", res.Message, "attempt", attempt)
			// Record failed DA submission (will retry)
			m.recordDAMetrics("submission", DAModeFail)
			backoff = m.exponentialBackoff(backoff)
		}
		attempt++
	}
	if !submittedAll {
		// Record final failure after all retries are exhausted
		m.recordDAMetrics("submission", DAModeFail)
		// If not all items are submitted, the remaining items will be retried in the next submission loop.
		return fmt.Errorf("failed to submit all %s(s) to DA layer, submitted %d items (%d left) after %d attempts", itemType, numSubmitted, remainingLen, attempt)
	}
	return nil
}

// submitHeadersToDA submits a list of headers to the DA layer using the generic submitToDA helper.
func (m *Manager) submitHeadersToDA(ctx context.Context, headersToSubmit []*types.SignedHeader) error {
	if len(headersToSubmit) == 0 {
		return nil
	}
	marshaledHeaders := make([][]byte, len(headersToSubmit))
	for i, header := range headersToSubmit {
		headerPb, err := header.ToProto()
		if err != nil {
			return fmt.Errorf("failed to transform header to proto: %w", err)
		}
		marshaledHeaders[i], err = proto.Marshal(headerPb)
		if err != nil {
			return fmt.Errorf("failed to marshal header to proto: %w", err)
		}
	}

	// Track submission offset to maintain correspondence with original headers
	submissionOffset := 0

	return submitToDA(m, ctx, marshaledHeaders,
		func(submittedCount int, res *coreda.ResultSubmit, currentGasPrice float64) {
			for i := 0; i < submittedCount; i++ {
				headerIdx := submissionOffset + i
				if headerIdx < len(headersToSubmit) {
					m.headerCache.SetDAIncluded(headersToSubmit[headerIdx].Hash().String(), res.Height)
				}
			}

			// Update submission offset for next potential retry
			submissionOffset += submittedCount

			lastSubmittedHeaderHeight := uint64(0)
			if submissionOffset > 0 && submissionOffset <= len(headersToSubmit) {
				lastSubmittedHeaderHeight = headersToSubmit[submissionOffset-1].Height()
			}
			m.pendingHeaders.setLastSubmittedHeaderHeight(ctx, lastSubmittedHeaderHeight)
			// Update sequencer metrics if the sequencer supports it
			if seq, ok := m.sequencer.(MetricsRecorder); ok {
				seq.RecordMetrics(currentGasPrice, res.BlobSize, res.Code, m.pendingHeaders.numPendingHeaders(), lastSubmittedHeaderHeight)
			}
			m.sendNonBlockingSignalToDAIncluderCh()
		},
		"header",
	)
}

// submitDataToDA submits a list of signed data to the DA layer using the generic submitToDA helper.
func (m *Manager) submitDataToDA(ctx context.Context, signedDataToSubmit []*types.SignedData) error {
	if len(signedDataToSubmit) == 0 {
		return nil
	}
	marshaledSignedDataToSubmit := make([][]byte, len(signedDataToSubmit))
	for i, signedData := range signedDataToSubmit {
		marshaled, err := signedData.MarshalBinary()
		if err != nil {
			return fmt.Errorf("failed to marshal signed data: %w", err)
		}
		marshaledSignedDataToSubmit[i] = marshaled
	}

	// Track submission offset to maintain correspondence with original data
	submissionOffset := 0

	return submitToDA(m, ctx, marshaledSignedDataToSubmit,
		func(submittedCount int, res *coreda.ResultSubmit, currentGasPrice float64) {
			for i := 0; i < submittedCount; i++ {
				dataIdx := submissionOffset + i
				if dataIdx < len(signedDataToSubmit) {
					m.dataCache.SetDAIncluded(signedDataToSubmit[dataIdx].Data.DACommitment().String(), res.Height)
				}
			}

			// Update submission offset for next potential retry
			submissionOffset += submittedCount

			lastSubmittedDataHeight := uint64(0)
			if submissionOffset > 0 && submissionOffset <= len(signedDataToSubmit) {
				lastSubmittedDataHeight = signedDataToSubmit[submissionOffset-1].Height()
			}
			m.pendingData.setLastSubmittedDataHeight(ctx, lastSubmittedDataHeight)
			// Update sequencer metrics if the sequencer supports it
			if seq, ok := m.sequencer.(MetricsRecorder); ok {
				seq.RecordMetrics(currentGasPrice, res.BlobSize, res.Code, m.pendingData.numPendingData(), lastSubmittedDataHeight)
			}
			m.sendNonBlockingSignalToDAIncluderCh()
		},
		"data",
	)
}

// createSignedDataToSubmit converts the list of pending data to a list of SignedData.
func (m *Manager) createSignedDataToSubmit(ctx context.Context) ([]*types.SignedData, error) {
	dataList, err := m.pendingData.getPendingData(ctx)
	if err != nil {
		return nil, err
	}

	if m.signer == nil {
		return nil, fmt.Errorf("signer is nil; cannot sign data")
	}

	pubKey, err := m.signer.GetPublic()
	if err != nil {
		return nil, fmt.Errorf("failed to get public key: %w", err)
	}

	signer := types.Signer{
		PubKey:  pubKey,
		Address: m.genesis.ProposerAddress,
	}

	signedDataToSubmit := make([]*types.SignedData, 0, len(dataList))

	for _, data := range dataList {
		if len(data.Txs) == 0 {
			continue
		}
		signature, err := m.getDataSignature(data)
		if err != nil {
			return nil, fmt.Errorf("failed to get data signature: %w", err)
		}
		signedDataToSubmit = append(signedDataToSubmit, &types.SignedData{
			Data:      *data,
			Signature: signature,
			Signer:    signer,
		})
	}

	return signedDataToSubmit, nil
}
