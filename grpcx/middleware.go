package grpcx

import (
	"google.golang.org/grpc"
)

type MiddlewareHttpHandlerFunc func(ctx *HttpHandlerContext)
type MiddlewareHttpFunc func(next MiddlewareHttpHandlerFunc) MiddlewareHttpHandlerFunc

type Middleware interface {
	GrpcUnary() grpc.UnaryServerInterceptor
	GrpcStream() grpc.StreamServerInterceptor
	Http() MiddlewareHttpFunc
}

type NopMiddleware struct {
}

func (m *NopMiddleware) GrpcUnary() grpc.UnaryServerInterceptor {
	return nil
}

func (m *NopMiddleware) GrpcStream() grpc.StreamServerInterceptor {
	return nil
}

func (m *NopMiddleware) Http() MiddlewareHttpFunc {
	return nil
}
