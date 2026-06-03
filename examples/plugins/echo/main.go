// Package main is a minimal cogniflow gRPC plugin that echoes its upstream data
// back as output. Use it for testing the plugin protocol end-to-end.
//
// Usage:
//
//	go run ./examples/plugins/echo --port 50051
//
// Then start the backend with:
//
//	PLUGIN_ADDRESSES=localhost:50051 go run ./cmd/server
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"

	"google.golang.org/grpc"

	pluginv1 "github.com/g8rswimmer/cogniflow/proto/plugin/v1"
)

var inputSchema = []byte(`{
  "type": "object",
  "properties": {},
  "additionalProperties": false
}`)

var outputSchema = []byte(`{
  "type": "object",
  "description": "Echoes the upstream_data received from the engine."
}`)

type echoPlugin struct {
	pluginv1.UnimplementedNodePluginServer
}

func (e *echoPlugin) Meta(_ context.Context, _ *pluginv1.MetaRequest) (*pluginv1.MetaResponse, error) {
	return &pluginv1.MetaResponse{
		TypeId:       "echo.passthrough",
		DisplayName:  "Echo",
		Category:     "plugin",
		Description:  "Returns the upstream data map unchanged. Useful for testing the gRPC plugin protocol.",
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	}, nil
}

func (e *echoPlugin) Execute(_ context.Context, req *pluginv1.ExecuteRequest) (*pluginv1.ExecuteResponse, error) {
	// Return the raw upstream_data bytes as the output JSON.
	data := req.GetUpstreamData()
	if len(data) == 0 {
		data = []byte(`{}`)
	}
	return &pluginv1.ExecuteResponse{
		Result: &pluginv1.ExecuteResponse_Data{Data: data},
	}, nil
}

func main() {
	port := flag.Int("port", 50051, "gRPC listen port")
	flag.Parse()

	addr := fmt.Sprintf(":%d", *port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen %s: %v", addr, err)
	}

	srv := grpc.NewServer()
	pluginv1.RegisterNodePluginServer(srv, &echoPlugin{})

	log.Printf("echo plugin listening on %s", addr)
	if err := srv.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
