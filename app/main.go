// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/embedded"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

// Create channel to listen for signals.
var signalChan chan (os.Signal) = make(chan os.Signal, 1)

type MeterProvider struct {
	embedded.MeterProvider
}

// set up metrics exporter and meter provider
var serviceName = getEnv("K_SERVICE", "sample-cloud-run-app")

var meter = otel.Meter("fm")
var counter2, _ = meter.Int64Counter(
	"prefix.example.com",
	metric.WithUnit("1"),
	metric.WithDescription("Number of calls to the API * 100"),
)

func main() {
	ctx := context.Background()
	// set up traceExporter
	traceExporter, err := otlptrace.New(ctx,
		otlptracegrpc.NewClient(otlptracegrpc.WithInsecure()),
	)
	if err != nil {
		log.Fatalf("Error creating trace exporter: %s", err)
	}

	// set up tracerprovider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(sdktrace.NewBatchSpanProcessor(traceExporter)),
	)
	defer tp.Shutdown(ctx)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
		),
	)
	exporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		log.Fatalf("Error creating exporter: %s", err)
	}
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		sdkmetric.WithResource(r),
	)
	defer provider.Shutdown(ctx)
	meter := provider.Meter("example.com/metrics")
	counter2, err = meter.Int64Counter("sidecar-sample-counter2")
	if err != nil {
		log.Fatalf("Error creating counter2: %s", err)
	}

	// SIGINT handles Ctrl+C locally.
	// SIGTERM handles Cloud Run termination signal.
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	// Start HTTP server.
	srv := &http.Server{
		Addr:    ":8080",
		Handler: http.HandlerFunc(handler),
	}
	go func() {
		http.HandleFunc("/", handler)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	// Receive output from signalChan.
	sig := <-signalChan
	log.Printf("%s signal caught. Graceful Shutdown.", sig)

	// Gracefully shutdown the server by waiting on existing requests (except websockets).
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("server shutdown failed: %+v", err)
	}
	log.Print("server exited")
}

func traceLogPrefix(traceId, spanId string) string {
	return fmt.Sprintf("sample-app [%s][spanId: %s]: ", traceId, spanId)
}

func handler(w http.ResponseWriter, r *http.Request) {
	// get trace context propagated from http request
	prop := otel.GetTextMapPropagator()
	ctx := prop.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
	tp := otel.GetTracerProvider()
	tracer := tp.Tracer("example.com/trace")

	commonAttrs := []attribute.KeyValue{
		attribute.String("", ""),
	}
	ctx, span := tracer.Start(ctx,
		"foo",
		trace.WithAttributes(commonAttrs...))
	defer span.End()

	// extract current span ID
	spanId := span.SpanContext().SpanID().String()
	traceId := span.SpanContext().TraceID().String()

	// open logging file
	f, err := os.OpenFile("/logging/sample-app.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Error opening log file: %s", err)
	}
	defer f.Close()

	// log incoming request with spanID
	logger := log.New(f, traceLogPrefix(traceId, spanId), log.LstdFlags)
	logger.Printf("Request: %s %s", r.Method, r.URL.Path)
	fmt.Fprintln(w, "Logged request to /logging/sample-app.log")

	// write traces
	generateSpans(ctx, tracer, logger, 10)
	fmt.Fprintln(w, "Generated 10 spans!")

	// update metric
	counter2.Add(ctx, 100)
	fmt.Fprintln(w, "Updated sidecar-sample-counter2 metric!")
}

func generateSpans(ctx context.Context, tracer trace.Tracer, logger *log.Logger, id int) {
	if id > 0 {
		ctx, span := tracer.Start(ctx, fmt.Sprintf("foo-%d", id))
		defer span.End()
		logger.SetPrefix(traceLogPrefix(
			span.SpanContext().TraceID().String(),
			span.SpanContext().SpanID().String(),
		))
		logger.Printf("Generating span %d...\n", id)
		generateSpans(ctx, tracer, logger, id-1)
	} else {
		fmt.Println("Done.")
	}
}

func getEnv(var_name string, default_val string) string {
	if val, ok := os.LookupEnv(var_name); ok {
		return val
	}
	return default_val
}
