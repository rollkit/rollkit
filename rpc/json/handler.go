package json

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"

	"github.com/gorilla/rpc/v2"
	"github.com/gorilla/rpc/v2/json2"
	"github.com/tendermint/tendermint/libs/bytes"
)

type handler struct {
	s *service
	m *http.ServeMux
	c rpc.Codec
}

func newHandler(s *service, codec rpc.Codec) *handler {
	mux := http.NewServeMux()
	h := &handler{
		m: mux,
		s: s,
		c: codec,
	}
	mux.HandleFunc("/", h.serveJSONRPC)
	for name, method := range s.methods {
		mux.HandleFunc("/"+name, h.newHandler(method))
	}
	return h
}
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.m.ServeHTTP(w, r)
}

// serveJSONRPC servces HTTP request
// implementation is highly inspired by Gorilla RPC v2 (but simplified a lot)
func (h *handler) serveJSONRPC(w http.ResponseWriter, r *http.Request) {
	// Create a new c request.
	codecReq := h.c.NewRequest(r)
	// Get service method to be called.
	method, errMethod := codecReq.Method()
	if errMethod != nil {
		if errors.Is(errMethod, io.EOF) && method == "" {
			// just serve empty page if request is empty
			return
		}
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

func (h *handler) newHandler(methodSpec *method) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		args := reflect.New(methodSpec.argsType)
		values, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			h.encodeAndWriteResponse(w, nil, err, http.StatusBadRequest)
			return
		}
		for i := 0; i < methodSpec.argsType.NumField(); i++ {
			field := methodSpec.argsType.Field(i)
			name := field.Tag.Get("json")
			rawVal := values.Get(name)
			var err error
			switch field.Type.Kind() {
			case reflect.Bool:
				err = setBoolParam(rawVal, &args, i)
			case reflect.Uint:
				err = setUintParam(rawVal, &args, i)
			case reflect.Int, reflect.Int64:
				err = setIntParam(rawVal, &args, i)
			case reflect.String:
				args.Elem().Field(i).SetString(rawVal)
			case reflect.Slice:
				// []byte is a reflect.Slice of reflect.Uint8's
				if field.Type.Elem().Kind() == reflect.Uint8 {
					err = setByteSliceParam(rawVal, &args, i)
				}
			default:
				err = errors.New("unknown type")
			}
			if err != nil {
				err = fmt.Errorf("failed to parse param '%s': %w", name, err)
				h.encodeAndWriteResponse(w, nil, err, http.StatusBadRequest)
				return
			}
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

		h.encodeAndWriteResponse(w, rets[0].Interface(), errResult, statusCode)
	}
}

func (h *handler) encodeAndWriteResponse(w http.ResponseWriter, result interface{}, errResult error, statusCode int) {
	// Prevents Internet Explorer from MIME-sniffing a response away
	// from the declared content-type
	w.Header().Set("x-content-type-options", "nosniff")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	resp := struct {
		Version string          `json:"jsonrpc"`
		Result  interface{}     `json:"result,omitempty"`
		Error   *json2.Error    `json:"error,omitempty"`
		Id      json.RawMessage `json:"id"`
	}{
		Version: "2.0",
		Id:      []byte("-1"),
	}

	if errResult != nil {
		resp.Error = &json2.Error{Code: json2.ErrorCode(statusCode), Data: errResult.Error()}
	} else {
		resp.Result = result
	}

	encoder := json.NewEncoder(w)
	err := encoder.Encode(resp)
	if err != nil {
		// TODO(tzdybal): log error
	}
}

func setBoolParam(rawVal string, args *reflect.Value, i int) error {
	v, err := strconv.ParseBool(rawVal)
	if err != nil {
		return err
	}
	args.Elem().Field(i).SetBool(v)
	return nil
}

func setIntParam(rawVal string, args *reflect.Value, i int) error {
	v, err := strconv.ParseInt(rawVal, 10, 64)
	if err != nil {
		return err
	}
	args.Elem().Field(i).SetInt(v)
	return nil
}

func setUintParam(rawVal string, args *reflect.Value, i int) error {
	v, err := strconv.ParseUint(rawVal, 10, 64)
	if err != nil {
		return err
	}
	args.Elem().Field(i).SetUint(v)
	return nil
}

func setByteSliceParam(rawVal string, args *reflect.Value, i int) error {
	var b bytes.HexBytes
	err := b.Unmarshal([]byte(rawVal))
	if err != nil {
		return err
	}
	args.Elem().Field(i).SetBytes(b)
	return nil
}
