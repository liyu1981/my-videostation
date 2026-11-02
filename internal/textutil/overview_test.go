package textutil

import "testing"

func TestNormalizeOverview(t *testing.T) {
	cases := []struct{ in, want string }{
		{"a  \u3000\u3000b", "a\n\nb"},
		{"a\u3000\u3000b", "a\n\nb"},
		{"a    b", "a\n\nb"},
		{"a  b", "a b"},
		{"a\n\nb", "a\n\nb"},
		{"a\r\n\r\nb", "a\n\nb"},
		{"a<br>b", "a\nb"},
		{"a</p><p>b", "a\n\nb"},
		{"  a  ", "a"},
	}
	for _, c := range cases {
		if got := NormalizeOverview(c.in); got != c.want {
			t.Errorf("NormalizeOverview(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
