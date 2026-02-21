package localapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
)

type HTTPExecutor struct {
	h http.Handler
}

func NewHTTPExecutor(h http.Handler) *HTTPExecutor {
	return &HTTPExecutor{h: h}
}

func (e *HTTPExecutor) Execute(method, path string, headers map[string]string, body string) (int, map[string]string, string, error) {
	if method == "" {
		method = http.MethodGet
	}
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	e.h.ServeHTTP(rr, req)
	outHeaders := map[string]string{}
	for k, values := range rr.Header() {
		if len(values) > 0 {
			outHeaders[k] = values[0]
		}
	}
	return rr.Code, outHeaders, rr.Body.String(), nil
}
