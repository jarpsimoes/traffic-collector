module github.com/jarpsimoes/traffic-collector

go 1.21

require (
	github.com/cilium/ebpf v0.13.2
	go.opentelemetry.io/otel v1.21.0
	go.opentelemetry.io/otel/exporters/jaeger/thrift v1.21.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.21.0
	go.opentelemetry.io/otel/sdk v1.21.0
	go.uber.org/zap v1.26.0
	k8s.io/api v0.28.4
	k8s.io/apimachinery v0.28.4
	k8s.io/client-go v0.28.4
)

require (
	github.com/apex/log v1.9.0
	github.com/goreleaser/goreleaser v1.21.2
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.18.1
	google.golang.org/grpc v1.59.0
	google.golang.org/protobuf v1.31.0
)
