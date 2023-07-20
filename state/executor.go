package state

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	cryptoenc "github.com/cometbft/cometbft/crypto/encoding"
	cmbytes "github.com/cometbft/cometbft/libs/bytes"
	cmstate "github.com/cometbft/cometbft/proto/tendermint/state"
	cmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cometbft/cometbft/proxy"
	cmtypes "github.com/cometbft/cometbft/types"
	"go.uber.org/multierr"

	abciconv "github.com/rollkit/rollkit/conv/abci"
	"github.com/rollkit/rollkit/log"
	"github.com/rollkit/rollkit/mempool"
	"github.com/rollkit/rollkit/types"
)

var ErrEmptyValSetGenerated = errors.New("applying the validator changes would result in empty set")
var ErrAddingValidatorToBased = errors.New("cannot add validators to empty validator set")

// BlockExecutor creates and applies blocks and maintains state.
type BlockExecutor struct {
	proposerAddress []byte
	namespaceID     types.NamespaceID
	chainID         string
	proxyApp        proxy.AppConnConsensus
	mempool         mempool.Mempool

	eventBus *cmtypes.EventBus

	logger log.Logger
}

// NewBlockExecutor creates new instance of BlockExecutor.
// Proposer address and namespace ID will be used in all newly created blocks.
func NewBlockExecutor(proposerAddress []byte, namespaceID [8]byte, chainID string, mempool mempool.Mempool, proxyApp proxy.AppConnConsensus, eventBus *cmtypes.EventBus, logger log.Logger) *BlockExecutor {
	return &BlockExecutor{
		proposerAddress: proposerAddress,
		namespaceID:     namespaceID,
		chainID:         chainID,
		proxyApp:        proxyApp,
		mempool:         mempool,
		eventBus:        eventBus,
		logger:          logger,
	}
}

// InitChain calls InitChainSync using consensus connection to app.
func (e *BlockExecutor) InitChain(genesis *cmtypes.GenesisDoc) (*abci.ResponseInitChain, error) {
	params := genesis.ConsensusParams

	validators := make([]*cmtypes.Validator, len(genesis.Validators))
	for i, v := range genesis.Validators {
		validators[i] = cmtypes.NewValidator(v.PubKey, v.Power)
	}

	return e.proxyApp.InitChainSync(abci.RequestInitChain{
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
		},
		Validators:    cmtypes.TM2PB.ValidatorUpdates(cmtypes.NewValidatorSet(validators)),
		AppStateBytes: genesis.AppState,
		InitialHeight: genesis.InitialHeight,
	})
}

// CreateBlock reaps transactions from mempool and builds a block.
func (e *BlockExecutor) CreateBlock(height uint64, lastCommit *types.Commit, lastHeaderHash types.Hash, state types.State) *types.Block {
	maxBytes := state.ConsensusParams.Block.MaxBytes
	maxGas := state.ConsensusParams.Block.MaxGas

	mempoolTxs := e.mempool.ReapMaxBytesMaxGas(maxBytes, maxGas)

	block := &types.Block{
		SignedHeader: types.SignedHeader{
			Header: types.Header{
				Version: types.Version{
					Block: state.Version.Consensus.Block,
					App:   state.Version.Consensus.App,
				},
				BaseHeader: types.BaseHeader{
					ChainID: e.chainID,
					Height:  height,
					Time:    uint64(time.Now().Unix()), // TODO(tzdybal): how to get TAI64?
				},
				//LastHeaderHash: lastHeaderHash,
				//LastCommitHash:  lastCommitHash,
				DataHash:        make(types.Hash, 32),
				ConsensusHash:   make(types.Hash, 32),
				AppHash:         state.AppHash,
				LastResultsHash: state.LastResultsHash,
				ProposerAddress: e.proposerAddress,
			},
			Commit: *lastCommit,
		},
		Data: types.Data{
			Txs:                    toRollkitTxs(mempoolTxs),
			IntermediateStateRoots: types.IntermediateStateRoots{RawRootsList: nil},
			// Note: Temporarily remove Evidence #896
			// Evidence:               types.EvidenceData{Evidence: nil},
		},
	}
	block.SignedHeader.Header.LastCommitHash = lastCommit.GetCommitHash(&block.SignedHeader.Header, e.proposerAddress)
	block.SignedHeader.Header.LastHeaderHash = lastHeaderHash
	block.SignedHeader.Header.AggregatorsHash = state.Validators.Hash()

	return block
}

// ApplyBlock validates and executes the block.
func (e *BlockExecutor) ApplyBlock(ctx context.Context, state types.State, block *types.Block) (types.State, *cmstate.ABCIResponses, error) {
	err := e.validate(state, block)
	if err != nil {
		return types.State{}, nil, err
	}

	// This makes calls to the AppClient
	resp, err := e.execute(ctx, state, block)
	if err != nil {
		return types.State{}, nil, err
	}

	abciValUpdates := resp.EndBlock.ValidatorUpdates

	err = validateValidatorUpdates(abciValUpdates, state.ConsensusParams.Validator)
	if err != nil {
		return state, nil, fmt.Errorf("error in validator updates: %v", err)
	}

	validatorUpdates, err := cmtypes.PB2TM.ValidatorUpdates(abciValUpdates)
	if err != nil {
		return state, nil, err
	}
	if len(validatorUpdates) > 0 {
		e.logger.Debug("updates to validators", "updates", cmtypes.ValidatorListString(validatorUpdates))
	}
	if state.ConsensusParams.Block.MaxBytes == 0 {
		e.logger.Error("maxBytes=0", "state.ConsensusParams.Block", state.ConsensusParams.Block, "block", block)
	}

	state, err = e.updateState(state, block, resp, validatorUpdates)
	if err != nil {
		return types.State{}, nil, err
	}

	return state, resp, nil
}

// Commit commits the block
func (e *BlockExecutor) Commit(ctx context.Context, state types.State, block *types.Block, resp *cmstate.ABCIResponses) ([]byte, uint64, error) {
	appHash, retainHeight, err := e.commit(ctx, state, block, resp.DeliverTxs)
	if err != nil {
		return []byte{}, 0, err
	}

	state.AppHash = appHash

	err = e.publishEvents(resp, block, state)
	if err != nil {
		e.logger.Error("failed to fire block events", "error", err)
	}

	return appHash, retainHeight, nil
}

func (e *BlockExecutor) updateState(state types.State, block *types.Block, abciResponses *cmstate.ABCIResponses, validatorUpdates []*cmtypes.Validator) (types.State, error) {
	nValSet := state.NextValidators.Copy()
	lastHeightValSetChanged := state.LastHeightValidatorsChanged
	// Rollkit can work without validators
	if len(nValSet.Validators) > 0 {
		if len(validatorUpdates) > 0 {
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
			lastHeightValSetChanged = block.SignedHeader.Header.Height() + 1 + 1
		}

		if len(nValSet.Validators) > 0 {
			nValSet.IncrementProposerPriority(1)
		}
		// TODO(tzdybal):  right now, it's for backward compatibility, may need to change this
	} else if len(validatorUpdates) > 0 {
		return state, ErrAddingValidatorToBased
	}

	s := types.State{
		Version:         state.Version,
		ChainID:         state.ChainID,
		InitialHeight:   state.InitialHeight,
		LastBlockHeight: block.SignedHeader.Header.Height(),
		LastBlockTime:   block.SignedHeader.Header.Time(),
		LastBlockID: cmtypes.BlockID{
			Hash: cmbytes.HexBytes(block.SignedHeader.Header.Hash()),
			// for now, we don't care about part set headers
		},
		NextValidators:                   nValSet,
		Validators:                       nValSet,
		LastValidators:                   state.Validators.Copy(),
		LastHeightValidatorsChanged:      lastHeightValSetChanged,
		ConsensusParams:                  state.ConsensusParams,
		LastHeightConsensusParamsChanged: state.LastHeightConsensusParamsChanged,
		AppHash:                          make(types.Hash, 32),
	}
	copy(s.LastResultsHash[:], cmtypes.NewResults(abciResponses.DeliverTxs).Hash())

	return s, nil
}

func (e *BlockExecutor) commit(ctx context.Context, state types.State, block *types.Block, deliverTxs []*abci.ResponseDeliverTx) ([]byte, uint64, error) {
	e.mempool.Lock()
	defer e.mempool.Unlock()

	err := e.mempool.FlushAppConn()
	if err != nil {
		return nil, 0, err
	}

	resp, err := e.proxyApp.CommitSync()
	if err != nil {
		return nil, 0, err
	}

	maxBytes := state.ConsensusParams.Block.MaxBytes
	maxGas := state.ConsensusParams.Block.MaxGas
	err = e.mempool.Update(int64(block.SignedHeader.Header.Height()), fromRollkitTxs(block.Data.Txs), deliverTxs, mempool.PreCheckMaxBytes(maxBytes), mempool.PostCheckMaxGas(maxGas))
	if err != nil {
		return nil, 0, err
	}

	return resp.Data, uint64(resp.RetainHeight), err
}

func (e *BlockExecutor) validate(state types.State, block *types.Block) error {
	err := block.ValidateBasic()
	if err != nil {
		return err
	}
	if block.SignedHeader.Header.Version.App != state.Version.Consensus.App ||
		block.SignedHeader.Header.Version.Block != state.Version.Consensus.Block {
		return errors.New("block version mismatch")
	}
	if state.LastBlockHeight <= 0 && block.SignedHeader.Header.Height() != state.InitialHeight {
		return errors.New("initial block height mismatch")
	}
	if state.LastBlockHeight > 0 && block.SignedHeader.Header.Height() != state.LastBlockHeight+1 {
		return errors.New("block height mismatch")
	}
	if !bytes.Equal(block.SignedHeader.Header.AppHash[:], state.AppHash[:]) {
		return errors.New("AppHash mismatch")
	}

	if !bytes.Equal(block.SignedHeader.Header.LastResultsHash[:], state.LastResultsHash[:]) {
		return errors.New("LastResultsHash mismatch")
	}

	if !bytes.Equal(block.SignedHeader.Header.AggregatorsHash[:], state.Validators.Hash()) {
		return errors.New("AggregatorsHash mismatch")
	}

	return nil
}

func (e *BlockExecutor) execute(ctx context.Context, state types.State, block *types.Block) (*cmstate.ABCIResponses, error) {
	abciResponses := new(cmstate.ABCIResponses)
	abciResponses.DeliverTxs = make([]*abci.ResponseDeliverTx, len(block.Data.Txs))

	txIdx := 0
	validTxs := 0
	invalidTxs := 0

	e.proxyApp.SetResponseCallback(func(req *abci.Request, res *abci.Response) {
		if r, ok := res.Value.(*abci.Response_DeliverTx); ok {
			txRes := r.DeliverTx
			if txRes.Code == abci.CodeTypeOK {
				validTxs++
			} else {
				e.logger.Debug("Invalid tx", "code", txRes.Code, "log", txRes.Log)
				invalidTxs++
			}
			abciResponses.DeliverTxs[txIdx] = txRes
			txIdx++
		}
	})

	hash := block.Hash()
	abciHeader, err := abciconv.ToABCIHeaderPB(&block.SignedHeader.Header)
	if err != nil {
		return nil, err
	}
	abciHeader.ChainID = e.chainID
	abciHeader.ValidatorsHash = state.Validators.Hash()
	beginBlockRequest := abci.RequestBeginBlock{
		Hash:   hash[:],
		Header: abciHeader,
		LastCommitInfo: abci.CommitInfo{
			Round: 0,
			Votes: nil,
		},
		ByzantineValidators: nil,
	}
	abciResponses.BeginBlock, err = e.proxyApp.BeginBlockSync(beginBlockRequest)
	if err != nil {
		return nil, err
	}

	for _, tx := range block.Data.Txs {
		res := e.proxyApp.DeliverTxAsync(abci.RequestDeliverTx{Tx: tx})
		if res.GetException() != nil {
			return nil, errors.New(res.GetException().GetError())
		}

	}
	endBlockRequest := abci.RequestEndBlock{Height: block.SignedHeader.Header.Height()}
	abciResponses.EndBlock, err = e.proxyApp.EndBlockSync(endBlockRequest)
	if err != nil {
		return nil, err
	}

	return abciResponses, nil
}

func (e *BlockExecutor) publishEvents(resp *cmstate.ABCIResponses, block *types.Block, state types.State) error {
	if e.eventBus == nil {
		return nil
	}

	abciBlock, err := abciconv.ToABCIBlock(block)
	abciBlock.Header.ValidatorsHash = state.Validators.Hash()
	if err != nil {
		return err
	}

	err = multierr.Append(err, e.eventBus.PublishEventNewBlock(cmtypes.EventDataNewBlock{
		Block:            abciBlock,
		ResultBeginBlock: *resp.BeginBlock,
		ResultEndBlock:   *resp.EndBlock,
	}))
	err = multierr.Append(err, e.eventBus.PublishEventNewBlockHeader(cmtypes.EventDataNewBlockHeader{
		Header:           abciBlock.Header,
		NumTxs:           int64(len(abciBlock.Txs)),
		ResultBeginBlock: *resp.BeginBlock,
		ResultEndBlock:   *resp.EndBlock,
	}))
	for _, ev := range abciBlock.Evidence.Evidence {
		err = multierr.Append(err, e.eventBus.PublishEventNewEvidence(cmtypes.EventDataNewEvidence{
			Evidence: ev,
			Height:   block.SignedHeader.Header.Height(),
		}))
	}
	for i, dtx := range resp.DeliverTxs {
		err = multierr.Append(err, e.eventBus.PublishEventTx(cmtypes.EventDataTx{
			TxResult: abci.TxResult{
				Height: block.SignedHeader.Header.Height(),
				Index:  uint32(i),
				Tx:     abciBlock.Data.Txs[i],
				Result: *dtx,
			},
		}))
	}
	return err
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

func validateValidatorUpdates(abciUpdates []abci.ValidatorUpdate, params *cmproto.ValidatorParams) error {
	for _, valUpdate := range abciUpdates {
		if valUpdate.GetPower() < 0 {
			return fmt.Errorf("voting power can't be negative %v", valUpdate)
		} else if valUpdate.GetPower() == 0 {
			// continue, since this is deleting the validator, and thus there is no
			// pubkey to check
			continue
		}

		// Check if validator's pubkey matches an ABCI type in the consensus params
		pk, err := cryptoenc.PubKeyFromProto(valUpdate.PubKey)
		if err != nil {
			return err
		}

		if !cmtypes.IsValidPubkeyType(cmtypes.ValidatorParams{PubKeyTypes: params.PubKeyTypes}, pk.Type()) {
			return fmt.Errorf("validator %v is using pubkey %s, which is unsupported for consensus",
				valUpdate, pk.Type())
		}
	}
	return nil
}
