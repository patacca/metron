package canon

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCanonicalize(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{
			// RFC 8785 section 3.2.2 example, without the "numbers" member,
			// which holds non-integers.
			name: "rfc8785 strings and literals",
			in: `{
  "string": "\u20ac$\u000F\u000aA'\u0042\u0022\u005c\\\"\/",
  "literals": [null, true, false]
}`,
			want: `{"literals":[null,true,false],"string":"€$\u000f\nA'B\"\\\\\"/"}`,
		},
		{
			// RFC 8785 section 3.2.3 sorting example.
			name: "rfc8785 member sorting",
			in: `{
  "\u20ac": "Euro Sign",
  "\r": "Carriage Return",
  "\ufb33": "Hebrew Letter Dalet With Dagesh",
  "1": "One",
  "\ud83d\ude00": "Emoji: Grinning Face",
  "\u0080": "Control",
  "\u00f6": "Latin Small Letter O With Diaeresis"
}`,
			want: "{\"\\r\":\"Carriage Return\",\"1\":\"One\",\"\u0080\":\"Control\"," +
				"\"\u00f6\":\"Latin Small Letter O With Diaeresis\",\"\u20ac\":\"Euro Sign\"," +
				"\"\U0001F600\":\"Emoji: Grinning Face\",\"\ufb33\":\"Hebrew Letter Dalet With Dagesh\"}",
		},
		// RFC 8785 appendix B, the integer rows.
		{name: "zero", in: `0`, want: `0`},
		{name: "minus zero", in: `-0`, want: `0`},
		{name: "max safe integer", in: `9007199254740991`, want: `9007199254740991`},
		{name: "min safe integer", in: `-9007199254740991`, want: `-9007199254740991`},
		{name: "keys differing only in case", in: `{"b":1,"a":2,"B":3,"A":4}`, want: `{"A":4,"B":3,"a":2,"b":1}`},
		{name: "prefix sorts first", in: `{"ab":1,"a":2,"":3}`, want: `{"":3,"a":2,"ab":1}`},
		{
			// U+10000 is D800 DC00 in UTF-16, which sorts before U+E000 and
			// U+FFFF although its code point is larger.
			name: "non-BMP before high BMP",
			in:   `{"\uffff":1,"\ud800\udc00":2,"\ue000":3,"\ud7ff":4}`,
			want: "{\"\ud7ff\":4,\"\U00010000\":2,\"\ue000\":3,\"\uffff\":1}",
		},
		{name: "nested objects sorted", in: `{"z":{"y":1,"x":[{"b":1,"a":2}]}}`, want: `{"z":{"x":[{"a":2,"b":1}],"y":1}}`},
		{name: "whitespace removed", in: " \t\r\n[ 1 , { } , [ ] ] \n", want: `[1,{},[]]`},
		{name: "control characters", in: `"\u0000\u001f\u007f\b\f\t"`, want: `"\u0000\u001f` + "\x7f" + `\b\f\t"`},
		{name: "no escaping of html or line separators", in: `"<&>\u2028\u2029"`, want: "\"<&>\u2028\u2029\""},
		{name: "solidus unescaped", in: `"\/"`, want: `"/"`},
		{name: "escaped non-ASCII emitted raw", in: `"\u00e9\u00E9"`, want: `"éé"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Canonicalize([]byte(tt.in))
			if err != nil {
				t.Fatalf("Canonicalize: %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("got  %s\nwant %s", got, tt.want)
			}
			again, err := Canonicalize(got)
			if err != nil || !bytes.Equal(again, got) {
				t.Fatalf("not idempotent: %s, %v", again, err)
			}
		})
	}
}

func TestParseRejects(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"rfc8785 numbers", `{"numbers":[333333333.33333329,1E30,4.50,2e-3]}`, "fraction or exponent"},
		{"fraction", `1.0`, "fraction or exponent"},
		{"exponent", `1e2`, "fraction or exponent"},
		{"too large", `9007199254740992`, "outside the range"},
		{"too small", `-9007199254740992`, "outside the range"},
		{"beyond int64", `123456789012345678901234567890`, "outside the range"},
		{"leading zero", `01`, "leading zero"},
		{"lone minus", `-`, "expected a digit"},
		{"plus sign", `+1`, "unexpected character"},
		{"duplicate member", `{"a":1,"a":2}`, `duplicate member "a"`},
		{"duplicate after unescaping", `{"a":1,"\u0061":2}`, `duplicate member "a"`},
		{"nested duplicate", `[{"x":{"k":1,"k":1}}]`, `duplicate member "k"`},
		{"trailing data", `{} {}`, "after the JSON value"},
		{"trailing garbage", `1x`, "after the JSON value"},
		{"empty", ``, "unexpected end"},
		{"byte-order mark", "\ufeff{}", "byte-order mark is not allowed; remove it"},
		{"byte-order mark after whitespace", " \ufeff{}", "unexpected character"},
		{"invalid UTF-8 in string", "\"\xff\"", "invalid UTF-8"},
		{"overlong UTF-8", "\"\xc0\xaf\"", "invalid UTF-8"},
		{"encoded surrogate", "\"\xed\xa0\x80\"", "invalid UTF-8"},
		{"lone high surrogate", `"\ud800"`, "unpaired surrogate"},
		{"lone low surrogate", `"\udc00"`, "unpaired surrogate"},
		{"high then non-surrogate", `"\ud800\u0041"`, "unpaired surrogate"},
		{"raw control character", "\"a\nb\"", "must be escaped"},
		{"bad escape", `"\x41"`, "invalid escape"},
		{"short unicode escape", `"\u12"`, "truncated \\u escape"},
		{"non-hex unicode escape", `"\u12zz"`, "invalid \\u escape"},
		{"unterminated string", `"abc`, "unterminated string"},
		{"trailing comma in object", `{"a":1,}`, "member name"},
		{"trailing comma in array", `[1,]`, "unexpected character"},
		{"single quotes", `{'a':1}`, "member name"},
		{"bare word", `nul`, "invalid literal"},
		{"NaN", `NaN`, "unexpected character"},
		{"too deep", strings.Repeat("[", maxDepth+1) + strings.Repeat("]", maxDepth+1), "nesting deeper"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.in))
			var se *SyntaxError
			if !errors.As(err, &se) {
				t.Fatalf("Parse(%q) = %v, want a *SyntaxError", tt.in, err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Parse(%q) error %q does not contain %q", tt.in, err, tt.want)
			}
		})
	}
}

func TestSyntaxErrorPosition(t *testing.T) {
	_, err := Parse([]byte("{\n  \"a\": 1,\n  \"a\": 2\n}"))
	var se *SyntaxError
	if !errors.As(err, &se) {
		t.Fatalf("got %v", err)
	}
	if se.Line != 3 || se.Column != 3 {
		t.Fatalf("position = %d:%d, want 3:3", se.Line, se.Column)
	}
}

func TestEncode(t *testing.T) {
	v := map[string]any{"b": []any{int64(1), "x", true, nil}, "a": map[string]any{}}
	got, err := Encode(v)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"a":{},"b":[1,"x",true,null]}`; string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	for _, bad := range []any{
		1.5,
		int(1),
		int64(MaxInt + 1),
		"\xff",
		map[string]any{"\xff": true},
	} {
		if _, err := Encode(bad); err == nil {
			t.Errorf("Encode(%#v) succeeded, want an error", bad)
		}
	}
	cyclic := []any{nil}
	cyclic[0] = cyclic
	if _, err := Encode(cyclic); err == nil {
		t.Error("Encode of a cyclic value succeeded")
	}
}

func TestMarshal(t *testing.T) {
	type payload struct {
		Zeta  string `json:"zeta"`
		Alpha int    `json:"alpha"`
		HTML  string `json:"html"`
	}
	got, err := Marshal(payload{Zeta: "z", Alpha: 7, HTML: "<a&b>"})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"alpha":7,"html":"<a&b>","zeta":"z"}`; string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	if _, err := Marshal(struct{ F float64 }{1.5}); err == nil {
		t.Fatal("Marshal of a non-integer succeeded")
	}
}

func TestDigest(t *testing.T) {
	sum := sha256.Sum256([]byte("abc"))
	want := "sha256:" + hex.EncodeToString(sum[:])
	if got := Digest([]byte("abc")); got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	if want != "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatal("unexpected SHA-256 test vector")
	}
}

func FuzzCanonicalize(f *testing.F) {
	for _, seed := range []string{
		`{"b":1,"a":[true,false,null,"x\u0000"]}`,
		`{"\ud83d\ude00":1,"\ufb33":2,"\u20ac":3}`,
		`[-0, 9007199254740991, -9007199254740991]`,
		`{"a":{"a":{"a":[]}}}`,
		`"\ud800\udc00\\\/\b\f\n\r\t"`,
		`{"a":1,"a":2}`,
		`1.5`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		out, err := Canonicalize(data)
		if err != nil {
			return
		}
		if !json.Valid(out) {
			t.Fatalf("canonical form is not valid JSON: %q", out)
		}
		again, err := Canonicalize(out)
		if err != nil {
			t.Fatalf("canonical form %q does not parse: %v", out, err)
		}
		if !bytes.Equal(again, out) {
			t.Fatalf("not idempotent:\n%q\n%q", out, again)
		}
	})
}
