package trial

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// ErrIdempotencyConflict is returned when a write carries an operation number
// that was already used with different content.
var ErrIdempotencyConflict = fmt.Errorf("idempotency conflict")

// RequestDigest computes a canonical, order-independent digest of a write
// request. Structurally equal requests produce the same digest regardless of
// slice ordering, so retries can be recognised as duplicates.
func RequestDigest(v any) (string, error) {
	canon, err := canonicalJSON(v)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(canon)
	return hex.EncodeToString(h[:]), nil
}

// canonicalJSON marshals v to JSON with every object key sorted so the result
// is deterministic for structurally equal values.
func canonicalJSON(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var anyVal any
	if err := json.Unmarshal(raw, &anyVal); err != nil {
		return nil, err
	}
	return json.Marshal(sortKeys(anyVal))
}

func sortKeys(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out[k] = sortKeys(t[k])
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = sortKeys(e)
		}
		return out
	default:
		return v
	}
}

// Operation carries the caller-supplied operation number and the canonical
// request digest used to deduplicate writes.
type Operation struct {
	OpNo   string
	Digest string
}

// IdempotencyRecord is the stored result of a completed write, keyed by the
// operation number so identical retries return the original result.
type IdempotencyRecord struct {
	OpNo       string
	Digest     string
	StatusCode int
	Response   []byte
}
