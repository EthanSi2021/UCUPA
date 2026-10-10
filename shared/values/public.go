package values

import (
	"crypto/rand"
	"encoding/base64"
	"strconv"
	"strings"
	"time"
)

type PublicError string

func (err PublicError) Error() string { return string(err) }

const (
	PublicInvalid           PublicError = "INVALID"
	PublicDenied            PublicError = "DENIED"
	PublicUnavailable       PublicError = "UNAVAILABLE"
	PublicNotFound          PublicError = "NOT_FOUND"
	PublicGap               PublicError = "GAP"
	PublicInvalidContext    PublicError = "INVALID_CONTEXT"
	PublicResourceExhausted PublicError = "RESOURCE_EXHAUSTED"
	PublicConflict          PublicError = "CONFLICT"
	PublicIndeterminate     PublicError = "INDETERMINATE"
	PublicOutcomeUnknown    PublicError = "OUTCOME_UNKNOWN"
	PublicUnsupportedType   PublicError = "UNSUPPORTED_TYPE"
)

// PublicAccessPolicy is copied at startup. Nil disables public contexts, not
// owner-local fact operations. Count pointers distinguish omission from zero.
type PublicAccessPolicy struct {
	ContextTTL               time.Duration
	ObservationTTL           time.Duration
	MaxContexts              *uint64
	MaxContextsPerCaller     *uint64
	MaxGroupsPerObservation  *uint64
	MaxPublicBytes           *uint64
	MaxConcurrentPublicReads *uint64
}

type PublicLimits struct {
	ContextTTL      time.Duration
	ObservationTTL  time.Duration
	Contexts        uint64
	PerCaller       uint64
	Groups          uint64
	Bytes           uint64
	ConcurrentReads uint64
}

func NormalizePublicPolicy(policy *PublicAccessPolicy) (PublicLimits, error) {
	if policy == nil {
		return PublicLimits{}, nil
	}
	limits := PublicLimits{ContextTTL: policy.ContextTTL, ObservationTTL: policy.ObservationTTL}
	if limits.ContextTTL <= 0 || limits.ObservationTTL <= 0 {
		return PublicLimits{}, PublicInvalid
	}
	inputs := []*uint64{policy.MaxContexts, policy.MaxContextsPerCaller, policy.MaxGroupsPerObservation, policy.MaxPublicBytes, policy.MaxConcurrentPublicReads}
	outputs := []*uint64{&limits.Contexts, &limits.PerCaller, &limits.Groups, &limits.Bytes, &limits.ConcurrentReads}
	defaults := []uint64{128, 32, 1024, 64 << 20, 64}
	for i, input := range inputs {
		*outputs[i] = defaults[i]
		if input != nil {
			if *input == 0 || *input > 1<<53-1 {
				return PublicLimits{}, PublicInvalid
			}
			*outputs[i] = *input
		}
	}
	if limits.PerCaller > limits.Contexts {
		return PublicLimits{}, PublicInvalid
	}
	return limits, nil
}

// PublicToken deliberately contains no fact sequence, authority directory or
// authorization proof. Its value is either random or an owner-produced MAC.
type PublicToken struct {
	Owner FactAuthorityRef `json:"owner"`
	Kind  string           `json:"kind"`
	Value string           `json:"value"`
}

func NewPublicToken(owner FactAuthorityRef, kind string) (PublicToken, error) {
	var raw [32]byte
	_, _ = rand.Read(raw[:])
	return PublicTokenFromBytes(owner, kind, raw)
}

func PublicTokenFromBytes(owner FactAuthorityRef, kind string, raw [32]byte) (PublicToken, error) {
	token := PublicToken{Owner: owner, Kind: kind, Value: EncodePublicBytes(raw[:])}
	if !token.Valid() {
		return PublicToken{}, PublicInvalid
	}
	return token, nil
}

func (token PublicToken) Valid() bool {
	if !token.Owner.Valid() {
		return false
	}
	switch token.Kind {
	case "space-handle", "record-revision", "mount-revision", "attachment-revision", "contact-revision", "transition-revision", "carrier-revision", "facet-replacement-revision", "read-receipt", "query-continuation", "observation-preparation", "observation-cursor", "change-id", "change-basis":
	default:
		return false
	}
	raw, err := DecodePublicBytes(token.Value)
	return err == nil && len(raw) == 32
}

func EncodePublicBytes(raw []byte) string { return base64.RawURLEncoding.EncodeToString(raw) }

func DecodePublicBytes(text string) ([]byte, error) {
	if strings.ContainsAny(text, "=\r\n \t") {
		return nil, PublicInvalid
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(text)
	if err != nil || EncodePublicBytes(decoded) != text {
		return nil, PublicInvalid
	}
	return decoded, nil
}

func ParsePublicUpDepth(text string) (int64, error) {
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil || value < -1 || strconv.FormatInt(value, 10) != text {
		return 0, PublicInvalid
	}
	return value, nil
}

func ParsePublicParentCount(text string) (uint64, error) {
	value, err := strconv.ParseUint(text, 10, 64)
	if err != nil || strconv.FormatUint(value, 10) != text {
		return 0, PublicInvalid
	}
	return value, nil
}
