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

// submitToDA is a generic helper for submitting items to the DA layer with retry, backoff, and gas price logic.
// marshalFn marshals an item to []byte.
// postSubmit is called after a successful submission to update caches, pending lists, etc.
func submitToDA[T any](
	m *Manager,
	ctx context.Context,
	items []T,
	marshalFn func(T) ([]byte, error),
	postSubmit func([]T, *coreda.ResultSubmit),
	itemType string,
) error {
	submittedAll := false
	var backoff time.Duration
	attempt := 0
	initialGasPrice := m.gasPrice
	gasPrice := initialGasPrice
	remaining := items
	numSubmitted := 0

	// Marshal all items once before the loop
	marshaled := make([][]byte, len(items))
	for i, item := range items {
		bz, err := marshalFn(item)
		if err != nil {
			return fmt.Errorf("failed to marshal item: %w", err)
		}
		marshaled[i] = bz
	}
	remLen := len(items)

	for !submittedAll && attempt < maxSubmitAttempts {
		select {
		case <-ctx.Done():
			m.logger.Info("context done, stopping submission loop")
			return nil
		case <-time.After(backoff):
		}

		// Use the current remaining items and marshaled bytes
		currMarshaled := marshaled
		remLen = len(remaining)

		submitctx, submitCtxCancel := context.WithTimeout(ctx, 60*time.Second)
		res := types.SubmitWithHelpers(submitctx, m.da, m.logger, currMarshaled, gasPrice, nil)
		submitCtxCancel()

		switch res.Code {
		case coreda.StatusSuccess:
			m.logger.Info(fmt.Sprintf("successfully submitted %s to DA layer", itemType), "gasPrice", gasPrice, "count", res.SubmittedCount)
			if res.SubmittedCount == uint64(remLen) {
				submittedAll = true
			}
			submitted := remaining[:res.SubmittedCount]
			notSubmitted := remaining[res.SubmittedCount:]
			notSubmittedMarshaled := currMarshaled[res.SubmittedCount:]
			numSubmitted += int(res.SubmittedCount)
			postSubmit(submitted, &res)
			remaining = notSubmitted
			marshaled = notSubmittedMarshaled
			backoff = 0
			if m.gasMultiplier > 0 && gasPrice != -1 {
				gasPrice = gasPrice / m.gasMultiplier
				gasPrice = max(gasPrice, initialGasPrice)
			}
			m.logger.Debug("resetting DA layer submission options", "backoff", backoff, "gasPrice", gasPrice)

			// Update sequencer metrics if the sequencer supports it
			if seq, ok := m.sequencer.(MetricsRecorder); ok {
				seq.RecordMetrics(gasPrice, res.BlobSize, res.Code, m.pendingHeaders.numPendingHeaders(), lastSubmittedHeight)
			}
		case coreda.StatusNotIncludedInBlock, coreda.StatusAlreadyInMempool:
			m.logger.Error("DA layer submission failed", "error", res.Message, "attempt", attempt)
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
			backoff = m.exponentialBackoff(backoff)
		}
		attempt++
	}
	if !submittedAll {
		// If not all items are submitted, the remaining items will be retried in the next submission loop.
		return fmt.Errorf("failed to submit all %s(s) to DA layer, submitted %d items (%d left) after %d attempts", itemType, numSubmitted, remLen, attempt)
	}
	return nil
}

// submitHeadersToDA submits a list of headers to the DA layer using the generic submitToDA helper.
func (m *Manager) submitHeadersToDA(ctx context.Context, headersToSubmit []*types.SignedHeader) error {
	return submitToDA(m, ctx, headersToSubmit,
		func(header *types.SignedHeader) ([]byte, error) {
			headerPb, err := header.ToProto()
			if err != nil {
				return nil, fmt.Errorf("failed to transform header to proto: %w", err)
			}
			return proto.Marshal(headerPb)
		},
		func(submitted []*types.SignedHeader, res *coreda.ResultSubmit) {
			for _, header := range submitted {
				m.headerCache.SetDAIncluded(header.Hash().String())
			}
			lastSubmittedHeaderHeight := uint64(0)
			if l := len(submitted); l > 0 {
				lastSubmittedHeaderHeight = submitted[l-1].Height()
			}
			m.pendingHeaders.setLastSubmittedHeaderHeight(ctx, lastSubmittedHeaderHeight)
			m.sendNonBlockingSignalToDAIncluderCh()
		},
		"header",
	)
}

// submitDataToDA submits a list of signed data to the DA layer using the generic submitToDA helper.
func (m *Manager) submitDataToDA(ctx context.Context, signedDataToSubmit []*types.SignedData) error {
	return submitToDA(m, ctx, signedDataToSubmit,
		func(signedData *types.SignedData) ([]byte, error) {
			return signedData.MarshalBinary()
		},
		func(submitted []*types.SignedData, res *coreda.ResultSubmit) {
			for _, signedData := range submitted {
				m.dataCache.SetDAIncluded(signedData.DACommitment().String())
			}
			lastSubmittedDataHeight := uint64(0)
			if l := len(submitted); l > 0 {
				lastSubmittedDataHeight = submitted[l-1].Height()
			}
			m.pendingData.setLastSubmittedDataHeight(ctx, lastSubmittedDataHeight)
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
