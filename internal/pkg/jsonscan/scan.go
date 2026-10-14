package jsonscan

import (
	"bytes"
	"encoding/json"
	"errors"
)

// Field is a located top-level string member. Start and End delimit the raw
// JSON value in the scanned buffer, quotes included, so the value can be
// spliced out without re-encoding the rest of the document.
type Field struct {
	Value string
	Start int
	End   int
}

// TopLevelString returns the first top-level member named key, which must be a
// JSON string. Keys are compared on their raw bytes, so an escaped key never
// matches. Nested objects and arrays are skipped without being decoded.
// It returns ErrNotFound, ErrNotString or ErrMalformed.
func TopLevelString(data []byte, key string) (Field, error) {
	i := skipSpace(data, 0)
	if i >= len(data) || data[i] != '{' {
		return Field{}, ErrMalformed
	}
	i = skipSpace(data, i+1)
	if i < len(data) && data[i] == '}' {
		return Field{}, ErrNotFound
	}
	for {
		if i >= len(data) || data[i] != '"' {
			return Field{}, ErrMalformed
		}
		keyEnd, err := skipString(data, i)
		if err != nil {
			return Field{}, err
		}
		matched := string(data[i+1:keyEnd-1]) == key
		i = skipSpace(data, keyEnd)
		if i >= len(data) || data[i] != ':' {
			return Field{}, ErrMalformed
		}
		i = skipSpace(data, i+1)
		if i >= len(data) {
			return Field{}, ErrMalformed
		}
		if matched {
			return readString(data, i)
		}
		end, err := skipValue(data, i)
		if err != nil {
			return Field{}, err
		}
		i = skipSpace(data, end)
		if i >= len(data) {
			return Field{}, ErrMalformed
		}
		switch data[i] {
		case ',':
			i = skipSpace(data, i+1)
		case '}':
			return Field{}, ErrNotFound
		default:
			return Field{}, ErrMalformed
		}
	}
}

// Replace returns a new buffer where the field's value is replaced by value
// encoded as a JSON string. data must be the buffer the field was located in;
// it is left untouched.
func (f Field) Replace(data []byte, value string) []byte {
	encoded, _ := json.Marshal(value)
	out := make([]byte, 0, len(data)-(f.End-f.Start)+len(encoded))
	out = append(out, data[:f.Start]...)
	out = append(out, encoded...)
	return append(out, data[f.End:]...)
}

func readString(data []byte, i int) (Field, error) {
	if data[i] != '"' {
		return Field{}, ErrNotString
	}
	end, err := skipString(data, i)
	if err != nil {
		return Field{}, err
	}
	raw := data[i+1 : end-1]
	if isPlainASCII(raw) {
		return Field{Value: string(raw), Start: i, End: end}, nil
	}
	var value string
	if err := json.Unmarshal(data[i:end], &value); err != nil {
		return Field{}, errors.Join(ErrMalformed, err)
	}
	return Field{Value: value, Start: i, End: end}, nil
}

func isPlainASCII(raw []byte) bool {
	for _, c := range raw {
		if c < 0x20 || c >= 0x7f || c == '\\' {
			return false
		}
	}
	return true
}

func skipSpace(data []byte, i int) int {
	for i < len(data) {
		switch data[i] {
		case ' ', '\t', '\n', '\r':
			i++
		default:
			return i
		}
	}
	return i
}

func skipString(data []byte, i int) (int, error) {
	j := i + 1
	for {
		k := bytes.IndexByte(data[j:], '"')
		if k < 0 {
			return 0, ErrMalformed
		}
		quote := j + k
		backslashes := 0
		for b := quote - 1; b > i && data[b] == '\\'; b-- {
			backslashes++
		}
		if backslashes%2 == 0 {
			return quote + 1, nil
		}
		j = quote + 1
	}
}

func skipValue(data []byte, i int) (int, error) {
	switch data[i] {
	case '"':
		return skipString(data, i)
	case '{', '[':
		return skipContainer(data, i)
	default:
		return skipScalar(data, i)
	}
}

func skipContainer(data []byte, i int) (int, error) {
	depth := 0
	for j := i; j < len(data); {
		switch data[j] {
		case '"':
			end, err := skipString(data, j)
			if err != nil {
				return 0, err
			}
			j = end
			continue
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				return j + 1, nil
			}
		}
		j++
	}
	return 0, ErrMalformed
}

func skipScalar(data []byte, i int) (int, error) {
	for j := i; j < len(data); j++ {
		switch data[j] {
		case ',', '}', ']', ' ', '\t', '\n', '\r':
			if j == i {
				return 0, ErrMalformed
			}
			return j, nil
		}
	}
	return 0, ErrMalformed
}
