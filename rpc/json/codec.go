package json

import (
	gorillarpc "github.com/gorilla/rpc/v2"
	gorillajson "github.com/gorilla/rpc/v2/json"
	"net/http"
)

type MapperCodec struct {
	aliases map[string]string
	codec gorillarpc.Codec
}

func (m *MapperCodec) NewRequest(request *http.Request) gorillarpc.CodecRequest {
	return &MapperCodecRequest{
		CodecRequest: m.codec.NewRequest(request).(*gorillajson.CodecRequest),
		aliases: m.aliases,
	}
}

func NewMapperCodec(aliases map[string]string) *MapperCodec {
	return &MapperCodec{
		aliases: aliases,
		codec: gorillajson.NewCodec(),
	}
}

type MapperCodecRequest struct {
	*gorillajson.CodecRequest
	aliases map[string]string
}

func (m *MapperCodecRequest) Method() (string, error) {
	raw, err := m.CodecRequest.Method()
	if err != nil {
		return "", err
	}

	alias, ok := m.aliases[raw]
	if ok {
		return alias, nil
	}
	return raw, nil
}
