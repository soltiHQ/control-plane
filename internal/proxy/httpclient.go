package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// httpClient is the subset of *http.Client used by the proxy helpers.
type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// maxErrorBodyBytes caps how much of a non-2xx response body is read back for
// diagnostic purposes. The SDK emits compact JSON error bodies (~ tens of bytes);
// 4 KiB leaves enough headroom for a stack-style message without letting a
// misbehaving agent balloon our log lines.
const maxErrorBodyBytes = 4 * 1024

// sdkErrorBody is the HTTP error envelope emitted by solti-api:
// {"error":"<label>","message":"<detail>"}. We tolerate missing/extra fields
// so a broken or older agent is still diagnosable.
type sdkErrorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// unexpectedStatusError carries the agent's HTTP status code so the transport
// layer (errkind) can recover the agent's semantics. Unwrap exposes
// ErrUnexpectedStatus, so existing errors.Is checks keep working.
type unexpectedStatusError struct {
	code int
	msg  string
}

func (e *unexpectedStatusError) Error() string { return e.msg }

func (e *unexpectedStatusError) Unwrap() error { return ErrUnexpectedStatus }

func (e *unexpectedStatusError) HTTPStatus() int { return e.code }

// formatUnexpectedStatus reads a bounded preview of the response body and
// returns an [unexpectedStatusError] that surfaces the SDK's structured
// {error,message} payload when present (or the raw snippet otherwise) and
// carries the agent's HTTP status code. Bytes consumed here are lost for
// further processing, so callers must only invoke this on the non-success branch.
func formatUnexpectedStatus(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	base := ErrUnexpectedStatus.Error()

	msg := fmt.Sprintf("%s: %d", base, resp.StatusCode)
	if len(body) > 0 {
		var e sdkErrorBody
		if err := json.Unmarshal(body, &e); err == nil && (e.Error != "" || e.Message != "") {
			switch {
			case e.Error != "" && e.Message != "":
				msg = fmt.Sprintf("%s: %d %s: %s", base, resp.StatusCode, e.Error, e.Message)
			case e.Error != "":
				msg = fmt.Sprintf("%s: %d %s", base, resp.StatusCode, e.Error)
			default:
				msg = fmt.Sprintf("%s: %d %s", base, resp.StatusCode, e.Message)
			}
		} else {
			msg = fmt.Sprintf("%s: %d: %s", base, resp.StatusCode, string(body))
		}
	}
	return &unexpectedStatusError{code: resp.StatusCode, msg: msg}
}

// doDelete performs a DELETE request [statuses: 200, 204].
func doDelete(ctx context.Context, client httpClient, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCreateRequest, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRequest, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent:
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	default:
		return formatUnexpectedStatus(resp)
	}
}

// doProtoJSONPutDecoding PUTs `in` as canonical proto-JSON and decodes the
// response body into `out`. Used for idempotent apply/upsert calls such as
// ApplyTask, where the response carries meaningful data (e.g. the TaskId).
func doProtoJSONPutDecoding(ctx context.Context, client httpClient, url string, in, out proto.Message) error {
	return doProtoJSONWriteDecoding(ctx, client, http.MethodPut, url, in, out)
}

// doProtoJSONWriteDecoding sends `in` as canonical proto-JSON with the given
// method and decodes the response body into `out`.
//
// On a non-2xx response, `formatUnexpectedStatus` pulls the SDK error
// envelope into the returned error so callers see the agent's reason
// verbatim. Any 2xx body is decoded with `DiscardUnknown: true` so the
// agent can add fields without breaking older control planes.
func doProtoJSONWriteDecoding(ctx context.Context, client httpClient, method, url string, in, out proto.Message) error {
	payload, err := protojson.MarshalOptions{
		UseProtoNames:   false,
		EmitUnpopulated: false,
	}.Marshal(in)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCreateRequest, err)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCreateRequest, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRequest, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		if resp.StatusCode == http.StatusNoContent {
			return nil
		}
		body, rerr := io.ReadAll(resp.Body)
		if rerr != nil {
			return fmt.Errorf("%w: %v", ErrDecode, rerr)
		}
		if len(body) == 0 {
			return nil
		}
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(body, out); err != nil {
			return fmt.Errorf("%w: %v", ErrDecode, err)
		}
		return nil
	default:
		return formatUnexpectedStatus(resp)
	}
}
