# Ring Hash

This example shows how to use the ring hash load balancer for consistent hashing
across a pool of servers with soft affinity.

## Documentation

- [Envoy documentation that gRPC is based on](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/upstream/load_balancing/load_balancers#ring-hash)
- [gRFC for ring hash support](https://github.com/grpc/proposal/blob/master/A42-xds-ring-hash-lb-policy.md)
- [gRFC for improvements to ring hash](https://github.com/grpc/proposal/blob/master/A76-ring-hash-improvements.md)

## Try it

This example includes 4 servers that respond with their address. Each server
uses a specific hash to be placed on the ring. The client routes requests to the
server according to the "request-id" header using the ring hash balancer.

First start the servers:

```bash
go run server/main.go
```

Then run the client:

```bash
export GRPC_EXPERIMENTAL_RING_HASH_SET_REQUEST_HASH_KEY=true
export GRPC_XDS_ENDPOINT_HASH_KEY_BACKWARD_COMPAT=false
go run client/main.go
```

## Usage

### Define your hash parameters and headers

```go
var serviceConfig = `{"loadBalancingConfig": [{"ring_hash_experimental":{
    "minRingSize": 1024,
    "maxRingSize": 4096,
    "requestHashHeader": "request-id"
}}]}`
```

### Providing the ring hash balancer as a DialOption

To use the above service config, pass it with `grpc.WithDefaultServiceConfig` to
`grpc.NewClient`.

```go
conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(
    insecure.NewCredentials()),
    grpc.WithDefaultServiceConfig(serviceConfig)
)
client := pb.NewEchoClient(conn)
```

### Passing a the header when making RPCs

Pass a header to use as request hash key when making RPCs:

```go
md := metadata.Pairs("request-id", "my-request-id")
ctx := metadata.NewOutgoingContext(context.Background(), md)
client.Echo(ctx, &pb.EchoRequest{Message: "Hello"})
```
