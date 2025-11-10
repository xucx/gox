package grpcx

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/soheilhy/cmux"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/encoding/protojson"
)

type ServerOptions struct {
	Addr                    string
	MaxRecvSize             int
	MaxSendSize             int
	Middlewares             []Middleware
	HttpUrlPrefix           string
	GatewayInHeaderMatcher  HeaderMatcherFunc
	GatewayOutHeaderMatcher HeaderMatcherFunc
}

type ServerOption func(*ServerOptions) *ServerOptions

func WithServerAddr(addr string) ServerOption {
	return func(so *ServerOptions) *ServerOptions {
		so.Addr = addr
		return so
	}
}
func WithServerMaxRecvSize(size int) ServerOption {
	return func(so *ServerOptions) *ServerOptions {
		so.MaxRecvSize = size
		return so
	}
}

func WithServerMaxSendSize(size int) ServerOption {
	return func(so *ServerOptions) *ServerOptions {
		so.MaxSendSize = size
		return so
	}
}

func WithServerMiddleware(m Middleware) ServerOption {
	return func(so *ServerOptions) *ServerOptions {
		if m != nil {
			so.Middlewares = append(so.Middlewares, m)
		}
		return so
	}
}

func WithServerHttpUrlPrefix(prefix string) ServerOption {
	return func(so *ServerOptions) *ServerOptions {
		so.HttpUrlPrefix = prefix
		return so
	}
}

func WithServerGatewayInHeaderMatcher(f HeaderMatcherFunc) ServerOption {
	return func(so *ServerOptions) *ServerOptions {
		so.GatewayInHeaderMatcher = f
		return so
	}
}

func WithServerGatewayOutHeaderMatcher(f HeaderMatcherFunc) ServerOption {
	return func(so *ServerOptions) *ServerOptions {
		so.GatewayOutHeaderMatcher = f
		return so
	}
}

type HeaderMatcherFunc func(string) (string, bool)

type HttpHandlerContext struct {
	Context  context.Context
	Response http.ResponseWriter
	Request  *http.Request
}

type HttpHandlerFunc func(ctx *HttpHandlerContext) bool

type Server struct {
	options *ServerOptions
	config  *ServerConfig
}

type ServerConfig struct {
	grpcServer       *grpc.Server
	gatewayServerMux *runtime.ServeMux
	gatewayConn      *grpc.ClientConn
	httpServer       *http.Server
	httpHandlers     []HttpHandlerFunc
	mux              cmux.CMux
}

func NewServer(opts ...ServerOption) (*Server, error) {
	//default
	options := &ServerOptions{
		Addr: ":8080",
	}
	for _, f := range opts {
		options = f(options)
	}

	var (
		config = &ServerConfig{}
		err    error
	)

	// crate grpc server
	unaryInterceptors := []grpc.UnaryServerInterceptor{}
	streamInterceptors := []grpc.StreamServerInterceptor{}
	httpInterceptors := []MiddlewareHttpFunc{}
	for _, m := range options.Middlewares {
		if v := m.GrpcUnary(); v != nil {
			unaryInterceptors = append(unaryInterceptors, v)
		}
		if v := m.GrpcStream(); v != nil {
			streamInterceptors = append(streamInterceptors, v)
		}
		if v := m.Http(); v != nil {
			httpInterceptors = append(httpInterceptors, v)
		}
	}

	grpcServerOptions := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(unaryInterceptors...),
		grpc.ChainStreamInterceptor(streamInterceptors...),
	}
	if options.MaxRecvSize > 0 {
		grpcServerOptions = append(grpcServerOptions, grpc.MaxRecvMsgSize(options.MaxRecvSize))
	}
	if options.MaxSendSize > 0 {
		grpcServerOptions = append(grpcServerOptions, grpc.MaxSendMsgSize(options.MaxRecvSize))

	}
	config.grpcServer = grpc.NewServer(grpcServerOptions...)
	reflection.Register(config.grpcServer)

	// create grpc gateway server
	gatewayServerOpts := []runtime.ServeMuxOption{
		runtime.WithMarshalerOption(
			runtime.MIMEWildcard,
			newGatewayMarshaler(),
		),
	}
	if options.GatewayInHeaderMatcher != nil {
		gatewayServerOpts = append(gatewayServerOpts, runtime.WithIncomingHeaderMatcher(func(key string) (string, bool) {
			if newKey, ok := runtime.DefaultHeaderMatcher(key); ok {
				return newKey, true
			}
			return options.GatewayInHeaderMatcher(key)
		}))
	}
	if options.GatewayOutHeaderMatcher != nil {
		gatewayServerOpts = append(gatewayServerOpts, runtime.WithOutgoingHeaderMatcher(func(key string) (string, bool) {
			if newKey, ok := runtime.DefaultHeaderMatcher(key); ok {
				return newKey, true
			}
			return options.GatewayOutHeaderMatcher(key)
		}))
	}
	config.gatewayServerMux = runtime.NewServeMux(gatewayServerOpts...)

	config.gatewayConn, err = grpc.NewClient(options.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	var (
		httpEntry MiddlewareHttpHandlerFunc = func(h *HttpHandlerContext) {
			for _, handler := range config.httpHandlers {
				if handler(h) {
					return
				}
			}

			h.Response.WriteHeader(http.StatusNotFound)
		}
	)

	for i := len(httpInterceptors) - 1; i >= 0; i-- {
		httpEntry = httpInterceptors[i](httpEntry)
	}

	var httpHandler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {

		if options.HttpUrlPrefix != "" && options.HttpUrlPrefix != "/" {
			if !strings.HasPrefix(r.URL.Path, options.HttpUrlPrefix) {
				w.WriteHeader(http.StatusNotFound)
				return
			}

			r.URL.Path = strings.TrimPrefix(r.URL.Path, options.HttpUrlPrefix)
			if !strings.HasPrefix(r.URL.Path, "/") {
				r.URL.Path = "/" + r.URL.Path
			}
		}

		if r.Header.Get("content-type") == "application/grpc" {
			config.gatewayServerMux.ServeHTTP(w, r)
			return
		}

		httpEntry(&HttpHandlerContext{
			Context:  context.Background(),
			Response: w,
			Request:  r,
		})
	}

	config.httpServer = &http.Server{
		Handler: httpHandler,
	}

	return &Server{
		options: options,
		config:  config,
	}, nil
}

func (s *Server) Serve() error {
	listener, err := net.Listen("tcp", s.options.Addr)
	if err != nil {
		return err
	}

	s.config.mux = cmux.New(listener)
	grpcListener := s.config.mux.MatchWithWriters(cmux.HTTP2MatchHeaderFieldSendSettings("content-type", "application/grpc"))
	httpListener := s.config.mux.Match(cmux.Any())

	go func() {
		s.config.grpcServer.Serve(grpcListener)
	}()

	go func() {
		s.config.httpServer.Serve(httpListener)
	}()

	return s.config.mux.Serve()
}

func (s *Server) Close() {
	s.config.grpcServer.GracefulStop()
	s.config.httpServer.Close()
	if s.config.mux != nil {
		s.config.mux.Close()
	}
}

func (s *Server) RegistGrpc(f func(grpcServer *grpc.Server, gatewayServerMux *runtime.ServeMux, gatewayConn *grpc.ClientConn)) {
	f(s.config.grpcServer, s.config.gatewayServerMux, s.config.gatewayConn)
}

func (s *Server) RegistHttp(handlers ...HttpHandlerFunc) {
	s.config.httpHandlers = append(s.config.httpHandlers, handlers...)
}

type gatewayMarshaler struct {
	*runtime.JSONPb
}

func (*gatewayMarshaler) StreamContentType(interface{}) string {
	return "text/event-stream"
}

func newGatewayMarshaler() *gatewayMarshaler {
	return &gatewayMarshaler{
		JSONPb: &runtime.JSONPb{
			MarshalOptions: protojson.MarshalOptions{
				EmitUnpopulated: false,
			},
			UnmarshalOptions: protojson.UnmarshalOptions{
				DiscardUnknown: true,
			},
		},
	}
}
