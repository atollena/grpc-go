/*
 *
 * Copyright 2018 gRPC authors.
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
// distribute RPCs across backend servers.
package main

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	ecpb "google.golang.org/grpc/examples/features/proto/echo"
	_ "google.golang.org/grpc/health"
	"google.golang.org/grpc/resolver"
	_ "google.golang.org/grpc/xds"
)

const (
	exampleScheme      = "example"
	exampleServiceName = "lb.example.grpc.io"
)

var addrs = []string{}

func init() {
	for i := 0; i < 3000; i++ {
		addrs = append(addrs, fmt.Sprintf("localhost:%d", 25000+i))
	}
}

func callUnaryEcho(c ecpb.EchoClient, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := c.UnaryEcho(ctx, &ecpb.EchoRequest{Message: message})
	if err != nil {
		log.Printf("could not greet: %v", err)
	}
}

func makeRPCs(cc *grpc.ClientConn) {
	hwc := ecpb.NewEchoClient(cc)
	var i uint64
	go func() {
		t := time.NewTicker(1 * time.Minute)
		for range t.C {
			log.Printf("sent %d RPCs", atomic.SwapUint64(&i, 0))
		}
	}()
	t := time.NewTicker(15 * time.Second)
	for range t.C {
		log.Printf("sending %d RPCs", len(addrs))
		for j := 1; j <= len(addrs); j++ {
			callUnaryEcho(hwc, "this is examples/load_balancing")
			atomic.AddUint64(&i, 1)
		}
	}
}

func main() {
	// Make a ClientConn with round_robin policy.
	roundrobinConn, err := grpc.Dial(
		fmt.Sprintf("%s:///%s", exampleScheme, exampleServiceName),
		grpc.WithDefaultServiceConfig(`
{"loadBalancingConfig": [{"outlier_detection_experimental": {
					"childPolicy": [{"round_robin": {}}]}}],
"healthCheckConfig": {"serviceName":""}
}`), // This sets the initial balancing policy.
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer roundrobinConn.Close()

	fmt.Println("--- calling helloworld.Greeter/SayHello with round_robin ---")
	makeRPCs(roundrobinConn)
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
	addrs := make([]resolver.Address, len(addrStrs))
	for i, s := range addrStrs {
		addrs[i] = resolver.Address{Addr: s}
	}
	r.cc.UpdateState(resolver.State{Addresses: addrs})
}
func (*exampleResolver) ResolveNow(resolver.ResolveNowOptions) {
}
func (*exampleResolver) Close() {}

func init() {
	resolver.Register(&exampleResolverBuilder{})
}
