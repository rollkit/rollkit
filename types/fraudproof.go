package types

// Represents a single-round fraudProof
type FraudProof struct {
	BlockHeight  uint64
	StateWitness StateWitness
}

type StateWitness struct {
	WitnessData []WitnessData
}

type WitnessData struct {
	Key   []byte
	Value []byte
}
