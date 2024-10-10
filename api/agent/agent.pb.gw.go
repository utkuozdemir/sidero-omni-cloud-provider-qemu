// Code generated by protoc-gen-grpc-gateway. DO NOT EDIT.
// source: agent/agent.proto

/*
Package agent is a reverse proxy.

It translates gRPC into RESTful JSON APIs.
*/
package agent

import (
	"context"
	"io"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/grpc-ecosystem/grpc-gateway/v2/utilities"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// Suppress "imported and not used" errors
var _ codes.Code
var _ io.Reader
var _ status.Status
var _ = runtime.String
var _ = utilities.NewDoubleArray
var _ = metadata.Join

func request_AgentService_SetIPMICredentials_0(ctx context.Context, marshaler runtime.Marshaler, client AgentServiceClient, req *http.Request, pathParams map[string]string) (proto.Message, runtime.ServerMetadata, error) {
	var protoReq SetIPMICredentialsRequest
	var metadata runtime.ServerMetadata

	if err := marshaler.NewDecoder(req.Body).Decode(&protoReq); err != nil && err != io.EOF {
		return nil, metadata, status.Errorf(codes.InvalidArgument, "%v", err)
	}

	msg, err := client.SetIPMICredentials(ctx, &protoReq, grpc.Header(&metadata.HeaderMD), grpc.Trailer(&metadata.TrailerMD))
	return msg, metadata, err

}

func local_request_AgentService_SetIPMICredentials_0(ctx context.Context, marshaler runtime.Marshaler, server AgentServiceServer, req *http.Request, pathParams map[string]string) (proto.Message, runtime.ServerMetadata, error) {
	var protoReq SetIPMICredentialsRequest
	var metadata runtime.ServerMetadata

	if err := marshaler.NewDecoder(req.Body).Decode(&protoReq); err != nil && err != io.EOF {
		return nil, metadata, status.Errorf(codes.InvalidArgument, "%v", err)
	}

	msg, err := server.SetIPMICredentials(ctx, &protoReq)
	return msg, metadata, err

}

// RegisterAgentServiceHandlerServer registers the http handlers for service AgentService to "mux".
// UnaryRPC     :call AgentServiceServer directly.
// StreamingRPC :currently unsupported pending https://github.com/grpc/grpc-go/issues/906.
// Note that using this registration option will cause many gRPC library features to stop working. Consider using RegisterAgentServiceHandlerFromEndpoint instead.
// GRPC interceptors will not work for this type of registration. To use interceptors, you must use the "runtime.WithMiddlewares" option in the "runtime.NewServeMux" call.
func RegisterAgentServiceHandlerServer(ctx context.Context, mux *runtime.ServeMux, server AgentServiceServer) error {

	mux.Handle("POST", pattern_AgentService_SetIPMICredentials_0, func(w http.ResponseWriter, req *http.Request, pathParams map[string]string) {
		ctx, cancel := context.WithCancel(req.Context())
		defer cancel()
		var stream runtime.ServerTransportStream
		ctx = grpc.NewContextWithServerTransportStream(ctx, &stream)
		inboundMarshaler, outboundMarshaler := runtime.MarshalerForRequest(mux, req)
		var err error
		var annotatedContext context.Context
		annotatedContext, err = runtime.AnnotateIncomingContext(ctx, mux, req, "/management.AgentService/SetIPMICredentials", runtime.WithHTTPPathPattern("/management.AgentService/SetIPMICredentials"))
		if err != nil {
			runtime.HTTPError(ctx, mux, outboundMarshaler, w, req, err)
			return
		}
		resp, md, err := local_request_AgentService_SetIPMICredentials_0(annotatedContext, inboundMarshaler, server, req, pathParams)
		md.HeaderMD, md.TrailerMD = metadata.Join(md.HeaderMD, stream.Header()), metadata.Join(md.TrailerMD, stream.Trailer())
		annotatedContext = runtime.NewServerMetadataContext(annotatedContext, md)
		if err != nil {
			runtime.HTTPError(annotatedContext, mux, outboundMarshaler, w, req, err)
			return
		}

		forward_AgentService_SetIPMICredentials_0(annotatedContext, mux, outboundMarshaler, w, req, resp, mux.GetForwardResponseOptions()...)

	})

	return nil
}

// RegisterAgentServiceHandlerFromEndpoint is same as RegisterAgentServiceHandler but
// automatically dials to "endpoint" and closes the connection when "ctx" gets done.
func RegisterAgentServiceHandlerFromEndpoint(ctx context.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) (err error) {
	conn, err := grpc.NewClient(endpoint, opts...)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if cerr := conn.Close(); cerr != nil {
				grpclog.Errorf("Failed to close conn to %s: %v", endpoint, cerr)
			}
			return
		}
		go func() {
			<-ctx.Done()
			if cerr := conn.Close(); cerr != nil {
				grpclog.Errorf("Failed to close conn to %s: %v", endpoint, cerr)
			}
		}()
	}()

	return RegisterAgentServiceHandler(ctx, mux, conn)
}

// RegisterAgentServiceHandler registers the http handlers for service AgentService to "mux".
// The handlers forward requests to the grpc endpoint over "conn".
func RegisterAgentServiceHandler(ctx context.Context, mux *runtime.ServeMux, conn *grpc.ClientConn) error {
	return RegisterAgentServiceHandlerClient(ctx, mux, NewAgentServiceClient(conn))
}

// RegisterAgentServiceHandlerClient registers the http handlers for service AgentService
// to "mux". The handlers forward requests to the grpc endpoint over the given implementation of "AgentServiceClient".
// Note: the gRPC framework executes interceptors within the gRPC handler. If the passed in "AgentServiceClient"
// doesn't go through the normal gRPC flow (creating a gRPC client etc.) then it will be up to the passed in
// "AgentServiceClient" to call the correct interceptors. This client ignores the HTTP middlewares.
func RegisterAgentServiceHandlerClient(ctx context.Context, mux *runtime.ServeMux, client AgentServiceClient) error {

	mux.Handle("POST", pattern_AgentService_SetIPMICredentials_0, func(w http.ResponseWriter, req *http.Request, pathParams map[string]string) {
		ctx, cancel := context.WithCancel(req.Context())
		defer cancel()
		inboundMarshaler, outboundMarshaler := runtime.MarshalerForRequest(mux, req)
		var err error
		var annotatedContext context.Context
		annotatedContext, err = runtime.AnnotateContext(ctx, mux, req, "/management.AgentService/SetIPMICredentials", runtime.WithHTTPPathPattern("/management.AgentService/SetIPMICredentials"))
		if err != nil {
			runtime.HTTPError(ctx, mux, outboundMarshaler, w, req, err)
			return
		}
		resp, md, err := request_AgentService_SetIPMICredentials_0(annotatedContext, inboundMarshaler, client, req, pathParams)
		annotatedContext = runtime.NewServerMetadataContext(annotatedContext, md)
		if err != nil {
			runtime.HTTPError(annotatedContext, mux, outboundMarshaler, w, req, err)
			return
		}

		forward_AgentService_SetIPMICredentials_0(annotatedContext, mux, outboundMarshaler, w, req, resp, mux.GetForwardResponseOptions()...)

	})

	return nil
}

var (
	pattern_AgentService_SetIPMICredentials_0 = runtime.MustPattern(runtime.NewPattern(1, []int{2, 0, 2, 1}, []string{"management.AgentService", "SetIPMICredentials"}, ""))
)

var (
	forward_AgentService_SetIPMICredentials_0 = runtime.ForwardResponseMessage
)
