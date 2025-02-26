package types

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/cometbft/cometbft/crypto/merkle"
	"google.golang.org/protobuf/proto"

	pb "github.com/rollkit/rollkit/types/pb/rollkit"
)

//----------------------------------------
// Validator
//----------------------------------------

// Validator is a type for a validator.
// Volatile state for each Validator
// NOTE: The ProposerPriority is not included in Validator.Hash();
// make sure to update that method if changes are made here.
type Validator struct {
	Address     []byte `json:"address"`
	PubKey      PubKey `json:"pub_key"`
	VotingPower int64  `json:"voting_power"`

	ProposerPriority int64 `json:"proposer_priority"`
}

// ToProto converts Validator to protobuf.
func (v *Validator) ToProto() (*pb.Validator, error) {
	if v == nil {
		return nil, errors.New("nil validator")
	}

	if len(v.PubKey.Bytes) == 0 {
		return nil, errors.New("nil pubkey")
	}

	vb := pb.Validator{}
	vb.SetAddress(v.Address)
	vb.SetPubKeyType(v.PubKey.Type)
	vb.SetPubKeyBytes(v.PubKey.Bytes)
	vb.SetVotingPower(v.VotingPower)
	vb.SetProposerPriority(v.ProposerPriority)

	return &vb, nil
}

// ValidateBasic performs basic validation.
func (v *Validator) ValidateBasic() error {
	if v == nil {
		return errors.New("nil validator")
	}
	if len(v.PubKey.Bytes) == 0 {
		return errors.New("validator does not have a public key")
	}

	if v.VotingPower < 0 {
		return errors.New("validator has negative voting power")
	}

	addr := v.PubKey.Address()
	if !bytes.Equal(v.Address, addr) {
		return fmt.Errorf("validator address is incorrectly derived from pubkey. Exp: %v, got %v", addr, v.Address)
	}

	return nil
}

// Bytes computes the unique encoding of a validator with a given voting power.
// These are the bytes that gets hashed in consensus. It excludes address
// as its redundant with the pubkey. This also excludes ProposerPriority
// which changes every round.
func (v *Validator) Bytes() []byte {
	pbv := pb.Validator{}
	pbv.SetAddress(v.Address)
	pbv.SetPubKeyType(v.PubKey.Type)
	pbv.SetPubKeyBytes(v.PubKey.Bytes)
	pbv.SetVotingPower(v.VotingPower)
	pbv.SetProposerPriority(v.ProposerPriority)

	bz, err := proto.Marshal(&pbv)
	if err != nil {
		panic(err)
	}
	return bz
}

// ValidatorFromProto sets a protobuf Validator to the given pointer.
// It returns an error if the public key is invalid.
func ValidatorFromProto(vp *pb.Validator) (*Validator, error) {
	if vp == nil {
		return nil, errors.New("nil validator")
	}

	v := new(Validator)
	v.Address = vp.GetAddress()
	v.PubKey = PubKey{
		Type:  vp.GetPubKeyType(),
		Bytes: vp.GetPubKeyBytes(),
	}
	v.VotingPower = vp.GetVotingPower()
	v.ProposerPriority = vp.GetProposerPriority()

	return v, nil
}

//----------------------------------------
// ValidatorSet
//----------------------------------------

// ErrProposerNotInVals is returned if the proposer is not in the validator set.
var ErrProposerNotInVals = errors.New("proposer not in validator set")

// ValidatorSet is a set of validators.
type ValidatorSet struct {
	// NOTE: persisted via reflect, must be exported.
	Validators []*Validator `json:"validators"`
	Proposer   *Validator   `json:"proposer"`

	// cached (unexported)
	totalVotingPower int64
	// true if all validators have the same type of public key or if the set is empty.
	allKeysHaveSameType bool
}

// NewValidatorSet initializes a ValidatorSet by copying over the values from
// `valz`, a list of Validators. If valz is nil or empty, the new ValidatorSet
// will have an empty list of Validators.
//
// The addresses of validators in `valz` must be unique otherwise the function
// panics.
//
// Note the validator set size has an implied limit equal to that of the
// MaxVotesCount - commits by a validator set larger than this will fail
// validation.
func NewValidatorSet(valz []*Validator) *ValidatorSet {
	vals := &ValidatorSet{
		allKeysHaveSameType: true,
	}
	return vals
}

// ToProto converts ValidatorSet to protobuf
func (vals *ValidatorSet) ToProto() (*pb.ValidatorSet, error) {
	if vals == nil || len(vals.Validators) == 0 {
		return &pb.ValidatorSet{}, nil // validator set should never be nil
	}

	vp := new(pb.ValidatorSet)
	valsProto := make([]*pb.Validator, len(vals.Validators))
	for i := 0; i < len(vals.Validators); i++ {
		valp, err := vals.Validators[i].ToProto()
		if err != nil {
			return nil, err
		}
		valsProto[i] = valp
	}
	vp.SetValidators(valsProto)

	valProposer, err := vals.Proposer.ToProto()
	if err != nil {
		return nil, fmt.Errorf("toProto: validatorSet proposer error: %w", err)
	}
	vp.SetProposer(valProposer)

	// NOTE: Sometimes we use the bytes of the proto form as a hash. This means that we need to
	// be consistent with cached data
	vp.SetTotalVotingPower(0)

	return vp, nil
}

// ValidateBasic performs basic validation on the validator set.
func (vals *ValidatorSet) ValidateBasic() error {
	if vals == nil || len(vals.Validators) == 0 {
		return errors.New("validator set is nil or empty")
	}

	for idx, val := range vals.Validators {
		if err := val.ValidateBasic(); err != nil {
			return fmt.Errorf("invalid validator #%d: %w", idx, err)
		}
	}

	if err := vals.Proposer.ValidateBasic(); err != nil {
		return fmt.Errorf("proposer failed validate basic, error: %w", err)
	}

	for _, val := range vals.Validators {
		if bytes.Equal(val.Address, vals.Proposer.Address) {
			return nil
		}
	}

	return ErrProposerNotInVals
}

// ValidatorSetFromProto sets a protobuf ValidatorSet to the given pointer.
// It returns an error if any of the validators from the set or the proposer
// is invalid
func ValidatorSetFromProto(vp *pb.ValidatorSet) (*ValidatorSet, error) {
	if vp == nil {
		return nil, errors.New("nil validator set") // validator set should never be nil, bigger issues are at play if empty
	}
	vals := new(ValidatorSet)

	valsProto := make([]*Validator, len(vp.GetValidators()))
	for i := 0; i < len(vp.GetValidators()); i++ {
		v, err := ValidatorFromProto(vp.GetValidators()[i])
		if err != nil {
			return nil, err
		}
		valsProto[i] = v
	}
	vals.Validators = valsProto

	p, err := ValidatorFromProto(vp.GetProposer())
	if err != nil {
		return nil, fmt.Errorf("fromProto: validatorSet proposer error: %w", err)
	}

	vals.Proposer = p

	// NOTE: We can't trust the total voting power given to us by other peers. If someone were to
	// inject a non-zeo value that wasn't the correct voting power we could assume a wrong total
	// power hence we need to recompute it.
	// FIXME: We should look to remove TotalVotingPower from proto or add it in the validators hash
	// so we don't have to do this
	vals.totalVotingPower = vp.GetTotalVotingPower()

	return vals, vals.ValidateBasic()
}

// Hash returns the Merkle root hash build using validators (as leaves) in the
// set.
//
// See merkle.HashFromByteSlices.
func (vals *ValidatorSet) Hash() []byte {
	bzs := make([][]byte, len(vals.Validators))
	for i, val := range vals.Validators {
		bzs[i] = val.Bytes()
	}
	return merkle.HashFromByteSlices(bzs)
}
