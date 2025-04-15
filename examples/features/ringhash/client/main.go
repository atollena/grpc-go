/*
 * Copyright 2025 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

// Binary client demonstrates how to configure load balancing policies to
// use ring hash for distributing requests across backend servers.
package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	ecpb "google.golang.org/grpc/examples/features/proto/echo"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/ringhash"

	_ "google.golang.org/grpc/balancer/ringhash" // registers the ring hash balancer
)

const (
	exampleScheme      = "example"
	exampleServiceName = "lb.example.grpc.io"
)

var addrs = []string{
	"localhost:50051",
	"localhost:50052",
	"localhost:50053",
	"localhost:50054",

	// Those addresses have no server running. The requests that hash to those
	// addresses will be routed to the next available server.
	"localhost:50055",
	"localhost:50056",
}

func callUnaryEcho(c ecpb.EchoClient, requestID string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	msg := fmt.Sprintf("this is examples/ringhash with request-id %s", requestID)
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("request-id", requestID))
	r, err := c.UnaryEcho(ctx, &ecpb.EchoRequest{Message: msg})
	if err != nil {
		log.Fatalf("could not greet: %v", err)
	}
	fmt.Println(r.Message)
}

func makeRPCs(cc *grpc.ClientConn, n int, requestID string) {
	hwc := ecpb.NewEchoClient(cc)
	for range n {
		callUnaryEcho(hwc, requestID)
	}
}

func main() {
	var serviceConfig = `{"loadBalancingConfig": [{"ring_hash_experimental":{
		"minRingSize": 1024,
		"maxRingSize": 4096,
		"requestHashHeader": "request-id"
	}}]}`

	conn, err := grpc.NewClient(
		fmt.Sprintf("%s:///%s", exampleScheme, exampleServiceName),
		grpc.WithDefaultServiceConfig(serviceConfig),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("could not connect: %v", err)
	}
	defer conn.Close()

	for i := range len(addrs) {
		requestID := fmt.Sprintf("%d", i)
		fmt.Printf("--- calling helloworld.Greeter/SayHello with request-id %q ---\n", requestID)
		makeRPCs(conn, 3, requestID)
	}
}

// Following is an example name resolver implementation. Read the name
// resolution example to learn more about it.

type exampleResolverBuilder struct{}

func (*exampleResolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, _ resolver.BuildOptions) (resolver.Resolver, error) {
	r := &exampleResolver{
		target: target,
		cc:     cc,
		addrsStore: map[string][]string{
			exampleServiceName: addrs,
		},
	}
	r.start()
	return r, nil
}
func (*exampleResolverBuilder) Scheme() string { return exampleScheme }

type exampleResolver struct {
	target     resolver.Target
	cc         resolver.ClientConn
	addrsStore map[string][]string
}

func (r *exampleResolver) start() {
	addrStrs := r.addrsStore[r.target.Endpoint()]
	endpoints := make([]resolver.Endpoint, len(addrStrs))
	for i, s := range addrStrs {
		ep := resolver.Endpoint{Addresses: []resolver.Address{{Addr: s}}}

		// Explicitly set the hash key for the endpoint. If omitted, the
		// default hash key is the endpoint's first address.
		endpoints[i] = ringhash.SetHashKey(ep, strconv.Itoa(i))
	}
	r.cc.UpdateState(resolver.State{Endpoints: endpoints})
}
func (*exampleResolver) ResolveNow(resolver.ResolveNowOptions) {}
func (*exampleResolver) Close()                                {}

func init() {
	resolver.Register(&exampleResolverBuilder{})
}
