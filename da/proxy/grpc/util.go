package grpc

import (
	"github.com/rollkit/rollkit/da"
	pbda "github.com/rollkit/rollkit/types/pb/rollkit"
)

func blobsDA2PB(blobs []da.Blob) []*pbda.Blob {
	pb := make([]*pbda.Blob, len(blobs))
	for i := range blobs {
		pb[i] = &pbda.Blob{Value: blobs[i]}
	}
	return pb
}

func blobsPB2DA(pb []*pbda.Blob) []da.Blob {
	blobs := make([]da.Blob, len(pb))
	for i := range pb {
		blobs[i] = pb[i].Value
	}
	return blobs
}

func idsPB2DA(pb []*pbda.ID) []da.ID {
	ids := make([]da.ID, len(pb))
	for i := range ids {
		ids[i] = pb[i].Value
	}
	return ids
}

func idsDA2PB(ids []da.ID) []*pbda.ID {
	pb := make([]*pbda.ID, len(ids))
	for i := range ids {
		pb[i] = &pbda.ID{Value: ids[i]}
	}
	return pb
}

func commitsPB2DA(pb []*pbda.Commitment) []da.Commitment {
	commits := make([]da.Commitment, len(pb))
	for i := range pb {
		commits[i] = pb[i].Value
	}
	return commits
}

func commitsDA2PB(commits []da.Commitment) []*pbda.Commitment {
	pb := make([]*pbda.Commitment, len(commits))
	for i := range commits {
		pb[i] = &pbda.Commitment{Value: commits[i]}
	}
	return pb
}

func proofsPB2DA(pb []*pbda.Proof) []da.Proof {
	proofs := make([]da.Proof, len(pb))
	for i := range pb {
		proofs[i] = pb[i].Value
	}
	return proofs
}

func proofsDA2PB(proofs []da.Proof) []*pbda.Proof {
	pb := make([]*pbda.Proof, len(proofs))
	for i := range proofs {
		pb[i] = &pbda.Proof{Value: proofs[i]}
	}
	return pb
}
