package values

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// JSONLimits bound allocation before materializing a public envelope, artifact
// or payload. They are concrete wire-profile limits, not domain validation.
type JSONLimits struct{ Bytes, Depth, Nodes, ContainerEntries, StringBytes int }

func PublicJSONLimits() JSONLimits { return JSONLimits{4 << 20, 64, 65536, 4096, 2 << 20} }

// ParseCanonicalJSON validates exact RFC 8785 bytes; it never repairs a Put.
func ParseCanonicalJSON(raw []byte, limits JSONLimits) (any, error) {
	value, err := ParsePublicJSON(raw, limits)
	if err != nil {
		return nil, err
	}
	canonical, err := CanonicalJSONValue(value)
	if err != nil || !bytes.Equal(raw, canonical) {
		return nil, PublicInvalid
	}
	return value, nil
}

func ParsePublicJSON(raw []byte, limits JSONLimits) (any, error) {
	if limits.Bytes <= 0 || limits.Depth <= 0 || limits.Nodes <= 0 || limits.ContainerEntries <= 0 || limits.StringBytes <= 0 || len(raw) > limits.Bytes || !utf8.Valid(raw) || !validJSONSurrogates(raw) {
		return nil, PublicInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	nodes := 0
	value, err := parseJSONValue(decoder, limits, 1, &nodes)
	if err != nil {
		return nil, PublicInvalid
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, PublicInvalid
	}
	return value, nil
}

func parseJSONValue(decoder *json.Decoder, limits JSONLimits, depth int, nodes *int) (any, error) {
	*nodes++
	if depth > limits.Depth || *nodes > limits.Nodes {
		return nil, PublicInvalid
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			object := make(map[string]any)
			for decoder.More() {
				if len(object) >= limits.ContainerEntries {
					return nil, PublicInvalid
				}
				token, err := decoder.Token()
				if err != nil {
					return nil, err
				}
				key, ok := token.(string)
				if !ok || len(key) > limits.StringBytes {
					return nil, PublicInvalid
				}
				if _, exists := object[key]; exists {
					return nil, PublicInvalid
				}
				child, err := parseJSONValue(decoder, limits, depth+1, nodes)
				if err != nil {
					return nil, err
				}
				object[key] = child
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim('}') {
				return nil, PublicInvalid
			}
			return object, nil
		case '[':
			array := make([]any, 0)
			for decoder.More() {
				if len(array) >= limits.ContainerEntries {
					return nil, PublicInvalid
				}
				child, err := parseJSONValue(decoder, limits, depth+1, nodes)
				if err != nil {
					return nil, err
				}
				array = append(array, child)
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim(']') {
				return nil, PublicInvalid
			}
			return array, nil
		default:
			return nil, PublicInvalid
		}
	case json.Number:
		number, err := strconv.ParseFloat(string(value), 64)
		if err != nil || math.IsInf(number, 0) || math.IsNaN(number) {
			return nil, PublicInvalid
		}
		return number, nil
	case string:
		if len(value) > limits.StringBytes {
			return nil, PublicInvalid
		}
		return value, nil
	case bool, nil:
		return value, nil
	default:
		return nil, PublicInvalid
	}
}

// CanonicalJSONValue accepts only the parsed JSON value algebra. Callers cannot
// serialize private owner structs by accident through reflection.
func CanonicalJSONValue(value any) ([]byte, error) { return appendCanonicalJSON(nil, value, 1) }

func appendCanonicalJSON(out []byte, value any, depth int) ([]byte, error) {
	if depth > 64 {
		return nil, PublicInvalid
	}
	switch value := value.(type) {
	case nil:
		return append(out, "null"...), nil
	case bool:
		return strconv.AppendBool(out, value), nil
	case string:
		return appendJSONString(out, value)
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, PublicInvalid
		}
		if value == 0 {
			return append(out, '0'), nil
		}
		format := byte('e')
		if math.Abs(value) >= 1e-6 && math.Abs(value) < 1e21 {
			format = 'f'
		}
		number := strconv.FormatFloat(value, format, -1, 64)
		if i := strings.IndexByte(number, 'e'); i >= 0 {
			exp, err := strconv.Atoi(number[i+1:])
			if err != nil {
				return nil, PublicInvalid
			}
			sign := ""
			if exp >= 0 {
				sign = "+"
			}
			number = number[:i+1] + sign + strconv.Itoa(exp)
		}
		return append(out, number...), nil
	case []any:
		out = append(out, '[')
		for i, item := range value {
			if i > 0 {
				out = append(out, ',')
			}
			var err error
			out, err = appendCanonicalJSON(out, item, depth+1)
			if err != nil {
				return nil, err
			}
		}
		return append(out, ']'), nil
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			if !utf8.ValidString(key) {
				return nil, PublicInvalid
			}
			keys = append(keys, key)
		}
		slices.SortFunc(keys, func(a, b string) int { return slices.Compare(utf16.Encode([]rune(a)), utf16.Encode([]rune(b))) })
		out = append(out, '{')
		for i, key := range keys {
			if i > 0 {
				out = append(out, ',')
			}
			var err error
			out, err = appendJSONString(out, key)
			if err != nil {
				return nil, err
			}
			out = append(out, ':')
			out, err = appendCanonicalJSON(out, value[key], depth+1)
			if err != nil {
				return nil, err
			}
		}
		return append(out, '}'), nil
	default:
		return nil, PublicInvalid
	}
}

func appendJSONString(out []byte, value string) ([]byte, error) {
	if !utf8.ValidString(value) {
		return nil, PublicInvalid
	}
	out = append(out, '"')
	const hex = "0123456789abcdef"
	for _, r := range value {
		switch r {
		case '"', '\\':
			out = append(out, '\\', byte(r))
		case '\b':
			out = append(out, '\\', 'b')
		case '\t':
			out = append(out, '\\', 't')
		case '\n':
			out = append(out, '\\', 'n')
		case '\f':
			out = append(out, '\\', 'f')
		case '\r':
			out = append(out, '\\', 'r')
		default:
			if r < 0x20 {
				out = append(out, '\\', 'u', '0', '0', hex[r>>4], hex[r&15])
			} else {
				out = utf8.AppendRune(out, r)
			}
		}
	}
	return append(out, '"'), nil
}

// encoding/json replaces lone surrogate escapes with U+FFFD. Reject those
// before decoding, while allowing an actual U+FFFD or a valid surrogate pair.
func validJSONSurrogates(raw []byte) bool {
	inString := false
	for i := 0; i < len(raw); i++ {
		if raw[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) {
			return false
		}
		if raw[i] != 'u' {
			continue
		}
		if i+4 >= len(raw) {
			return false
		}
		unit, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if unit >= 0xdc00 && unit <= 0xdfff {
			return false
		}
		if unit < 0xd800 || unit > 0xdbff {
			continue
		}
		if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
			return false
		}
		low, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
		if err != nil || low < 0xdc00 || low > 0xdfff {
			return false
		}
		i += 6
	}
	return !inString
}
