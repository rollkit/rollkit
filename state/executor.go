package state

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	cmbytes "github.com/cometbft/cometbft/libs/bytes"
	cmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cometbft/cometbft/proxy"
	cmtypes "github.com/cometbft/cometbft/types"

	"github.com/rollkit/rollkit/mempool"
	"github.com/rollkit/rollkit/third_party/log"
	"github.com/rollkit/rollkit/types"
	abciconv "github.com/rollkit/rollkit/types/abci"
)

// ErrEmptyValSetGenerated is returned when applying the validator changes would result in empty set.
var ErrEmptyValSetGenerated = errors.New("applying the validator changes would result in empty set")

// ErrAddingValidatorToBased is returned when trying to add a validator to an empty validator set.
var ErrAddingValidatorToBased = errors.New("cannot add validators to empty validator set")

// BlockExecutor creates and applies blocks and maintains state.
type BlockExecutor struct {
	proposerAddress []byte
	chainID         string
	proxyApp        proxy.AppConnConsensus
	mempool         mempool.Mempool
	maxBytes        uint64

	eventBus *cmtypes.EventBus

	logger log.Logger

	metrics *Metrics
}

// NewBlockExecutor creates new instance of BlockExecutor.
func NewBlockExecutor(proposerAddress []byte, chainID string, mempool mempool.Mempool, proxyApp proxy.AppConnConsensus, eventBus *cmtypes.EventBus, maxBytes uint64, logger log.Logger, metrics *Metrics) *BlockExecutor {
	return &BlockExecutor{
		proposerAddress: proposerAddress,
		chainID:         chainID,
		proxyApp:        proxyApp,
		mempool:         mempool,
		eventBus:        eventBus,
		maxBytes:        maxBytes,
		logger:          logger,
		metrics:         metrics,
	}
}

// InitChain calls InitChainSync using consensus connection to app.
func (e *BlockExecutor) InitChain(genesis *cmtypes.GenesisDoc) (*abci.ResponseInitChain, error) {
	params := genesis.ConsensusParams

	validators := make([]*cmtypes.Validator, len(genesis.Validators))
	for i, v := range genesis.Validators {
		validators[i] = cmtypes.NewValidator(v.PubKey, v.Power)
	}

	return e.proxyApp.InitChain(context.Background(), &abci.RequestInitChain{
		Time:    genesis.GenesisTime,
		ChainId: genesis.ChainID,
		ConsensusParams: &cmproto.ConsensusParams{
			Block: &cmproto.BlockParams{
				MaxBytes: params.Block.MaxBytes,
				MaxGas:   params.Block.MaxGas,
			},
			Evidence: &cmproto.EvidenceParams{
				MaxAgeNumBlocks: params.Evidence.MaxAgeNumBlocks,
				MaxAgeDuration:  params.Evidence.MaxAgeDuration,
				MaxBytes:        params.Evidence.MaxBytes,
			},
			Validator: &cmproto.ValidatorParams{
				PubKeyTypes: params.Validator.PubKeyTypes,
			},
			Version: &cmproto.VersionParams{
				App: params.Version.App,
			},
			Abci: &cmproto.ABCIParams{
				VoteExtensionsEnableHeight: params.ABCI.VoteExtensionsEnableHeight,
			},
		},
		Validators:    cmtypes.TM2PB.ValidatorUpdates(cmtypes.NewValidatorSet(validators)),
		AppStateBytes: genesis.AppState,
		InitialHeight: genesis.InitialHeight,
	})
}

// CreateBlock reaps transactions from mempool and builds a block.
func (e *BlockExecutor) CreateBlock(height uint64, lastSignature *types.Signature, lastExtendedCommit abci.ExtendedCommitInfo, lastHeaderHash types.Hash, state types.State, txs cmtypes.Txs) (*types.SignedHeader, *types.Data, error) {
	maxBytes := state.ConsensusParams.Block.MaxBytes
	emptyMaxBytes := maxBytes == -1
	if emptyMaxBytes {
		maxBytes = int64(cmtypes.MaxBlockSizeBytes)
	}
	if maxBytes > int64(e.maxBytes) { //nolint:gosec
		e.logger.Debug("limiting maxBytes to", "e.maxBytes=%d", e.maxBytes)
		maxBytes = int64(e.maxBytes) //nolint:gosec
	}

	// maxGas := state.ConsensusParams.Block.MaxGas

	// mempoolTxs := e.mempool.ReapMaxBytesMaxGas(maxBytes, maxGas)

	header := &types.SignedHeader{
		Header: types.Header{
			Version: types.Version{
				Block: state.Version.Consensus.Block,
				App:   state.Version.Consensus.App,
			},
			BaseHeader: types.BaseHeader{
				ChainID: e.chainID,
				Height:  height,
				Time:    uint64(time.Now().UnixNano()),
			},
			//LastHeaderHash: lastHeaderHash,
			//LastCommitHash:  lastCommitHash,
			DataHash:        make(types.Hash, 32),
			ConsensusHash:   make(types.Hash, 32),
			AppHash:         state.AppHash,
			LastResultsHash: state.LastResultsHash,
			ProposerAddress: e.proposerAddress,
		},
		Signature: *lastSignature,
	}
	data := &types.Data{
		Txs: toRollkitTxs(txs),
		// IntermediateStateRoots: types.IntermediateStateRoots{RawRootsList: nil},
		// Note: Temporarily remove Evidence #896
		// Evidence:               types.EvidenceData{Evidence: nil},
	}

	rpp, err := e.proxyApp.PrepareProposal(
		context.TODO(),
		&abci.RequestPrepareProposal{
			MaxTxBytes:         maxBytes,
			Txs:                txs.ToSliceOfBytes(),
			LocalLastCommit:    lastExtendedCommit,
			Misbehavior:        []abci.Misbehavior{},
			Height:             int64(header.Height()), //nolint:gosec
			Time:               header.Time(),          //TODO: replace with sequencer timestamp
			NextValidatorsHash: state.Validators.Hash(),
			ProposerAddress:    e.proposerAddress,
		},
	)
	if err != nil {
		// The App MUST ensure that only valid (and hence 'processable') transactions
		// enter the mempool. Hence, at this point, we can't have any non-processable
		// transaction causing an error.
		//
		// Also, the App can simply skip any transaction that could cause any kind of trouble.
		// Either way, we cannot recover in a meaningful way, unless we skip proposing
		// this block, repair what caused the error and try again. Hence, we return an
		// error for now (the production code calling this function is expected to panic).
		return nil, nil, err
	}

	txl := cmtypes.ToTxs(rpp.Txs)
	if err := txl.Validate(maxBytes); err != nil {
		return nil, nil, err
	}

	data.Txs = toRollkitTxs(txl)
	// Note: This is hash of an ABCI type commit equivalent of the last signature in the signed header.
	header.LastCommitHash = lastSignature.GetCommitHash(&header.Header, e.proposerAddress)
	header.LastHeaderHash = lastHeaderHash

	return header, data, nil
}

// ProcessProposal calls the corresponding ABCI method on the app.
func (e *BlockExecutor) ProcessProposal(
	header *types.SignedHeader,
	data *types.Data,
	state types.State,
) (bool, error) {
	resp, err := e.proxyApp.ProcessProposal(context.TODO(), &abci.RequestProcessProposal{
		Hash:   header.Hash(),
		Height: int64(header.Height()), //nolint:gosec
		Time:   header.Time(),
		Txs:    data.Txs.ToSliceOfBytes(),
		ProposedLastCommit: abci.CommitInfo{
			Round: 0,
			Votes: []abci.VoteInfo{
				{
					Validator: abci.Validator{
						Address: header.Validators.GetProposer().Address,
						Power:   header.Validators.GetProposer().VotingPower,
					},
					BlockIdFlag: cmproto.BlockIDFlagCommit,
				},
			},
		},
		Misbehavior:        []abci.Misbehavior{},
		ProposerAddress:    e.proposerAddress,
		NextValidatorsHash: state.Validators.Hash(),
	})
	if err != nil {
		return false, err
	}
	if resp.IsStatusUnknown() {
		panic(fmt.Sprintf("ProcessProposal responded with status %s", resp.Status.String()))
	}

	return resp.IsAccepted(), nil
}

// ApplyBlock validates and executes the block.
func (e *BlockExecutor) ApplyBlock(ctx context.Context, state types.State, header *types.SignedHeader, data *types.Data) (types.State, *abci.ResponseFinalizeBlock, error) {
	isAppValid, err := e.ProcessProposal(header, data, state)
	if err != nil {
		return types.State{}, nil, err
	}
	if !isAppValid {
		return types.State{}, nil, fmt.Errorf("proposal processing resulted in an invalid application state")
	}

	err = e.Validate(state, header, data)
	if err != nil {
		return types.State{}, nil, err
	}
	// This makes calls to the AppClient
	resp, err := e.execute(ctx, state, header, data)
	if err != nil {
		return types.State{}, nil, err
	}
	abciValUpdates := resp.ValidatorUpdates

	validatorUpdates, err := cmtypes.PB2TM.ValidatorUpdates(abciValUpdates)
	if err != nil {
		return state, nil, err
	}

	if resp.ConsensusParamUpdates != nil {
		e.metrics.ConsensusParamUpdates.Add(1)
	}

	state, err = e.updateState(state, header, data, resp, validatorUpdates)
	if err != nil {
		return types.State{}, nil, err
	}

	if state.ConsensusParams.Block.MaxBytes <= 0 {
		e.logger.Error("maxBytes<=0", "state.ConsensusParams.Block", state.ConsensusParams.Block, "header", header)
	}

	return state, resp, nil
}

// ExtendVote calls the ExtendVote ABCI method on the proxy app.
func (e *BlockExecutor) ExtendVote(ctx context.Context, header *types.SignedHeader, data *types.Data) ([]byte, error) {
	resp, err := e.proxyApp.ExtendVote(ctx, &abci.RequestExtendVote{
		Hash:   header.Hash(),
		Height: int64(header.Height()), //nolint:gosec
		Time:   header.Time(),
		Txs:    data.Txs.ToSliceOfBytes(),
		ProposedLastCommit: abci.CommitInfo{
			Votes: []abci.VoteInfo{{
				Validator: abci.Validator{
					Address: header.Validators.GetProposer().Address,
					Power:   header.Validators.GetProposer().VotingPower,
				},
				BlockIdFlag: cmproto.BlockIDFlagCommit,
			}},
		},
		Misbehavior:        nil,
		NextValidatorsHash: header.ValidatorHash,
		ProposerAddress:    header.ProposerAddress,
	})
	if err != nil {
		return nil, err
	}
	return resp.VoteExtension, nil
}

// Commit commits the block
func (e *BlockExecutor) Commit(ctx context.Context, state types.State, header *types.SignedHeader, data *types.Data, resp *abci.ResponseFinalizeBlock) ([]byte, uint64, error) {
	appHash, retainHeight, err := e.commit(ctx, state, header, data, resp)
	if err != nil {
		return []byte{}, 0, err
	}

	state.AppHash = appHash

	e.publishEvents(resp, header, data, state)

	return appHash, retainHeight, nil
}

// updateConsensusParams updates the consensus parameters based on the provided updates.
func (e *BlockExecutor) updateConsensusParams(height uint64, params cmtypes.ConsensusParams, consensusParamUpdates *cmproto.ConsensusParams) (cmproto.ConsensusParams, uint64, error) {
	nextParams := params.Update(consensusParamUpdates)
	if err := types.ConsensusParamsValidateBasic(nextParams); err != nil {
		return cmproto.ConsensusParams{}, 0, fmt.Errorf("validating new consensus params: %w", err)
	}
	if err := nextParams.ValidateUpdate(consensusParamUpdates, int64(height)); err != nil { //nolint:gosec
		return cmproto.ConsensusParams{}, 0, fmt.Errorf("updating consensus params: %w", err)
	}
	return nextParams.ToProto(), nextParams.Version.App, nil
}

func (e *BlockExecutor) updateState(state types.State, header *types.SignedHeader, data *types.Data, finalizeBlockResponse *abci.ResponseFinalizeBlock, validatorUpdates []*cmtypes.Validator) (types.State, error) {
	height := header.Height()
	if finalizeBlockResponse.ConsensusParamUpdates != nil {
		nextParamsProto, appVersion, err := e.updateConsensusParams(height, types.ConsensusParamsFromProto(state.ConsensusParams), finalizeBlockResponse.ConsensusParamUpdates)
		if err != nil {
			return types.State{}, err
		}
		// Change results from this height but only applies to the next height.
		state.LastHeightConsensusParamsChanged = height + 1
		state.Version.Consensus.App = appVersion
		state.ConsensusParams = nextParamsProto
	}

	nValSet := state.NextValidators.Copy()
	lastHeightValSetChanged := state.LastHeightValidatorsChanged

	if len(nValSet.Validators) > 0 {
		err := nValSet.UpdateWithChangeSet(validatorUpdates)
		if err != nil {
			if err.Error() != ErrEmptyValSetGenerated.Error() {
				return state, err
			}
			nValSet = &cmtypes.ValidatorSet{
				Validators: make([]*cmtypes.Validator, 0),
				Proposer:   nil,
			}
		}
		// Change results from this height but only applies to the next next height.
		lastHeightValSetChanged = int64(header.Header.Height() + 1 + 1) //nolint:gosec

		if len(nValSet.Validators) > 0 {
			nValSet.IncrementProposerPriority(1)
		}
	}

	s := types.State{
		Version:         state.Version,
		ChainID:         state.ChainID,
		InitialHeight:   state.InitialHeight,
		LastBlockHeight: height,
		LastBlockTime:   header.Time(),
		LastBlockID: cmtypes.BlockID{
			Hash: cmbytes.HexBytes(header.Hash()),
			// for now, we don't care about part set headers
		},
		ConsensusParams:                  state.ConsensusParams,
		LastHeightConsensusParamsChanged: state.LastHeightConsensusParamsChanged,
		AppHash:                          finalizeBlockResponse.AppHash,
		Validators:                       state.NextValidators.Copy(),
		NextValidators:                   nValSet,
		LastHeightValidatorsChanged:      lastHeightValSetChanged,
		LastValidators:                   state.Validators.Copy(),
	}
	copy(s.LastResultsHash[:], cmtypes.NewResults(finalizeBlockResponse.TxResults).Hash())

	return s, nil
}

func (e *BlockExecutor) commit(ctx context.Context, state types.State, header *types.SignedHeader, data *types.Data, resp *abci.ResponseFinalizeBlock) ([]byte, uint64, error) {
	e.mempool.Lock()
	defer e.mempool.Unlock()

	err := e.mempool.FlushAppConn()
	if err != nil {
		return nil, 0, err
	}

	commitResp, err := e.proxyApp.Commit(ctx)
	if err != nil {
		return nil, 0, err
	}

	maxBytes := state.ConsensusParams.Block.MaxBytes
	maxGas := state.ConsensusParams.Block.MaxGas
	err = e.mempool.Update(header.Height(), fromRollkitTxs(data.Txs), resp.TxResults, mempool.PreCheckMaxBytes(maxBytes), mempool.PostCheckMaxGas(maxGas))
	if err != nil {
		return nil, 0, err
	}

	return resp.AppHash, uint64(commitResp.RetainHeight), err
}

// Validate validates the state and the block for the executor
func (e *BlockExecutor) Validate(state types.State, header *types.SignedHeader, data *types.Data) error {
	err := header.ValidateBasic()
	if err != nil {
		return err
	}
	err = data.ValidateBasic()
	if err != nil {
		return err
	}
	if header.Version.App != state.Version.Consensus.App ||
		header.Version.Block != state.Version.Consensus.Block {
		return errors.New("block version mismatch")
	}
	if state.LastBlockHeight <= 0 && header.Height() != state.InitialHeight {
		return errors.New("initial block height mismatch")
	}
	if state.LastBlockHeight > 0 && header.Height() != state.LastBlockHeight+1 {
		return errors.New("block height mismatch")
	}
	if !bytes.Equal(header.AppHash[:], state.AppHash[:]) {
		return errors.New("AppHash mismatch")
	}

	if !bytes.Equal(header.LastResultsHash[:], state.LastResultsHash[:]) {
		return errors.New("LastResultsHash mismatch")
	}

	return nil
}

func (e *BlockExecutor) execute(ctx context.Context, state types.State, header *types.SignedHeader, data *types.Data) (*abci.ResponseFinalizeBlock, error) {
	// Only execute if the node hasn't already shut down
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	abciHeader, err := abciconv.ToABCIHeaderPB(&header.Header)
	if err != nil {
		return nil, err
	}
	abciHeader.ChainID = e.chainID
	abciBlock, err := abciconv.ToABCIBlock(header, data)
	if err != nil {
		return nil, err
	}

	startTime := time.Now().UnixNano()
	finalizeBlockResponse, err := e.proxyApp.FinalizeBlock(ctx, &abci.RequestFinalizeBlock{
		Hash:               header.Hash(),
		NextValidatorsHash: state.Validators.Hash(),
		ProposerAddress:    abciHeader.ProposerAddress,
		Height:             abciHeader.Height,
		Time:               abciHeader.Time,
		DecidedLastCommit: abci.CommitInfo{
			Round: 0,
			Votes: nil,
		},
		Misbehavior: abciBlock.Evidence.Evidence.ToABCI(),
		Txs:         abciBlock.Txs.ToSliceOfBytes(),
	})
	endTime := time.Now().UnixNano()
	e.metrics.BlockProcessingTime.Observe(float64(endTime-startTime) / 1000000)
	if err != nil {
		e.logger.Error("error in proxyAppConn.FinalizeBlock", "err", err)
		return nil, err
	}

	e.logger.Info(
		"finalized block",
		"height", abciBlock.Height,
		"num_txs_res", len(finalizeBlockResponse.TxResults),
		"num_val_updates", len(finalizeBlockResponse.ValidatorUpdates),
		"block_app_hash", fmt.Sprintf("%X", finalizeBlockResponse.AppHash),
	)

	// Assert that the application correctly returned tx results for each of the transactions provided in the block
	if len(abciBlock.Data.Txs) != len(finalizeBlockResponse.TxResults) {
		return nil, fmt.Errorf("expected tx results length to match size of transactions in block. Expected %d, got %d", len(data.Txs), len(finalizeBlockResponse.TxResults))
	}

	e.logger.Info("executed block", "height", abciHeader.Height, "app_hash", fmt.Sprintf("%X", finalizeBlockResponse.AppHash))

	return finalizeBlockResponse, nil
}

func (e *BlockExecutor) publishEvents(resp *abci.ResponseFinalizeBlock, header *types.SignedHeader, data *types.Data, state types.State) {
	if e.eventBus == nil {
		return
	}

	abciBlock, err := abciconv.ToABCIBlock(header, data)
	if err != nil {
		return
	}

	if err := e.eventBus.PublishEventNewBlock(cmtypes.EventDataNewBlock{
		Block: abciBlock,
		BlockID: cmtypes.BlockID{
			Hash: cmbytes.HexBytes(header.Hash()),
			// for now, we don't care about part set headers
		},
		ResultFinalizeBlock: *resp,
	}); err != nil {
		e.logger.Error("failed publishing new block", "err", err)
	}

	if err := e.eventBus.PublishEventNewBlockHeader(cmtypes.EventDataNewBlockHeader{
		Header: abciBlock.Header,
	}); err != nil {
		e.logger.Error("failed publishing new block header", "err", err)
	}

	if err := e.eventBus.PublishEventNewBlockEvents(cmtypes.EventDataNewBlockEvents{
		Height: abciBlock.Height,
		Events: resp.Events,
		NumTxs: int64(len(abciBlock.Txs)),
	}); err != nil {
		e.logger.Error("failed publishing new block events", "err", err)
	}

	if len(abciBlock.Evidence.Evidence) != 0 {
		for _, ev := range abciBlock.Evidence.Evidence {
			if err := e.eventBus.PublishEventNewEvidence(cmtypes.EventDataNewEvidence{
				Evidence: ev,
				Height:   int64(header.Header.Height()), //nolint:gosec
			}); err != nil {
				e.logger.Error("failed publishing new evidence", "err", err)
			}
		}
	}

	for i, tx := range abciBlock.Data.Txs {
		err := e.eventBus.PublishEventTx(cmtypes.EventDataTx{
			TxResult: abci.TxResult{
				Height: abciBlock.Height,
				Index:  uint32(i), //nolint:gosec
				Tx:     tx,
				Result: *(resp.TxResults[i]),
			},
		})
		if err != nil {
			e.logger.Error("failed publishing event TX", "err", err)
		}
	}
}

func toRollkitTxs(txs cmtypes.Txs) types.Txs {
	rollkitTxs := make(types.Txs, len(txs))
	for i := range txs {
		rollkitTxs[i] = []byte(txs[i])
	}
	return rollkitTxs
}

func fromRollkitTxs(rollkitTxs types.Txs) cmtypes.Txs {
	txs := make(cmtypes.Txs, len(rollkitTxs))
	for i := range rollkitTxs {
		txs[i] = []byte(rollkitTxs[i])
	}
	return txs
}
