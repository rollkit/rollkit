package state

import (
	"bytes"
	"context"
	"errors"
	"time"

	abci "github.com/lazyledger/lazyledger-core/abci/types"
	tmstate "github.com/lazyledger/lazyledger-core/proto/tendermint/state"
	"github.com/lazyledger/lazyledger-core/proxy"
	lltypes "github.com/lazyledger/lazyledger-core/types"

	abciconv "github.com/lazyledger/optimint/conv/abci"
	"github.com/lazyledger/optimint/log"
	"github.com/lazyledger/optimint/mempool"
	"github.com/lazyledger/optimint/types"
)

// BlockExecutor creates and applies blocks and maintain state.
type BlockExecutor struct {
	proposerAddress []byte
	namespaceID     [8]byte
	proxyApp        proxy.AppConnConsensus
	mempool         mempool.Mempool

	logger log.Logger
}

// NewBlockExecutor creates new instance of BlockExecutor.
// Proposer address and namespace ID will be used in all newly created blocks.
func NewBlockExecutor(proposerAddress []byte, namespaceID [8]byte, mempool mempool.Mempool, proxyApp proxy.AppConnConsensus, logger log.Logger) *BlockExecutor {
	return &BlockExecutor{
		proposerAddress: proposerAddress,
		namespaceID:     namespaceID,
		proxyApp:        proxyApp,
		mempool:         mempool,
		logger:          logger,
	}
}

// CreateBlock reaps transactions from mempool and builds a block.
func (e *BlockExecutor) CreateBlock(height uint64, commit *types.Commit, state State) *types.Block {
	maxBytes := state.ConsensusParams.Block.MaxBytes
	maxGas := state.ConsensusParams.Block.MaxGas

	mempoolTxs := e.mempool.ReapMaxBytesMaxGas(maxBytes, maxGas)

	block := &types.Block{
		Header: types.Header{
			Version: types.Version{
				Block: state.Version.Consensus.Block,
				App:   state.Version.Consensus.App,
			},
			NamespaceID:     e.namespaceID,
			Height:          height,
			Time:            uint64(time.Now().Unix()), // TODO(tzdybal): how to get TAI64?
			LastHeaderHash:  [32]byte{},
			LastCommitHash:  [32]byte{},
			DataHash:        [32]byte{},
			ConsensusHash:   [32]byte{},
			AppHash:         state.AppHash,
			LastResultsHash: state.LastResultsHash,
			ProposerAddress: e.proposerAddress,
		},
		Data: types.Data{
			Txs:                    toOptimintTxs(mempoolTxs),
			IntermediateStateRoots: types.IntermediateStateRoots{RawRootsList: nil},
			Evidence:               types.EvidenceData{Evidence: nil},
		},
	}

	return block
}

// ApplyBlock validates, executes and commits the block.
func (e *BlockExecutor) ApplyBlock(ctx context.Context, state State, block *types.Block) (State, uint64, error) {
	err := e.validate(state, block)
	if err != nil {
		return State{}, 0, err
	}

	resp, err := e.execute(ctx, state, block)
	if err != nil {
		return State{}, 0, err
	}

	state, err = e.updateState(state, block, resp)
	if err != nil {
		return State{}, 0, err
	}

	appHash, retainHeight, err := e.commit(ctx, state, block, resp.DeliverTxs)
	if err != nil {
		return State{}, 0, err
	}

	copy(state.AppHash[:], appHash[:])

	return state, retainHeight, nil
}

func (e *BlockExecutor) updateState(state State, block *types.Block, abciResponses *tmstate.ABCIResponses) (State, error) {
	hash := block.Header.Hash()
	s := State{
		Version:         state.Version,
		ChainID:         state.ChainID,
		InitialHeight:   state.InitialHeight,
		LastBlockHeight: int64(block.Header.Height),
		LastBlockTime:   time.Unix(int64(block.Header.Time), 0),
		LastBlockID: lltypes.BlockID{
			Hash: hash[:],
			// for now, we don't care about part set headers
		},
		// skipped all "Validators" fields
		ConsensusParams:                  state.ConsensusParams,
		LastHeightConsensusParamsChanged: state.LastHeightConsensusParamsChanged,
	}
	copy(s.LastResultsHash[:], lltypes.NewResults(abciResponses.DeliverTxs).Hash())

	return s, nil
}

func (e *BlockExecutor) commit(ctx context.Context, state State, block *types.Block, deliverTxs []*abci.ResponseDeliverTx) ([]byte, uint64, error) {
	e.mempool.Lock()
	defer e.mempool.Unlock()

	err := e.mempool.FlushAppConn()
	if err != nil {
		return nil, 0, err
	}

	resp, err := e.proxyApp.CommitSync(ctx)
	if err != nil {
		return nil, 0, err
	}

	maxBytes := state.ConsensusParams.Block.MaxBytes
	maxGas := state.ConsensusParams.Block.MaxGas
	err = e.mempool.Update(int64(block.Header.Height), fromOptimintTxs(block.Data.Txs), deliverTxs, mempool.PreCheckMaxBytes(maxBytes), mempool.PostCheckMaxGas(maxGas))
	if err != nil {
		return nil, 0, err
	}

	return resp.Data, uint64(resp.RetainHeight), err
}

func (e *BlockExecutor) validate(state State, block *types.Block) error {
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

func (e *BlockExecutor) execute(ctx context.Context, state State, block *types.Block) (*tmstate.ABCIResponses, error) {
	abciResponses := new(tmstate.ABCIResponses)
	abciResponses.DeliverTxs = make([]*abci.ResponseDeliverTx, len(block.Data.Txs))

	txIdx := 0
	validTxs := 0
	invalidTxs := 0

	var err error

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
	abciHeader, err := abciconv.ToABCIHeader(&block.Header)
	if err != nil {
		return nil, err
	}
	abciResponses.BeginBlock, err = e.proxyApp.BeginBlockSync(
		ctx,
		abci.RequestBeginBlock{
			Hash:   hash[:],
			Header: abciHeader,
			LastCommitInfo: abci.LastCommitInfo{
				Round: 0,
				Votes: nil,
			},
			ByzantineValidators: nil,
		})
	if err != nil {
		return nil, err
	}

	for _, tx := range block.Data.Txs {
		_, err = e.proxyApp.DeliverTxAsync(ctx, abci.RequestDeliverTx{Tx: tx})
		if err != nil {
			return nil, err
		}
	}

	abciResponses.EndBlock, err = e.proxyApp.EndBlockSync(ctx, abci.RequestEndBlock{Height: int64(block.Header.Height)})
	if err != nil {
		return nil, err
	}

	return abciResponses, nil
}

func toOptimintTxs(txs lltypes.Txs) types.Txs {
	optiTxs := make(types.Txs, len(txs))
	for i := range txs {
		optiTxs[i] = []byte(txs[i])
	}
	return optiTxs
}

func fromOptimintTxs(optiTxs types.Txs) lltypes.Txs {
	txs := make(lltypes.Txs, len(optiTxs))
	for i := range optiTxs {
		txs[i] = []byte(optiTxs[i])
	}
	return txs
}
