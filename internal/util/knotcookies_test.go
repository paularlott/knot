package util

import (
	"net/http"
	"testing"
)

func TestStripKnotCookies(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   []string
		want []string
	}{
		{"none", nil, nil},
		{"only knot", []string{"__KNOT_WEBUI_SESSION=abc"}, nil},
		{"mixed", []string{"a=1; __KNOT_WEBUI_SESSION=abc; b=2"}, []string{"a=1; b=2"}},
		{"all knot cookies", []string{"__KNOT_OAUTH_STATE=s; __KNOT_OAUTH_PKCE=p; __KNOT_OAUTH_REDIRECT=r; x=y"}, []string{"x=y"}},
		{"several headers", []string{"a=1", "__KNOT_WEBUI_SESSION=abc", "b=2"}, []string{"a=1; b=2"}},
		{"no spaces", []string{"a=1;__KNOT_WEBUI_SESSION=abc;b=2"}, []string{"a=1; b=2"}},
		{"values kept verbatim", []string{`q="x y"; e==; __KNOT_WEBUI_SESSION=abc; flag`}, []string{`q="x y"; e==; flag`}},
		{"host-prefixed knot cookies", []string{"__Host-__KNOT_WEBUI_SESSION=abc; __Host-app=1; __Host-__KNOT_VNC=v"}, []string{"__Host-app=1"}},
		{"similar name kept", []string{"KNOT_WEBUI_SESSION=1; __knot_x=2"}, []string{"KNOT_WEBUI_SESSION=1; __knot_x=2"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := http.Header{}
			for _, v := range tt.in {
				h.Add("Cookie", v)
			}
			StripKnotCookies(h)
			got := h.Values("Cookie")
			if len(got) != len(tt.want) || (len(got) == 1 && got[0] != tt.want[0]) {
				t.Errorf("Cookie = %q, want %q", got, tt.want)
			}
		})
	}
}
