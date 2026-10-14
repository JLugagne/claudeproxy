package jsonscan_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/JLugagne/claudeproxy/internal/pkg/jsonscan"
)

func TestTopLevelString(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{name: "model first", input: `{"model":"claude-opus-5-5","messages":[]}`, want: "claude-opus-5-5"},
		{name: "model after nested values", input: `{"messages":[{"role":"user","content":"a } ] { [ \" model"}],"max_tokens":1024,"stream":true,"x":null,"model":"deepseek/deepseek-v4.1-flash"}`, want: "deepseek/deepseek-v4.1-flash"},
		{name: "whitespace everywhere", input: " \n{ \"a\" : { \"model\" : \"nested\" } ,\t\"model\"\r\n:\n \"m\" } ", want: "m"},
		{name: "nested model is ignored", input: `{"metadata":{"model":"nested"}}`, wantErr: jsonscan.ErrNotFound},
		{name: "escaped backslash before quote", input: `{"system":"path C:\\","model":"m"}`, want: "m"},
		{name: "escaped value", input: `{"model":"a\"b\u00e9"}`, want: "a\"bé"},
		{name: "first duplicate wins", input: `{"model":"first","model":"second"}`, want: "first"},
		{name: "empty object", input: `{}`, wantErr: jsonscan.ErrNotFound},
		{name: "missing key", input: `{"messages":[1,2,3],"n":-1.5e3,"b":false}`, wantErr: jsonscan.ErrNotFound},
		{name: "not a string", input: `{"model":42}`, wantErr: jsonscan.ErrNotString},
		{name: "not an object", input: `["model","m"]`, wantErr: jsonscan.ErrMalformed},
		{name: "empty input", input: ``, wantErr: jsonscan.ErrMalformed},
		{name: "truncated string", input: `{"messages":"abc`, wantErr: jsonscan.ErrMalformed},
		{name: "truncated container", input: `{"messages":[{"a":1}`, wantErr: jsonscan.ErrMalformed},
		{name: "truncated after colon", input: `{"model":`, wantErr: jsonscan.ErrMalformed},
		{name: "missing colon", input: `{"model" "m"}`, wantErr: jsonscan.ErrMalformed},
		{name: "unterminated object", input: `{"a":1`, wantErr: jsonscan.ErrMalformed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := jsonscan.TopLevelString([]byte(tt.input), "model")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if got.Value != tt.want {
				t.Fatalf("value = %q, want %q", got.Value, tt.want)
			}
			var decoded string
			if err := json.Unmarshal([]byte(tt.input[got.Start:got.End]), &decoded); err != nil || decoded != tt.want {
				t.Fatalf("span %q does not decode to %q: %v", tt.input[got.Start:got.End], tt.want, err)
			}
		})
	}
}

func TestFieldReplace(t *testing.T) {
	input := []byte(`{"messages":[{"role":"user","content":"hi"}],"model":"claude-sonnet-5-5","stream":true}`)
	original := string(input)
	field, err := jsonscan.TopLevelString(input, "model")
	if err != nil {
		t.Fatal(err)
	}

	got := field.Replace(input, `deepseek/"v4"`)

	want := `{"messages":[{"role":"user","content":"hi"}],"model":"deepseek/\"v4\"","stream":true}`
	if string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	if string(input) != original {
		t.Fatalf("input was mutated: %s", input)
	}
}

func BenchmarkTopLevelStringLargeBodyModelLast(b *testing.B) {
	body := []byte(`{"messages":[{"role":"user","content":"` + strings.Repeat(`lorem \"ipsum\" {[ `, 50_000) + `"}],"model":"claude-opus-5-5"}`)
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := jsonscan.TopLevelString(body, "model"); err != nil {
			b.Fatal(err)
		}
	}
}

func FuzzTopLevelString(f *testing.F) {
	f.Add([]byte(`{"model":"claude-opus-5-5","messages":[]}`))
	f.Add([]byte(`{"messages":[{"content":"\\\"}"}],"model":"m"}`))
	f.Add([]byte(`{"a":{"model":"x"},"model":"\u0041"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		field, err := jsonscan.TopLevelString(data, "model")
		if err != nil {
			return
		}
		var decoded string
		if err := json.Unmarshal(data[field.Start:field.End], &decoded); err != nil {
			t.Fatalf("span %q is not a JSON string: %v", data[field.Start:field.End], err)
		}
		if decoded != field.Value {
			t.Fatalf("value %q, span decodes to %q", field.Value, decoded)
		}
	})
}
