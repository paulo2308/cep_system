package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

type cepReq struct {
	CEP string `json:"cep"`
}

var cepRegex = regexp.MustCompile(`^\d{8}$`)

func main() {
	exporterEndpoint := getenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
	serviceName := getenv("OTEL_SERVICE_NAME", "service-a")
	shutdown := setupTracer(exporterEndpoint, serviceName)
	defer shutdown()

	mux := http.NewServeMux()
	mux.Handle("/cep", otelhttp.NewHandler(http.HandlerFunc(handleCEP), "handleCEP"))

	addr := ":8081"
	log.Printf("service-a listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func handleCEP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("method not allowed"))
		return
	}

	var payload cepReq
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&payload); err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte("invalid zipcode"))
		return
	}

	if payload.CEP == "" || !cepRegex.MatchString(payload.CEP) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte("invalid zipcode"))
		return
	}

	serviceB := getenv("SERVICE_B_URL", "http://localhost:8080")
	url := fmt.Sprintf("%s/weather?cep=%s", serviceB, payload.CEP)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	client := &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}

	ctx, span := otel.Tracer("service-a").Start(ctx, "forward to service-b")
	defer span.End()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("bad gateway"))
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("bad gateway"))
		return
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func setupTracer(endpoint, serviceName string) func() {
	ctx := context.Background()
	exp, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		log.Fatalf("failed to create exporter: %v", err)
	}
	rsrc := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
	)
	tp := trace.NewTracerProvider(
		trace.WithBatcher(exp),
		trace.WithResource(rsrc),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return func() { _ = tp.Shutdown(context.Background()) }
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
