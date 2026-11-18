package jobsim

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

type messageRetractionRequest struct {
	ExpectedVersion string `json:"expectedVersion"`
	RequestID       string `json:"requestId"`
}

func decodeRetractionRequest(r *http.Request) (messageRetractionRequest, error) {
	var q messageRetractionRequest
	if r.Body == nil || r.Body == http.NoBody {
		return q, nil
	}
	raw, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 1<<20))
	if err != nil {
		return q, fail(400, "INVALID_RETRACTION_REQUEST", "Invalid optional retraction body")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return q, nil
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err = d.Decode(&q); err != nil {
		return q, fail(400, "INVALID_RETRACTION_REQUEST", "Invalid optional retraction body")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return q, fail(400, "INVALID_RETRACTION_REQUEST", "One original retraction body required")
	}
	if q.ExpectedVersion != "" || q.RequestID != "" {
		if !syncValidationCounter(q.ExpectedVersion, false) || !validReceiptNonce(q.RequestID) {
			return q, fail(400, "INVALID_RETRACTION_REQUEST", "Exact decimal expectedVersion and stable requestId required")
		}
	}
	return q, nil
}
