// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package contracts

import (
	"errors"
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = errors.New
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
	_ = abi.ConvertType
)

// RollkitInboxMetaData contains all meta data concerning the RollkitInbox contract.
var RollkitInboxMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"namespace\",\"type\":\"bytes32\"}],\"name\":\"GetLatestBlock\",\"outputs\":[{\"internalType\":\"bytes\",\"name\":\"\",\"type\":\"bytes\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"namespace\",\"type\":\"bytes32\"},{\"internalType\":\"bytes\",\"name\":\"data\",\"type\":\"bytes\"}],\"name\":\"SubmitBlock\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"name\":\"namespaceToLatestBlock\",\"outputs\":[{\"internalType\":\"bytes\",\"name\":\"\",\"type\":\"bytes\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
	Bin: "0x608060405234801561001057600080fd5b506104df806100206000396000f3fe608060405234801561001057600080fd5b50600436106100415760003560e01c806361fa1ba614610046578063787a086c1461005b578063c5a5036814610084575b600080fd5b6100596100543660046101f2565b610097565b005b61006e61006936600461026e565b6100b6565b60405161007b9190610287565b60405180910390f35b61006e61009236600461026e565b610158565b60008381526020819052604090206100b08284836103c4565b50505050565b60008181526020819052604090208054606091906100d390610322565b80601f01602080910402602001604051908101604052809291908181526020018280546100ff90610322565b801561014c5780601f106101215761010080835404028352916020019161014c565b820191906000526020600020905b81548152906001019060200180831161012f57829003601f168201915b50505050509050919050565b6000602081905290815260409020805461017190610322565b80601f016020809104026020016040519081016040528092919081815260200182805461019d90610322565b80156101ea5780601f106101bf576101008083540402835291602001916101ea565b820191906000526020600020905b8154815290600101906020018083116101cd57829003601f168201915b505050505081565b60008060006040848603121561020757600080fd5b83359250602084013567ffffffffffffffff8082111561022657600080fd5b818601915086601f83011261023a57600080fd5b81358181111561024957600080fd5b87602082850101111561025b57600080fd5b6020830194508093505050509250925092565b60006020828403121561028057600080fd5b5035919050565b600060208083528351808285015260005b818110156102b457858101830151858201604001528201610298565b5060006040828601015260407fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe0601f8301168501019250505092915050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052604160045260246000fd5b600181811c9082168061033657607f821691505b60208210810361036f577f4e487b7100000000000000000000000000000000000000000000000000000000600052602260045260246000fd5b50919050565b601f8211156103bf57600081815260208120601f850160051c8101602086101561039c5750805b601f850160051c820191505b818110156103bb578281556001016103a8565b5050505b505050565b67ffffffffffffffff8311156103dc576103dc6102f3565b6103f0836103ea8354610322565b83610375565b6000601f841160018114610442576000851561040c5750838201355b7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff600387901b1c1916600186901b1783556104d8565b6000838152602090207fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe0861690835b828110156104915786850135825560209485019460019092019101610471565b50868210156104cc577fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff60f88860031b161c19848701351681555b505060018560011b0183555b505050505056",
}

// RollkitInboxABI is the input ABI used to generate the binding from.
// Deprecated: Use RollkitInboxMetaData.ABI instead.
var RollkitInboxABI = RollkitInboxMetaData.ABI

// RollkitInboxBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use RollkitInboxMetaData.Bin instead.
var RollkitInboxBin = RollkitInboxMetaData.Bin

// DeployRollkitInbox deploys a new Ethereum contract, binding an instance of RollkitInbox to it.
func DeployRollkitInbox(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *RollkitInbox, error) {
	parsed, err := RollkitInboxMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(RollkitInboxBin), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &RollkitInbox{RollkitInboxCaller: RollkitInboxCaller{contract: contract}, RollkitInboxTransactor: RollkitInboxTransactor{contract: contract}, RollkitInboxFilterer: RollkitInboxFilterer{contract: contract}}, nil
}

// RollkitInbox is an auto generated Go binding around an Ethereum contract.
type RollkitInbox struct {
	RollkitInboxCaller     // Read-only binding to the contract
	RollkitInboxTransactor // Write-only binding to the contract
	RollkitInboxFilterer   // Log filterer for contract events
}

// RollkitInboxCaller is an auto generated read-only Go binding around an Ethereum contract.
type RollkitInboxCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// RollkitInboxTransactor is an auto generated write-only Go binding around an Ethereum contract.
type RollkitInboxTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// RollkitInboxFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type RollkitInboxFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// RollkitInboxSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type RollkitInboxSession struct {
	Contract     *RollkitInbox     // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// RollkitInboxCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type RollkitInboxCallerSession struct {
	Contract *RollkitInboxCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts       // Call options to use throughout this session
}

// RollkitInboxTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type RollkitInboxTransactorSession struct {
	Contract     *RollkitInboxTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts       // Transaction auth options to use throughout this session
}

// RollkitInboxRaw is an auto generated low-level Go binding around an Ethereum contract.
type RollkitInboxRaw struct {
	Contract *RollkitInbox // Generic contract binding to access the raw methods on
}

// RollkitInboxCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type RollkitInboxCallerRaw struct {
	Contract *RollkitInboxCaller // Generic read-only contract binding to access the raw methods on
}

// RollkitInboxTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type RollkitInboxTransactorRaw struct {
	Contract *RollkitInboxTransactor // Generic write-only contract binding to access the raw methods on
}

// NewRollkitInbox creates a new instance of RollkitInbox, bound to a specific deployed contract.
func NewRollkitInbox(address common.Address, backend bind.ContractBackend) (*RollkitInbox, error) {
	contract, err := bindRollkitInbox(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &RollkitInbox{RollkitInboxCaller: RollkitInboxCaller{contract: contract}, RollkitInboxTransactor: RollkitInboxTransactor{contract: contract}, RollkitInboxFilterer: RollkitInboxFilterer{contract: contract}}, nil
}

// NewRollkitInboxCaller creates a new read-only instance of RollkitInbox, bound to a specific deployed contract.
func NewRollkitInboxCaller(address common.Address, caller bind.ContractCaller) (*RollkitInboxCaller, error) {
	contract, err := bindRollkitInbox(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &RollkitInboxCaller{contract: contract}, nil
}

// NewRollkitInboxTransactor creates a new write-only instance of RollkitInbox, bound to a specific deployed contract.
func NewRollkitInboxTransactor(address common.Address, transactor bind.ContractTransactor) (*RollkitInboxTransactor, error) {
	contract, err := bindRollkitInbox(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &RollkitInboxTransactor{contract: contract}, nil
}

// NewRollkitInboxFilterer creates a new log filterer instance of RollkitInbox, bound to a specific deployed contract.
func NewRollkitInboxFilterer(address common.Address, filterer bind.ContractFilterer) (*RollkitInboxFilterer, error) {
	contract, err := bindRollkitInbox(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &RollkitInboxFilterer{contract: contract}, nil
}

// bindRollkitInbox binds a generic wrapper to an already deployed contract.
func bindRollkitInbox(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := RollkitInboxMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_RollkitInbox *RollkitInboxRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _RollkitInbox.Contract.RollkitInboxCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_RollkitInbox *RollkitInboxRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _RollkitInbox.Contract.RollkitInboxTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_RollkitInbox *RollkitInboxRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _RollkitInbox.Contract.RollkitInboxTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_RollkitInbox *RollkitInboxCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _RollkitInbox.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_RollkitInbox *RollkitInboxTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _RollkitInbox.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_RollkitInbox *RollkitInboxTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _RollkitInbox.Contract.contract.Transact(opts, method, params...)
}

// GetLatestBlock is a free data retrieval call binding the contract method 0x787a086c.
//
// Solidity: function GetLatestBlock(bytes32 namespace) view returns(bytes)
func (_RollkitInbox *RollkitInboxCaller) GetLatestBlock(opts *bind.CallOpts, namespace [32]byte) ([]byte, error) {
	var out []interface{}
	err := _RollkitInbox.contract.Call(opts, &out, "GetLatestBlock", namespace)

	if err != nil {
		return *new([]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([]byte)).(*[]byte)

	return out0, err

}

// GetLatestBlock is a free data retrieval call binding the contract method 0x787a086c.
//
// Solidity: function GetLatestBlock(bytes32 namespace) view returns(bytes)
func (_RollkitInbox *RollkitInboxSession) GetLatestBlock(namespace [32]byte) ([]byte, error) {
	return _RollkitInbox.Contract.GetLatestBlock(&_RollkitInbox.CallOpts, namespace)
}

// GetLatestBlock is a free data retrieval call binding the contract method 0x787a086c.
//
// Solidity: function GetLatestBlock(bytes32 namespace) view returns(bytes)
func (_RollkitInbox *RollkitInboxCallerSession) GetLatestBlock(namespace [32]byte) ([]byte, error) {
	return _RollkitInbox.Contract.GetLatestBlock(&_RollkitInbox.CallOpts, namespace)
}

// NamespaceToLatestBlock is a free data retrieval call binding the contract method 0xc5a50368.
//
// Solidity: function namespaceToLatestBlock(bytes32 ) view returns(bytes)
func (_RollkitInbox *RollkitInboxCaller) NamespaceToLatestBlock(opts *bind.CallOpts, arg0 [32]byte) ([]byte, error) {
	var out []interface{}
	err := _RollkitInbox.contract.Call(opts, &out, "namespaceToLatestBlock", arg0)

	if err != nil {
		return *new([]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([]byte)).(*[]byte)

	return out0, err

}

// NamespaceToLatestBlock is a free data retrieval call binding the contract method 0xc5a50368.
//
// Solidity: function namespaceToLatestBlock(bytes32 ) view returns(bytes)
func (_RollkitInbox *RollkitInboxSession) NamespaceToLatestBlock(arg0 [32]byte) ([]byte, error) {
	return _RollkitInbox.Contract.NamespaceToLatestBlock(&_RollkitInbox.CallOpts, arg0)
}

// NamespaceToLatestBlock is a free data retrieval call binding the contract method 0xc5a50368.
//
// Solidity: function namespaceToLatestBlock(bytes32 ) view returns(bytes)
func (_RollkitInbox *RollkitInboxCallerSession) NamespaceToLatestBlock(arg0 [32]byte) ([]byte, error) {
	return _RollkitInbox.Contract.NamespaceToLatestBlock(&_RollkitInbox.CallOpts, arg0)
}

// SubmitBlock is a paid mutator transaction binding the contract method 0x61fa1ba6.
//
// Solidity: function SubmitBlock(bytes32 namespace, bytes data) returns()
func (_RollkitInbox *RollkitInboxTransactor) SubmitBlock(opts *bind.TransactOpts, namespace [32]byte, data []byte) (*types.Transaction, error) {
	return _RollkitInbox.contract.Transact(opts, "SubmitBlock", namespace, data)
}

// SubmitBlock is a paid mutator transaction binding the contract method 0x61fa1ba6.
//
// Solidity: function SubmitBlock(bytes32 namespace, bytes data) returns()
func (_RollkitInbox *RollkitInboxSession) SubmitBlock(namespace [32]byte, data []byte) (*types.Transaction, error) {
	return _RollkitInbox.Contract.SubmitBlock(&_RollkitInbox.TransactOpts, namespace, data)
}

// SubmitBlock is a paid mutator transaction binding the contract method 0x61fa1ba6.
//
// Solidity: function SubmitBlock(bytes32 namespace, bytes data) returns()
func (_RollkitInbox *RollkitInboxTransactorSession) SubmitBlock(namespace [32]byte, data []byte) (*types.Transaction, error) {
	return _RollkitInbox.Contract.SubmitBlock(&_RollkitInbox.TransactOpts, namespace, data)
}
