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

package tlscreds_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/internal/stubserver"
	"google.golang.org/grpc/internal/testutils/xds/e2e"
	testgrpc "google.golang.org/grpc/interop/grpc_testing"
	testpb "google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/testdata"
	"google.golang.org/grpc/xds/internal/xdsclient/tlscreds"
)

func TestValidTlsBuilder(t *testing.T) {
	tests := []struct {
		name string
		jd   string
	}{
		{"Absent configuration", `null`},
		{"Empty configuration", `{}`},
		{"Only CA certificate chain", `{"ca_certificate_file": "foo"}`},
		{"Only private key and certificate chain", `{"certificate_file":"bar","private_key_file":"baz"}`},
		{"CA chain, private key and certificate chain", `{"ca_certificate_file":"foo","certificate_file":"bar","private_key_file":"baz"}`},
		{"Only refresh interval", `{"refresh_interval": "1s"}`},
		{"Refresh interval and CA certificate chain", `{"refresh_interval": "1s","ca_certificate_file": "foo"}`},
		{"Refresh interval, private key and certificate chain", `{"refresh_interval": "1s","certificate_file":"bar","private_key_file":"baz"}`},
		{"Refresh interval, CA chain, private key and certificate chain", `{"refresh_interval": "1s","ca_certificate_file":"foo","certificate_file":"bar","private_key_file":"baz"}`},
		{"Unknown field", `{"unknown_field": "foo"}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			msg := json.RawMessage(test.jd)
			if _, err := tlscreds.NewBundle(msg); err != nil {
				t.Errorf("NewBundle(%s): expected no error but got: %s", test.jd, err)
			}
		})
	}
}

func TestInvalidTlsBuilder(t *testing.T) {
	tests := []struct {
		name, jd, wantErrPrefix string
	}{
		{"Wrong type in json", `{"ca_certificate_file": 1}`, "failed to unmarshal config:"},
		{"Missing private key", `{"certificate_file":"bar"}`, "pemfile: private key file and identity cert file should be both specified or not specified"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			msg := json.RawMessage(test.jd)
			if _, err := tlscreds.NewBundle(msg); err == nil || !strings.HasPrefix(err.Error(), test.wantErrPrefix) {
				t.Errorf("NewBundle(%s): want error %s, got: %s", msg, test.wantErrPrefix, err)
			}
		})
	}
}

func TestCaReloading(t *testing.T) {
	serverCa, err := os.ReadFile(testdata.Path("x509/server_ca_cert.pem"))
	if err != nil {
		t.Fatalf("Failed to read test CA cert: %s", err)
	}

	// Write CA certs to a temporary file so that we can modify it later.
	caPath := t.TempDir() + "/ca.pem"
	err = os.WriteFile(caPath, serverCa, 0644)
	if err != nil {
		t.Fatalf("Failed to write test CA cert: %v", err)
	}
	cfg := fmt.Sprintf(`{
		"ca_certificate_file": "%s",
		"refresh_interval": ".01s"
	}`, caPath)
	tlsBundle, err := tlscreds.NewBundle([]byte(cfg))
	if err != nil {
		t.Fatalf("Failed to create TLS bundle: %v", err)
	}

	serverCredentials := grpc.Creds(e2e.CreateServerTLSCredentials(t, tls.NoClientCert))
	server := stubserver.StartTestService(t, nil, serverCredentials)

	conn, err := grpc.Dial(
		server.Address,
		grpc.WithCredentialsBundle(tlsBundle),
		grpc.WithAuthority("x.test.example.com"),
	)
	if err != nil {
		t.Fatalf("Error dialing: %v", err)
	}
	defer conn.Close()
	client := testgrpc.NewTestServiceClient(conn)
	_, err = client.EmptyCall(context.Background(), &testpb.Empty{})
	if err != nil {
		t.Errorf("Error calling EmptyCall: %v", err)
	}
	// close the server and create a new one to force client to do a new
	// handshake.
	server.Stop()

	invalidCa, err := os.ReadFile(testdata.Path("ca.pem"))
	if err != nil {
		t.Fatalf("Failed to read test CA cert: %v", err)
	}
	// unload root cert
	err = os.WriteFile(caPath, invalidCa, 0644)
	if err != nil {
		t.Fatalf("Failed to write test CA cert: %v", err)
	}

	// Leave time for the file_watcher provider to reload the CA.
	time.Sleep(100 * time.Millisecond)

	server = stubserver.StartTestService(t, &stubserver.StubServer{Address: server.Address}, serverCredentials)
	defer server.Stop()

	// Client handshake should fail because the server cert is signed by an
	// unknown CA.
	_, err = client.EmptyCall(context.Background(), &testpb.Empty{})
	if st, ok := status.FromError(err); !ok || st.Code() != codes.Unavailable {
		t.Errorf("Expected unavailable error, got %v", err)
	} else if want := "certificate signed by unknown authority"; !strings.Contains(st.Message(), want) {
		t.Errorf("Expected call error to contain '%s', got %v", want, st.Message())
	}
}

func TestMTLS(t *testing.T) {
	s := stubserver.StartTestService(t, nil, grpc.Creds(e2e.CreateServerTLSCredentials(t, tls.RequireAndVerifyClientCert)))
	defer s.Stop()

	cfg := fmt.Sprintf(`{
		"ca_certificate_file": "%s",
		"certificate_file": "%s",
		"private_key_file": "%s"
	}`,
		testdata.Path("x509/server_ca_cert.pem"),
		testdata.Path("x509/client1_cert.pem"),
		testdata.Path("x509/client1_key.pem"))
	tlsBundle, err := tlscreds.NewBundle([]byte(cfg))
	if err != nil {
		t.Fatalf("Failed to create TLS bundle: %v", err)
	}
	dialOpts := []grpc.DialOption{
		grpc.WithCredentialsBundle(tlsBundle),
		grpc.WithAuthority("x.test.example.com"),
	}

	conn, err := grpc.Dial(s.Address, dialOpts...)
	if err != nil {
		t.Fatalf("Error dialing: %v", err)
	}
	client := testgrpc.NewTestServiceClient(conn)
	_, err = client.EmptyCall(context.Background(), &testpb.Empty{})
	if err != nil {
		t.Errorf("Error calling EmptyCall: %v", err)
	}
}
