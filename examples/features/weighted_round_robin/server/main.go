/*
 *
 * Copyright 2023 gRPC authors.
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

// Binary server is an example server.
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	pb "google.golang.org/grpc/examples/features/proto/echo"
	"google.golang.org/grpc/orca"
	"google.golang.org/grpc/status"
)

var (
	servers = []*ecServer{{
		addr:          ":50051",
		cpuPerRequest: 0.025,
	}, {
		addr:          ":50052",
		cpuPerRequest: 0.05,
	}}
)

type ecServer struct {
	pb.UnimplementedEchoServer
	addr          string
	cpuPerRequest float64

	mu       sync.Mutex
	received int64
}

func (s *ecServer) UnaryEcho(ctx context.Context, in *pb.EchoRequest) (*pb.EchoResponse, error) {
	// Report a sample cost for this query.
	cmr := orca.CallMetricsRecorderFromContext(ctx)
	if cmr == nil {
		return nil, status.Errorf(codes.Internal, "unable to retrieve call metrics recorder (missing ORCA ServerOption?)")
	}

	s.mu.Lock()
	s.received += 1
	s.mu.Unlock()
	return &pb.EchoResponse{Message: in.Message}, nil
}

func startServer(srv *ecServer) {
	lis, err := net.Listen("tcp", srv.addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	smp := orca.NewServerMetricsRecorder()

	s := grpc.NewServer(orca.CallMetricsServerOption(smp))
	pb.RegisterEchoServer(s, srv)
	log.Printf("serving on %s\n", srv.addr)

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		for range ticker.C {
			srv.mu.Lock()
			received := float64(srv.received)
			cpuUsed := received * srv.cpuPerRequest
			srv.received = 0
			srv.mu.Unlock()
			smp.SetQPS(received)
			smp.SetCPUUtilization(cpuUsed)
			fmt.Printf("%s received %d (cpu used: %f - load: %f)\n", srv.addr, int64(received), cpuUsed, received/cpuUsed)
		}
	}()

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func main() {
	var wg sync.WaitGroup
	for _, srv := range servers {
		wg.Add(1)
		go func(srv *ecServer) {
			defer wg.Done()
			startServer(srv)
		}(srv)
	}
	wg.Wait()
}
