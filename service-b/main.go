package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
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

var (
	cepRegex = regexp.MustCompile(`^\d{8}$`)
)

type viaCEPResp struct {
	Localidade string `json:"localidade"`
	Erro       string `json:"erro"`
}

type weatherResp struct {
	Current struct {
		TempC float64 `json:"temp_c"`
	} `json:"current"`
}

type out struct {
	City  string  `json:"city"`
	TempC float64 `json:"temp_C"`
	TempF float64 `json:"temp_F"`
	TempK float64 `json:"temp_K"`
}

func main() {
	exporterEndpoint := getenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
	serviceName := getenv("OTEL_SERVICE_NAME", "service-b")
	shutdown := setupTracer(exporterEndpoint, serviceName)
	defer shutdown()

	mux := http.NewServeMux()
	mux.Handle("/weather", otelhttp.NewHandler(http.HandlerFunc(handleWeather), "handleWeather"))

	addr := ":8080"
	log.Printf("service-b listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func handleWeather(w http.ResponseWriter, r *http.Request) {
	cep := r.URL.Query().Get("cep")
	if !cepRegex.MatchString(cep) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte("invalid zipcode"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	client := &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}

	var city string
	if err := func() error {
		ctx, span := otel.Tracer("service-b").Start(ctx, "viaCEP lookup")
		defer span.End()

		url := fmt.Sprintf("https://viacep.com.br/ws/%s/json/", cep)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("viacep status %d", resp.StatusCode)
		}
		var v viaCEPResp
		if err = json.NewDecoder(resp.Body).Decode(&v); err != nil {
			return err
		}
		if v.Erro == "true" || v.Localidade == "" {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("can not find zipcode"))
			return errors.New("notfound")
		}
		city = v.Localidade
		return nil
	}(); err != nil {
		if err.Error() == "notfound" {
			return
		}
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("bad gateway"))
		return
	}

	key := os.Getenv("WEATHER_API_KEY")
	if key == "" {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("weather api key missing"))
		return
	}

	var tempC float64
	if err := func() error {
		ctx, span := otel.Tracer("service-b").Start(ctx, "weatherapi current")
		defer span.End()

		q := fmt.Sprintf("%s", city)
		url := fmt.Sprintf("https://api.weatherapi.com/v1/current.json?key=%s&q=%s&aqi=no",
			key, url.QueryEscape(q))

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			b, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("weather status %d: %s", resp.StatusCode, string(b))
		}
		var wresp weatherResp
		if err := json.NewDecoder(resp.Body).Decode(&wresp); err != nil {
			return err
		}
		tempC = wresp.Current.TempC
		return nil
	}(); err != nil {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("bad gateway"))
		return
	}

	out := out{
		City:  city,
		TempC: round1(tempC),
		TempF: round1(tempC*1.8 + 32),
		TempK: round1(tempC + 273),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
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

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}
