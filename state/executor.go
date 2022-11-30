package state

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	abci "github.com/tendermint/tendermint/abci/types"
	cryptoenc "github.com/tendermint/tendermint/crypto/encoding"
	tmstate "github.com/tendermint/tendermint/proto/tendermint/state"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	"github.com/tendermint/tendermint/proxy"
	tmtypes "github.com/tendermint/tendermint/types"
	"go.uber.org/multierr"

	abciconv "github.com/celestiaorg/rollmint/conv/abci"
	"github.com/celestiaorg/rollmint/log"
	"github.com/celestiaorg/rollmint/mempool"
	"github.com/celestiaorg/rollmint/types"
)

// BlockExecutor creates and applies blocks and maintains state.
type BlockExecutor struct {
	proposerAddress    []byte
	namespaceID        types.NamespaceID
	chainID            string
	proxyApp           proxy.AppConnConsensus
	mempool            mempool.Mempool
	fraudProofsEnabled bool

	eventBus *tmtypes.EventBus

	logger log.Logger
}

// NewBlockExecutor creates new instance of BlockExecutor.
// Proposer address and namespace ID will be used in all newly created blocks.
func NewBlockExecutor(proposerAddress []byte, namespaceID [8]byte, chainID string, mempool mempool.Mempool, proxyApp proxy.AppConnConsensus, fraudProofsEnabled bool, eventBus *tmtypes.EventBus, logger log.Logger) *BlockExecutor {
	return &BlockExecutor{
		proposerAddress:    proposerAddress,
		namespaceID:        namespaceID,
		chainID:            chainID,
		proxyApp:           proxyApp,
		mempool:            mempool,
		fraudProofsEnabled: fraudProofsEnabled,
		eventBus:           eventBus,
		logger:             logger,
	}
}

// InitChain calls InitChainSync using consensus connection to app.
func (e *BlockExecutor) InitChain(genesis *tmtypes.GenesisDoc) (*abci.ResponseInitChain, error) {
	params := genesis.ConsensusParams

	validators := make([]*tmtypes.Validator, len(genesis.Validators))
	for i, v := range genesis.Validators {
		validators[i] = tmtypes.NewValidator(v.PubKey, v.Power)
	}

	return e.proxyApp.InitChainSync(abci.RequestInitChain{
		Time:    genesis.GenesisTime,
		ChainId: genesis.ChainID,
		ConsensusParams: &abci.ConsensusParams{
			Block: &abci.BlockParams{
				MaxBytes: params.Block.MaxBytes,
				MaxGas:   params.Block.MaxGas,
			},
			Evidence: &tmproto.EvidenceParams{
				MaxAgeNumBlocks: params.Evidence.MaxAgeNumBlocks,
				MaxAgeDuration:  params.Evidence.MaxAgeDuration,
				MaxBytes:        params.Evidence.MaxBytes,
			},
			Validator: &tmproto.ValidatorParams{
				PubKeyTypes: params.Validator.PubKeyTypes,
			},
			Version: &tmproto.VersionParams{
				AppVersion: params.Version.AppVersion,
			},
		},
		Validators:    tmtypes.TM2PB.ValidatorUpdates(tmtypes.NewValidatorSet(validators)),
		AppStateBytes: genesis.AppState,
		InitialHeight: genesis.InitialHeight,
	})
}

// CreateBlock reaps transactions from mempool and builds a block.
func (e *BlockExecutor) CreateBlock(height uint64, lastCommit *types.Commit, lastHeaderHash [32]byte, state types.State) *types.Block {
	maxBytes := state.ConsensusParams.Block.MaxBytes
	maxGas := state.ConsensusParams.Block.MaxGas

	mempoolTxs := e.mempool.ReapMaxBytesMaxGas(maxBytes, maxGas)

	block := &types.Block{
		Header: types.Header{
			Version: types.Version{
				Block: state.Version.Consensus.Block,
				App:   state.Version.Consensus.App,
			},
			NamespaceID:    e.namespaceID,
			Height:         height,
			Time:           uint64(time.Now().Unix()), // TODO(tzdybal): how to get TAI64?
			LastHeaderHash: lastHeaderHash,
			//LastCommitHash:  lastCommitHash,
			DataHash:        [32]byte{},
			ConsensusHash:   [32]byte{},
			AppHash:         state.AppHash,
			LastResultsHash: state.LastResultsHash,
			ProposerAddress: e.proposerAddress,
		},
		Data: types.Data{
			Txs:                    toRollmintTxs(mempoolTxs),
			IntermediateStateRoots: types.IntermediateStateRoots{RawRootsList: nil},
			Evidence:               types.EvidenceData{Evidence: nil},
		},
		LastCommit: *lastCommit,
	}
	copy(block.Header.LastCommitHash[:], e.getLastCommitHash(lastCommit, &block.Header))
	copy(block.Header.AggregatorsHash[:], state.Validators.Hash())

	return block
}

// ApplyBlock validates and executes the block.
func (e *BlockExecutor) ApplyBlock(ctx context.Context, state types.State, block *types.Block) (types.State, *tmstate.ABCIResponses, error) {
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

	validatorUpdates, err := tmtypes.PB2TM.ValidatorUpdates(abciValUpdates)
	if err != nil {
		return state, nil, err
	}
	if len(validatorUpdates) > 0 {
		e.logger.Debug("updates to validators", "updates", tmtypes.ValidatorListString(validatorUpdates))
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
func (e *BlockExecutor) Commit(ctx context.Context, state types.State, block *types.Block, resp *tmstate.ABCIResponses) ([]byte, uint64, error) {
	appHash, retainHeight, err := e.commit(ctx, state, block, resp.DeliverTxs)
	if err != nil {
		return []byte{}, 0, err
	}

	copy(state.AppHash[:], appHash[:])

	err = e.publishEvents(resp, block, state)
	if err != nil {
		e.logger.Error("failed to fire block events", "error", err)
	}
	return appHash, retainHeight, nil
}

func (e *BlockExecutor) VerifyFraudProof(fraudProof abci.FraudProof, expectedAppHash []byte) (bool, error) {
	resp, err := e.proxyApp.VerifyFraudProofSync(
		abci.RequestVerifyFraudProof{
			FraudProof:      &fraudProof,
			ExpectedAppHash: expectedAppHash,
		},
	)
	if err != nil {
		return false, err
	}
	return resp.Success, nil

}

func (e *BlockExecutor) updateState(state types.State, block *types.Block, abciResponses *tmstate.ABCIResponses, validatorUpdates []*tmtypes.Validator) (types.State, error) {
	nValSet := state.NextValidators.Copy()
	lastHeightValSetChanged := state.LastHeightValidatorsChanged
	// rollmint can work without validators
	if len(nValSet.Validators) > 0 {
		if len(validatorUpdates) > 0 {
			err := nValSet.UpdateWithChangeSet(validatorUpdates)
			if err != nil {
				return state, nil
			}
			// Change results from this height but only applies to the next next height.
			lastHeightValSetChanged = int64(block.Header.Height + 1 + 1)
		}

		// TODO(tzdybal):  right now, it's for backward compatibility, may need to change this
		nValSet.IncrementProposerPriority(1)
	}

	hash := block.Header.Hash()
	s := types.State{
		Version:         state.Version,
		ChainID:         state.ChainID,
		InitialHeight:   state.InitialHeight,
		LastBlockHeight: int64(block.Header.Height),
		LastBlockTime:   time.Unix(int64(block.Header.Time), 0),
		LastBlockID: tmtypes.BlockID{
			Hash: hash[:],
			// for now, we don't care about part set headers
		},
		NextValidators:                   nValSet,
		Validators:                       state.NextValidators.Copy(),
		LastValidators:                   state.Validators.Copy(),
		LastHeightValidatorsChanged:      lastHeightValSetChanged,
		ConsensusParams:                  state.ConsensusParams,
		LastHeightConsensusParamsChanged: state.LastHeightConsensusParamsChanged,
		AppHash:                          [32]byte{},
	}
	copy(s.LastResultsHash[:], tmtypes.NewResults(abciResponses.DeliverTxs).Hash())

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
	err = e.mempool.Update(int64(block.Header.Height), fromRollmintTxs(block.Data.Txs), deliverTxs, mempool.PreCheckMaxBytes(maxBytes), mempool.PostCheckMaxGas(maxGas))
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
	if block.Header.Version.App != state.Version.Consensus.App ||
		block.Header.Version.Block != state.Version.Consensus.Block {
		return errors.New("block version mismatch")
	}
	if state.LastBlockHeight <= 0 && block.Header.Height != uint64(state.InitialHeight) {
		return errors.New("initial block height mismatch")
	}
	if state.LastBlockHeight > 0 && block.Header.Height != uint64(state.LastBlockHeight)+1 {
		return errors.New("block height mismatch")
	}
	if !bytes.Equal(block.Header.AppHash[:], state.AppHash[:]) {
		return errors.New("AppHash mismatch")
	}

	if !bytes.Equal(block.Header.LastResultsHash[:], state.LastResultsHash[:]) {
		return errors.New("LastResultsHash mismatch")
	}

	return nil
}

func (e *BlockExecutor) execute(ctx context.Context, state types.State, block *types.Block) (*tmstate.ABCIResponses, error) {
	abciResponses := new(tmstate.ABCIResponses)
	abciResponses.DeliverTxs = make([]*abci.ResponseDeliverTx, len(block.Data.Txs))

	txIdx := 0
	validTxs := 0
	invalidTxs := 0

	currentIsrs := block.Data.IntermediateStateRoots.RawRootsList
	currentIsrIndex := 0

	if e.fraudProofsEnabled && currentIsrs != nil {
		expectedLength := len(block.Data.Txs) + 2
		// BeginBlock + DeliverTxs + EndBlock
		if len(currentIsrs) != expectedLength {
			return nil, fmt.Errorf("invalid length of ISR list: %d, expected length: %d", len(currentIsrs), expectedLength)
		}
	}

	ISRs := make([][]byte, 0)

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

	genAndGossipFraudProofIfNeeded := func(beginBlockRequest *abci.RequestBeginBlock, deliverTxRequests []*abci.RequestDeliverTx, endBlockRequest *abci.RequestEndBlock) (err error) {
		if !e.fraudProofsEnabled {
			return nil
		}
		isr, err := e.getAppHash()
		if err != nil {
			return err
		}
		ISRs = append(ISRs, isr)
		isFraud := e.isFraudProofTrigger(isr, currentIsrs, currentIsrIndex)
		if isFraud {
			fraudProof, err := e.generateFraudProof(beginBlockRequest, deliverTxRequests, endBlockRequest)
			if err != nil {
				return err
			}
			// TODO: gossip fraudProof to P2P network
			// fraudTx: current DeliverTx
			_ = fraudProof
		}
		currentIsrIndex++
		return nil
	}

	hash := block.Hash()
	abciHeader, err := abciconv.ToABCIHeaderPB(&block.Header)
	if err != nil {
		return nil, err
	}
	abciHeader.ChainID = e.chainID
	abciHeader.ValidatorsHash = state.Validators.Hash()
	beginBlockRequest := abci.RequestBeginBlock{
		Hash:   hash[:],
		Header: abciHeader,
		LastCommitInfo: abci.LastCommitInfo{
			Round: 0,
			Votes: nil,
		},
		ByzantineValidators: nil,
	}
	abciResponses.BeginBlock, err = e.proxyApp.BeginBlockSync(beginBlockRequest)
	if err != nil {
		return nil, err
	}

	err = genAndGossipFraudProofIfNeeded(&beginBlockRequest, nil, nil)
	if err != nil {
		return nil, err
	}

	deliverTxRequests := make([]*abci.RequestDeliverTx, len(block.Data.Txs))
	for _, tx := range block.Data.Txs {
		deliverTxRequest := abci.RequestDeliverTx{Tx: tx}
		deliverTxRequests = append(deliverTxRequests, &deliverTxRequest)
		res := e.proxyApp.DeliverTxAsync(deliverTxRequest)
		if res.GetException() != nil {
			return nil, errors.New(res.GetException().GetError())
		}

		err = genAndGossipFraudProofIfNeeded(&beginBlockRequest, deliverTxRequests, nil)
		if err != nil {
			return nil, err
		}
	}
	endBlockRequest := abci.RequestEndBlock{Height: int64(block.Header.Height)}
	abciResponses.EndBlock, err = e.proxyApp.EndBlockSync(endBlockRequest)
	if err != nil {
		return nil, err
	}

	err = genAndGossipFraudProofIfNeeded(&beginBlockRequest, deliverTxRequests, &endBlockRequest)
	if err != nil {
		return nil, err
	}

	if e.fraudProofsEnabled && block.Data.IntermediateStateRoots.RawRootsList == nil {
		// Block producer: Initial ISRs generated here
		block.Data.IntermediateStateRoots.RawRootsList = ISRs
	}

	return abciResponses, nil
}

func (e *BlockExecutor) isFraudProofTrigger(generatedIsr []byte, currentIsrs [][]byte, index int) bool {
	if currentIsrs == nil {
		return false
	}
	stateIsr := currentIsrs[index]
	if !bytes.Equal(stateIsr, generatedIsr) {
		e.logger.Debug("ISR Mismatch", "given_isr", stateIsr, "generated_isr", generatedIsr)
		return true
	}
	return false
}

func (e *BlockExecutor) generateFraudProof(beginBlockRequest *abci.RequestBeginBlock, deliverTxRequests []*abci.RequestDeliverTx, endBlockRequest *abci.RequestEndBlock) (*abci.FraudProof, error) {
	generateFraudProofRequest := abci.RequestGenerateFraudProof{}
	if beginBlockRequest == nil {
		return nil, fmt.Errorf("begin block request cannot be a nil parameter")
	}
	generateFraudProofRequest.BeginBlockRequest = *beginBlockRequest
	if deliverTxRequests != nil {
		generateFraudProofRequest.DeliverTxRequests = deliverTxRequests
		if endBlockRequest != nil {
			generateFraudProofRequest.EndBlockRequest = endBlockRequest
		}
	}
	resp, err := e.proxyApp.GenerateFraudProofSync(generateFraudProofRequest)
	if err != nil {
		return nil, err
	}
	if resp.FraudProof == nil {
		return nil, fmt.Errorf("fraud proof generation failed")
	}
	return resp.FraudProof, nil
}

func (e *BlockExecutor) getLastCommitHash(lastCommit *types.Commit, header *types.Header) []byte {
	lastABCICommit := abciconv.ToABCICommit(lastCommit)
	if len(lastCommit.Signatures) == 1 {
		lastABCICommit.Signatures[0].ValidatorAddress = e.proposerAddress
		lastABCICommit.Signatures[0].Timestamp = time.UnixMilli(int64(header.Time))
	}
	return lastABCICommit.Hash()
}

func (e *BlockExecutor) publishEvents(resp *tmstate.ABCIResponses, block *types.Block, state types.State) error {
	if e.eventBus == nil {
		return nil
	}

	abciBlock, err := abciconv.ToABCIBlock(block)
	abciBlock.Header.ValidatorsHash = state.Validators.Hash()
	if err != nil {
		return err
	}

	err = multierr.Append(err, e.eventBus.PublishEventNewBlock(tmtypes.EventDataNewBlock{
		Block:            abciBlock,
		ResultBeginBlock: *resp.BeginBlock,
		ResultEndBlock:   *resp.EndBlock,
	}))
	err = multierr.Append(err, e.eventBus.PublishEventNewBlockHeader(tmtypes.EventDataNewBlockHeader{
		Header:           abciBlock.Header,
		NumTxs:           int64(len(abciBlock.Txs)),
		ResultBeginBlock: *resp.BeginBlock,
		ResultEndBlock:   *resp.EndBlock,
	}))
	for _, ev := range abciBlock.Evidence.Evidence {
		err = multierr.Append(err, e.eventBus.PublishEventNewEvidence(tmtypes.EventDataNewEvidence{
			Evidence: ev,
			Height:   int64(block.Header.Height),
		}))
	}
	for i, dtx := range resp.DeliverTxs {
		err = multierr.Append(err, e.eventBus.PublishEventTx(tmtypes.EventDataTx{
			TxResult: abci.TxResult{
				Height: int64(block.Header.Height),
				Index:  uint32(i),
				Tx:     abciBlock.Data.Txs[i],
				Result: *dtx,
			},
		}))
	}
	return err
}

func (e *BlockExecutor) getAppHash() ([]byte, error) {
	isrResp, err := e.proxyApp.GetAppHashSync(abci.RequestGetAppHash{})
	if err != nil {
		return nil, err
	}
	return isrResp.AppHash, nil
}

func toRollmintTxs(txs tmtypes.Txs) types.Txs {
	rollmintTxs := make(types.Txs, len(txs))
	for i := range txs {
		rollmintTxs[i] = []byte(txs[i])
	}
	return rollmintTxs
}

func fromRollmintTxs(rollmintTxs types.Txs) tmtypes.Txs {
	txs := make(tmtypes.Txs, len(rollmintTxs))
	for i := range rollmintTxs {
		txs[i] = []byte(rollmintTxs[i])
	}
	return txs
}

func validateValidatorUpdates(abciUpdates []abci.ValidatorUpdate,
	params tmproto.ValidatorParams) error {
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

		if !tmtypes.IsValidPubkeyType(params, pk.Type()) {
			return fmt.Errorf("validator %v is using pubkey %s, which is unsupported for consensus",
				valUpdate, pk.Type())
		}
	}
	return nil
}
