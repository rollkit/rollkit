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
	"github.com/lazyledger/optimint/hash"
	"github.com/lazyledger/optimint/log"
	"github.com/lazyledger/optimint/mempool"
	"github.com/lazyledger/optimint/types"
)

type BlockExecutor struct {
	proxyApp proxy.AppConnConsensus
	mempool  mempool.Mempool

	logger log.Logger
}

func NewBlockExecutor(mempool mempool.Mempool, proxyApp proxy.AppConnConsensus, logger log.Logger) *BlockExecutor {
	return &BlockExecutor{
		proxyApp: proxyApp,
		mempool:  mempool,
		logger:   logger,
	}
}

func (e *BlockExecutor) CreateProposalBlock(height uint64, commit *types.Commit, state State) *types.Block {
	maxBytes := state.ConsensusParams.Block.MaxBytes
	maxGas := state.ConsensusParams.Block.MaxGas

	mempoolTxs := e.mempool.ReapMaxBytesMaxGas(maxBytes, maxGas)

	block := &types.Block{
		Header: types.Header{
			Version: types.Version{
				Block: state.Version.Consensus.Block,
				App:   state.Version.Consensus.App,
			},
			NamespaceID:     [8]byte{},
			Height:          height,
			Time:            uint64(time.Now().Unix()), // TODO(tzdybal): how to get TAI64?
			LastHeaderHash:  [32]byte{},
			LastCommitHash:  [32]byte{},
			DataHash:        [32]byte{},
			ConsensusHash:   [32]byte{},
			AppHash:         state.AppHash,
			LastResultsHash: state.LastResultsHash,
			ProposerAddress: nil,
		},
		Data: types.Data{
			Txs:                    toOptimintTxs(mempoolTxs),
			IntermediateStateRoots: types.IntermediateStateRoots{RawRootsList: nil},
			Evidence:               types.EvidenceData{Evidence: nil},
		},
	}

	return block
}

func (e *BlockExecutor) ApplyBlock(state State, block *types.Block) (State, uint64, error) {
	err := e.validate(state, block)
	if err != nil {
		return State{}, 0, err
	}

	resp, err := e.execute(state, block)
	if err != nil {
		return State{}, 0, err
	}

	state, err = e.updateState(state, block, resp)
	if err != nil {
		return State{}, 0, err
	}

	appHash, retainHeight, err := e.commit(state, block, resp.DeliverTxs)
	if err != nil {
		return State{}, 0, err
	}

	copy(state.AppHash[:], appHash[:])

	return state, retainHeight, nil
}

func (e *BlockExecutor) updateState(state State, block *types.Block, abciResponses *tmstate.ABCIResponses) (State, error) {
	// TODO(tzdybal): blockID
	s := State{
		Version:         state.Version,
		ChainID:         state.ChainID,
		InitialHeight:   state.InitialHeight,
		LastBlockHeight: int64(block.Header.Height),
		LastBlockTime:   time.Unix(int64(block.Header.Time), 0),
		// skipped all "Validators" fields
		ConsensusParams:                  state.ConsensusParams,
		LastHeightConsensusParamsChanged: state.LastHeightConsensusParamsChanged,
	}
	copy(s.LastResultsHash[:], lltypes.NewResults(abciResponses.DeliverTxs).Hash())

	return s, nil
}

func (e *BlockExecutor) commit(state State, block *types.Block, deliverTxs []*abci.ResponseDeliverTx) ([]byte, uint64, error) {
	e.mempool.Lock()
	defer e.mempool.Unlock()

	err := e.mempool.FlushAppConn()
	if err != nil {
		return nil, 0, err
	}

	resp, err := e.proxyApp.CommitSync(context.TODO())
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

	// TODO(tzdybal): what else to check

	return nil
}

func (e *BlockExecutor) execute(state State, block *types.Block) (*tmstate.ABCIResponses, error) {
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

	hash, err := hash.Hash(block)
	if err != nil {
		return nil, err
	}
	abciResponses.BeginBlock, err = e.proxyApp.BeginBlockSync(
		context.TODO(),
		abci.RequestBeginBlock{
			Hash:   hash[:],
			Header: abciconv.ToABCIHeader(&block.Header),
			LastCommitInfo: abci.LastCommitInfo{
				Round: 0,
				Votes: nil,
			},
			ByzantineValidators: nil,
		})

	for _, tx := range block.Data.Txs {
		_, err = e.proxyApp.DeliverTxAsync(context.TODO(), abci.RequestDeliverTx{Tx: tx})
		if err != nil {
			return nil, err
		}
	}

	abciResponses.EndBlock, err = e.proxyApp.EndBlockSync(context.TODO(), abci.RequestEndBlock{Height: int64(block.Header.Height)})
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
