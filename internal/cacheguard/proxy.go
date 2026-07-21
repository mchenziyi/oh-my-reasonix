package cacheguard

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// NewReverseProxy creates a transparent proxy whose request body is restored
// byte-for-byte after recording. It intentionally does not rewrite models,
// tools, or message content.
func NewReverseProxy(upstream *url.URL, record func(raw, forwarded []byte)) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		var raw []byte
		if req.Body != nil {
			raw, _ = io.ReadAll(req.Body)
			req.Body = io.NopCloser(bytes.NewReader(raw))
			req.ContentLength = int64(len(raw))
		}
		originalDirector(req)
		if record != nil {
			record(raw, raw)
		}
	}
	return proxy
}
