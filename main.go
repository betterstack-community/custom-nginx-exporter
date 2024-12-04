package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const templateMetrics string = `Active connections: %d
server accepts handled requests
%d %d %d
Reading: %d Writing: %d Waiting: %d
`

// StubStats represents NGINX stub_status metrics.
type StubStats struct {
	Connections StubConnections
	Requests    int64
}

// StubConnections represents connections related metrics.
type StubConnections struct {
	Active   int64
	Accepted int64
	Handled  int64
	Reading  int64
	Writing  int64
	Waiting  int64
}

// GetStubStats fetches the stub_status metrics.
func GetStubStats(endpoint string) (*StubStats, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		endpoint,
		http.NoBody,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create a get request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get %v: %w", endpoint, err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"expected %v response, got %v",
			http.StatusOK,
			resp.StatusCode,
		)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read the response body: %w", err)
	}

	r := bytes.NewReader(body)

	stats, err := parseStubStats(r)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to parse response body %q: %w",
			string(body),
			err,
		)
	}

	return stats, nil
}

func parseStubStats(r io.Reader) (*StubStats, error) {
	var s StubStats
	if _, err := fmt.Fscanf(r, templateMetrics,
		&s.Connections.Active,
		&s.Connections.Accepted,
		&s.Connections.Handled,
		&s.Requests,
		&s.Connections.Reading,
		&s.Connections.Writing,
		&s.Connections.Waiting); err != nil {
		return nil, fmt.Errorf("failed to scan template metrics: %w", err)
	}

	return &s, nil
}

// Metrics holds descriptions for NGINX-related metrics.
type metrics struct {
	ActiveConnectionsDesc   *prometheus.Desc
	ConnectionsReadingDesc  *prometheus.Desc
	ConnectionsAcceptedDesc *prometheus.Desc
	ConnectionsHandledDesc  *prometheus.Desc
	ConnectionsWaitingDesc  *prometheus.Desc
	ConnectionsWritingDesc  *prometheus.Desc
	HTTPRequestsTotalDesc   *prometheus.Desc
}

// NewMetrics initializes all metric descriptions.
func NewMetrics(namespace string) *metrics {
	return &metrics{
		ActiveConnectionsDesc: prometheus.NewDesc(
			namespace+"_connections_active",
			"Active client connections",
			nil, nil,
		),
		ConnectionsReadingDesc: prometheus.NewDesc(
			namespace+"_connections_reading",
			"Connections currently reading client request headers",
			nil, nil,
		),
		ConnectionsAcceptedDesc: prometheus.NewDesc(
			namespace+"_connections_accepted_total",
			"Total accepted client connections",
			nil, nil,
		),
		ConnectionsHandledDesc: prometheus.NewDesc(
			namespace+"_connections_handled_total",
			"Total handled client connections",
			nil, nil,
		),
		ConnectionsWaitingDesc: prometheus.NewDesc(
			namespace+"_connections_waiting",
			"Idle client connections",
			nil, nil,
		),
		ConnectionsWritingDesc: prometheus.NewDesc(
			namespace+"_connections_writing",
			"Connections where NGINX is currently writing responses to clients",
			nil, nil,
		),
		HTTPRequestsTotalDesc: prometheus.NewDesc(
			namespace+"_http_requests_total",
			"Total number of HTTP requests handled",
			nil, nil,
		),
	}
}

// CollectMetrics is a struct that collects metrics dynamically.
type CollectMetrics struct {
	metrics *metrics
}

// NewCollector creates a new instance of CollectMetrics.
func NewCollector(namespace string, reg prometheus.Registerer) *CollectMetrics {
	m := NewMetrics(namespace)
	c := &CollectMetrics{metrics: m}
	reg.MustRegister(c)
	return c
}

// Describe sends metric descriptions to the provided channel.
func (c *CollectMetrics) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.metrics.ActiveConnectionsDesc
	ch <- c.metrics.ConnectionsReadingDesc
	ch <- c.metrics.ConnectionsAcceptedDesc
	ch <- c.metrics.ConnectionsHandledDesc
	ch <- c.metrics.ConnectionsWaitingDesc
	ch <- c.metrics.ConnectionsWritingDesc
	ch <- c.metrics.HTTPRequestsTotalDesc
}

// Collect dynamically collects metrics and sends them to Prometheus.
func (c *CollectMetrics) Collect(ch chan<- prometheus.Metric) {
	endpoint := os.Getenv("NGINX_STATUS_ENDPOINT")

	nginxStats, err := GetStubStats(endpoint)
	if err != nil {
		log.Println(err)
		return
	}

	activeConnections := float64(nginxStats.Connections.Active)
	connectionsReading := float64(nginxStats.Connections.Reading)
	connectionsAccepted := float64(nginxStats.Connections.Accepted)
	connectionsHandled := float64(nginxStats.Connections.Handled)
	connectionsWaiting := float64(nginxStats.Connections.Waiting)
	connectionsWriting := float64(nginxStats.Connections.Writing)
	httpRequestsTotal := float64(nginxStats.Requests)

	ch <- prometheus.MustNewConstMetric(c.metrics.ActiveConnectionsDesc, prometheus.GaugeValue, activeConnections)
	ch <- prometheus.MustNewConstMetric(c.metrics.ConnectionsReadingDesc, prometheus.GaugeValue, connectionsReading)
	ch <- prometheus.MustNewConstMetric(c.metrics.ConnectionsAcceptedDesc, prometheus.CounterValue, connectionsAccepted)
	ch <- prometheus.MustNewConstMetric(c.metrics.ConnectionsHandledDesc, prometheus.CounterValue, connectionsHandled)
	ch <- prometheus.MustNewConstMetric(c.metrics.ConnectionsWaitingDesc, prometheus.GaugeValue, connectionsWaiting)
	ch <- prometheus.MustNewConstMetric(c.metrics.ConnectionsWritingDesc, prometheus.GaugeValue, connectionsWriting)
	ch <- prometheus.MustNewConstMetric(c.metrics.HTTPRequestsTotalDesc, prometheus.CounterValue, httpRequestsTotal)
}

func main() {
	mux := http.NewServeMux()

	reg := prometheus.NewRegistry()

	NewCollector("nginx", reg)

	handler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})

	mux.Handle("/metrics", handler)

	http.ListenAndServe(":9113", mux)
}
