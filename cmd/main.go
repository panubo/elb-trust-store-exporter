package cmd

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/alecthomas/kong"
	"github.com/panubo/elb-trust-store-exporter/collector"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	Version string
	Commit  string
	Date    string
	BuiltBy string
)

var CLI struct {
	ListenAddress  string           `kong:"name='web.listen-address',default=':9180',help='Address to listen on for web interface and telemetry.'"`
	MetricsPath    string           `kong:"name='web.metrics-path',default='/metrics',help='Path under which to expose metrics.'"`
	Region         string           `kong:"name='region',optional,help='AWS region to query. If not specified, the region will be auto-discovered.'"`
	QueryInterval  string           `kong:"name='query-interval',default='60m',help='Interval at which to query the AWS API.'"`
	TrustStoreARNs []string         `kong:"name='trust-store-arns',optional,help='A comma-separated list of ELB trust store ARNs to monitor.'"`
	Version        kong.VersionFlag `kong:"name='version',short='v',help='Print version information and exit.'"`
}

func Run(args []string) {
	kong.Parse(&CLI,
		kong.Name("elb-trust-store-exporter"),
		kong.Description("A Prometheus exporter for AWS Elastic Load Balancer (ELB) trust stores."),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
		}),
		kong.Vars{
			"version": fmt.Sprintf(
				"%s\ncommit: %s\nbuilt at: %s\nbuilt by: %s",
				Version,
				Commit,
				Date,
				BuiltBy,
			),
		},
	)

	reg := prometheus.NewRegistry()

	versionMetric := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "elb_trust_store_exporter_build_info",
		Help: "A metric with a constant '1' value labeled with version, commit, date and builtBy from which the exporter was built.",
		ConstLabels: prometheus.Labels{
			"version": Version,
			"commit":  Commit,
			"date":    Date,
			"builtBy": BuiltBy,
		},
	})
	versionMetric.Set(1)
	reg.MustRegister(versionMetric)

	interval, err := time.ParseDuration(CLI.QueryInterval)
	if err != nil {
		log.Fatalf("failed to parse query interval: %v", err)
	}
	c := collector.New(CLI.Region, CLI.TrustStoreARNs, interval)
	reg.MustRegister(c)

	http.Handle(CLI.MetricsPath, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte(`<html>
			<head><title>AWS ELB Trust Store Exporter</title></head>
			<body>
			<h1>AWS ELB Trust Store Exporter</h1>
			<p><a href="` + CLI.MetricsPath + `">Metrics</a></p>
			</body>
			</html>`)); err != nil {
			log.Printf("failed to write response: %v", err)
		}
	})

	log.Printf("Starting server on %s", CLI.ListenAddress)
	server := &http.Server{
		Addr:         CLI.ListenAddress,
		ReadTimeout:  time.Minute,
		WriteTimeout: time.Minute,
		IdleTimeout:  2 * time.Minute,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}
