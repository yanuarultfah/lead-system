package observability

import (
	"context"
	"leads-system/internal/config"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func InitTracerProvider(cfg *config.Config) (*sdktrace.TracerProvider, func(), error) {
	exp, err := otlptracehttp.New(context.Background(),
		otlptracehttp.WithEndpoint(cfg.OTEL.Endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, func() {}, err
	}
	res, _ := resource.Merge(resource.Default(), resource.NewWithAttributes(
		"", attribute.String("service.name", cfg.App.Name),
	))
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp, sdktrace.WithMaxExportBatchSize(4096), sdktrace.WithBatchTimeout(2*time.Second)),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return tp, func() { _ = tp.Shutdown(context.Background()) }, nil
}
