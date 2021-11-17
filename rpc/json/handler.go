package json

import (
	"github.com/gorilla/rpc/v2"
	"net/http"
	"reflect"
)

type handler struct {
	s *service
	c rpc.Codec
}

func newHandler(s *service, codec rpc.Codec) *handler {
	return &handler{
		s: s,
		c: codec,
	}
}

// ServeHTTP servces HTTP request
// implementation is highly inspired by Gorilla RPC v2 (but simplified a lot)
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		rpc.WriteError(w, http.StatusMethodNotAllowed, "rpc: POST method required, received "+r.Method)
		return
	}
	// Create a new c request.
	codecReq := h.c.NewRequest(r)
	// Get service method to be called.
	method, errMethod := codecReq.Method()
	if errMethod != nil {
		codecReq.WriteError(w, http.StatusBadRequest, errMethod)
		return
	}
	methodSpec, ok := h.s.methods[method]
	if !ok {
		codecReq.WriteError(w, http.StatusBadRequest, errMethod)
		return
	}

	// Decode the args.
	args := reflect.New(methodSpec.argsType)
	if errRead := codecReq.ReadRequest(args.Interface()); errRead != nil {
		codecReq.WriteError(w, http.StatusBadRequest, errRead)
		return
	}

	rets := methodSpec.m.Call([]reflect.Value{
		reflect.ValueOf(r),
		args,
	})

	// Extract the result to error if needed.
	var errResult error
	statusCode := http.StatusOK
	errInter := rets[1].Interface()
	if errInter != nil {
		statusCode = http.StatusBadRequest
		errResult = errInter.(error)
	}

	// Prevents Internet Explorer from MIME-sniffing a response away
	// from the declared content-type
	w.Header().Set("x-content-type-options", "nosniff")

	// Encode the response.
	if errResult == nil {
		codecReq.WriteResponse(w, rets[0].Interface())
	} else {
		codecReq.WriteError(w, statusCode, errResult)
	}
}
