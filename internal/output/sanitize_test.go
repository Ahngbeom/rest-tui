package output

import "testing"

func TestSanitize_StripsEscapeSequencesAndControls(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"csi and osc title", "hello \x1b[2J\x1b[H\x1b]0;PWNED-TITLE\x07\x1b[41mRED\x1b[0m end", "hello RED end"},
		{"osc8 with ST", "a\x1b]8;;http://x\x1b\\link\x1b]8;;\x1b\\b", "alinkb"},
		{"c0 except newline and tab", "tab\there\nnl\rcr\x00nul\x08bs", "tab\there\nnlcrnulbs"},
		{"crlf", "a\r\nb", "a\nb"},
		{"c1", "c1:\u009b31mX\u0085Y", "c1:31mXY"},
		{"wide text kept", "한글 \x1b[31m빨강\x1b[0m", "한글 빨강"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		if got := Sanitize(c.in); got != c.want {
			t.Errorf("%s: Sanitize(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestIsBinary(t *testing.T) {
	cases := []struct {
		name        string
		body        []byte
		contentType string
		want        bool
	}{
		{"png bytes", []byte("\x89PNG\r\n\x1a\n\x00\x00"), "image/png", true},
		{"image type alone", []byte("GIF89a"), "image/gif", true},
		{"svg is text", []byte("<svg/>"), "image/svg+xml", false},
		{"octet-stream", []byte("abc"), "application/octet-stream", true},
		{"json", []byte(`{"a":1}`), "application/json; charset=utf-8", false},
		{"nul in text/plain", []byte("a\x00b"), "text/plain", true},
		{"latin1 text/plain", []byte("caf\xe9"), "text/plain; charset=iso-8859-1", false},
		{"no type, invalid utf8", []byte{0xff, 0xfe, 0xfd}, "", true},
		{"no type, utf8 text", []byte("plain text"), "", false},
		{"empty body", nil, "image/png", false},
	}
	for _, c := range cases {
		if got := IsBinary(c.body, c.contentType); got != c.want {
			t.Errorf("%s: IsBinary = %v, want %v", c.name, got, c.want)
		}
	}
}
