package middlewares

import (
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/xucx/gox/grpcx"
)

type ProxyTarget struct {
	Url *url.URL
}

type ProxyChecker func(*http.Request) (*ProxyTarget, error)
type ProxyModifier func(*http.Response) error
type Proxy struct {
	grpcx.NopMiddleware
	checker  ProxyChecker
	modifier ProxyModifier
}

var ErrProxySkip = errors.New("proxy skip")

func NewProxy(checker ProxyChecker) *Proxy {
	return &Proxy{
		checker: checker,
	}
}

func (p *Proxy) Http() grpcx.MiddlewareHttpFunc {
	return func(next grpcx.MiddlewareHttpHandlerFunc) grpcx.MiddlewareHttpHandlerFunc {
		return func(c *grpcx.HttpHandlerContext) {
			target, err := p.checker(c.Request)
			if err != nil {
				if errors.Is(err, ErrProxySkip) {
					next(c)
				} else {
					c.Response.WriteHeader(http.StatusInternalServerError)
				}
				return
			}

			proxy := httputil.NewSingleHostReverseProxy(target.Url)
			if p.modifier != nil {
				proxy.ModifyResponse = p.modifier
			}

			proxy.ServeHTTP(c.Response, c.Request)

		}
	}
}
