package middlewares

import (
	"context"
	"fmt"
	"net/http"
	"runtime"

	"github.com/xucx/gox/grpcx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type RecoveryHandler func(error, []byte)
type RecoveryConfig struct {
	StackSize int
	Handler   RecoveryHandler
}

var DefaultRecoveryConfig = RecoveryConfig{
	StackSize: 4 << 10,
}

var _ grpcx.Middleware = (*Recovery)(nil)

type Recovery struct {
	c RecoveryConfig
}

func NewRecoveryWithConfig(c RecoveryConfig) *Recovery {
	if c.StackSize == 0 {
		c.StackSize = DefaultRecoveryConfig.StackSize
	}

	return &Recovery{
		c: c,
	}
}

func NewRecovery() *Recovery {
	return &Recovery{
		c: DefaultRecoveryConfig,
	}
}

func (r *Recovery) GrpcUnary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (_ any, err error) {
		defer func() {
			if pErr, _ := r.check(); pErr != nil {
				err = status.Errorf(codes.Internal, "Server error")
			}
		}()

		return handler(ctx, req)
	}
}

func (r *Recovery) GrpcStream() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if pErr, _ := r.check(); pErr != nil {
				err = status.Errorf(codes.Internal, "Server error")
			}
		}()

		return handler(srv, ss)
	}
}

func (r *Recovery) Http() grpcx.MiddlewareHttpFunc {
	return func(next grpcx.MiddlewareHttpHandlerFunc) grpcx.MiddlewareHttpHandlerFunc {
		return func(h *grpcx.HttpHandlerContext) {
			defer func() {
				if err, _ := r.check(); err != nil {
					h.Response.WriteHeader(http.StatusInternalServerError)
				}
			}()

			next(h)
		}
	}
}

func (r *Recovery) check() ([]byte, error) {
	if p := recover(); p != nil {
		err, ok := p.(error)
		if !ok {
			err = fmt.Errorf("%v", p)
		}

		var stack []byte
		var length int
		if r.c.StackSize > 0 {
			stack = make([]byte, r.c.StackSize)
			length = runtime.Stack(stack, false)
			stack = stack[:length]
		}

		if r.c.Handler != nil {
			r.c.Handler(err, stack)
		}

		return stack, err
	}

	return nil, nil
}
