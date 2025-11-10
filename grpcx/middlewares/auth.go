package middlewares

import (
	"context"
	"net/http"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/xucx/gox/grpcx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type AuthCeckerInfo struct {
	Path        string
	Headers     map[string][]string
	GrpcUnary   *grpc.UnaryServerInfo
	GrpcStream  *grpc.StreamServerInfo
	HttpRequest *http.Request
}

type AuthChecker func(info *AuthCeckerInfo) (any, bool)

type AuthInfoCtxKeyType struct{}

var AuthInfoCtxKey = AuthInfoCtxKeyType{}

var _ grpcx.Middleware = (*Auth)(nil)

type Auth struct {
	checker AuthChecker
}

func NewAuth(checker AuthChecker) *Auth {

	return &Auth{
		checker: checker,
	}
}

func (a *Auth) GrpcUnary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if a.checker != nil {
			checkInfo := &AuthCeckerInfo{
				Path:      info.FullMethod,
				Headers:   getGrcpHeaders(ctx),
				GrpcUnary: info,
			}
			authInfo, ok := a.checker(checkInfo)
			if !ok {
				return nil, status.Error(codes.Unauthenticated, "Auth fail")
			} else {
				if authInfo != nil {
					return handler(context.WithValue(ctx, AuthInfoCtxKey, authInfo), req)
				}

			}
		}

		return handler(ctx, req)
	}
}

func (a *Auth) GrpcStream() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		if a.checker != nil {
			checkInfo := &AuthCeckerInfo{
				Path:       info.FullMethod,
				Headers:    getGrcpHeaders(ctx),
				GrpcStream: info,
			}
			authInfo, ok := a.checker(checkInfo)
			if !ok {
				return status.Error(codes.Unauthenticated, "Auth fail")
			} else {
				if authInfo != nil {
					nextStream := grpc_middleware.WrapServerStream(ss)
					nextStream.WrappedContext = context.WithValue(ctx, AuthInfoCtxKey, authInfo)
					return handler(srv, nextStream)
				}
			}
		}

		return handler(srv, ss)
	}
}

func (a *Auth) Http() grpcx.MiddlewareHttpFunc {
	return func(next grpcx.MiddlewareHttpHandlerFunc) grpcx.MiddlewareHttpHandlerFunc {
		return func(h *grpcx.HttpHandlerContext) {
			if a.checker != nil {
				checkInfo := &AuthCeckerInfo{
					Path:        h.Request.URL.Path,
					Headers:     h.Request.Header,
					HttpRequest: h.Request,
				}

				authInfo, ok := a.checker(checkInfo)
				if !ok {
					h.Response.WriteHeader(http.StatusUnauthorized)
					return
				}

				if authInfo != nil {
					h.Context = context.WithValue(h.Context, AuthInfoCtxKey, authInfo)
				}
			}

			next(h)
		}
	}
}

func getGrcpHeaders(ctx context.Context) map[string][]string {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		return md
	}
	return map[string][]string{}
}
