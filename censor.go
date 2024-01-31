package klbslog

import (
	"net/http"
	"regexp"
)

var jwtTokenRegexp = regexp.MustCompile(`([a-zA-Z0-9_-]+)\.([a-zA-Z0-9_-]+)\.([a-zA-Z0-9_-]+)`)

func censorJwtTokens(data string) string {
	// find any base64url a.b.c string and remove c, replace with a *
	return jwtTokenRegexp.ReplaceAllString(data, "$1.$2.*")
}

func censorHeaders(h http.Header, hdrs ...string) http.Header {
	reallocated := false

	// replace Cookie in request & Set-Cookie in response if needed
	// this function supports adding more headers to censor
	for _, hdr := range hdrs {
		if sub, ok := h[hdr]; ok {
			if !reallocated {
				reallocated = true

				nh := make(http.Header, len(h))
				for n, s2 := range h {
					nh[n] = s2
				}
			}
			newsub := make([]string, len(sub))
			for n, v := range sub {
				newsub[n] = censorJwtTokens(v)
			}
			h[hdr] = newsub
		}
	}
	return h
}
