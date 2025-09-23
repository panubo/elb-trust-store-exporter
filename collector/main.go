package collector

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "elb_trust_store"
)

type Collector struct {
	mutex                         sync.Mutex
	metrics                       []prometheus.Metric
	scrapeInterval                time.Duration
	region                        string
	trustStoreARNs                []string
	collectorSuccess              *prometheus.Desc
	certificateInfo               *prometheus.Desc
	certificateNotBefore          *prometheus.Desc
	certificateExpiry             *prometheus.Desc
	trustStoreInfo                *prometheus.Desc
	trustStoreCertificates        *prometheus.Desc
	trustStoreRevokedEntries      *prometheus.Desc
	exporterLastScrapeTimestamp   *prometheus.Desc
	exporterScrapeDurationSeconds *prometheus.Desc
	exporterScrapeInterval        *prometheus.Desc
}

func New(region string, arns []string, interval time.Duration) *Collector {
	c := &Collector{
		scrapeInterval: interval,
		region:         region,
		trustStoreARNs: arns,
		collectorSuccess: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "collector_success"),
			"Was the last scrape of the collector successful.",
			nil,
			nil,
		),
		certificateInfo: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "certificate", "info"),
			"Information about a certificate in a trust store.",
			[]string{"trust_store_arn", "serial_number", "issuer", "subject", "signature_algo", "key_length"},
			nil,
		),
		certificateNotBefore: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "certificate", "not_before"),
			"The timestamp of the start of the certificate's validity (in seconds since epoch).",
			[]string{"trust_store_arn", "serial_number", "subject"},
			nil,
		),
		certificateExpiry: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "certificate", "expiry"),
			"The timestamp of the certificate's expiry (in seconds since epoch).",
			[]string{"trust_store_arn", "serial_number", "subject"},
			nil,
		),
		trustStoreInfo: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "info"),
			"Information about the trust store.",
			[]string{"trust_store_arn", "name", "region"},
			nil,
		),
		trustStoreCertificates: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "certificates"),
			"The number of CA certificates in the trust store.",
			[]string{"trust_store_arn"},
			nil,
		),
		trustStoreRevokedEntries: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "revoked_entries"),
			"The number of revoked entries in the trust store.",
			[]string{"trust_store_arn"},
			nil,
		),
		exporterLastScrapeTimestamp: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "exporter", "last_scrape_timestamp"),
			"The timestamp of the last successful scrape of the AWS API.",
			nil,
			nil,
		),
		exporterScrapeDurationSeconds: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "exporter", "scrape_duration_seconds"),
			"The duration of the last scrape of the AWS API.",
			nil,
			nil,
		),
		exporterScrapeInterval: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "exporter", "scrape_interval"),
			"The interval between scraping the AWS API.",
			nil,
			nil,
		),
	}
	c.scrape()
	go c.backgroundScrape(interval)
	return c
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.collectorSuccess
	ch <- c.certificateInfo
	ch <- c.certificateNotBefore
	ch <- c.certificateExpiry
	ch <- c.trustStoreInfo
	ch <- c.trustStoreCertificates
	ch <- c.trustStoreRevokedEntries
	ch <- c.exporterLastScrapeTimestamp
	ch <- c.exporterScrapeDurationSeconds
	ch <- c.exporterScrapeInterval
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for _, m := range c.metrics {
		ch <- m
	}
}

func (c *Collector) backgroundScrape(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		c.scrape()
	}
}

func (c *Collector) scrape() {
	log.Println("Scraping metrics")
	now := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	var metrics []prometheus.Metric
	success := true

	var cfgOpts []func(*config.LoadOptions) error
	if c.region != "" {
		cfgOpts = append(cfgOpts, config.WithRegion(c.region))
	}
	cfg, err := config.LoadDefaultConfig(ctx, cfgOpts...)
	if err != nil {
		log.Printf("Error creating AWS config: %v", err)
		success = false
	}

	if success {
		svc := elasticloadbalancingv2.NewFromConfig(cfg)

		input := &elasticloadbalancingv2.DescribeTrustStoresInput{}
		if len(c.trustStoreARNs) > 0 {
			input.TrustStoreArns = c.trustStoreARNs
		}

		result, err := svc.DescribeTrustStores(ctx, input)
		if err != nil {
			log.Printf("Error describing trust stores: %v", err)
			success = false
		}

		if success {
			log.Printf("Found %d trust stores", len(result.TrustStores))

			for _, ts := range result.TrustStores {
				if err := c.collectTrustStoreMetrics(ctx, svc, ts, &metrics); err != nil {
					log.Printf("Error collecting metrics for trust store %s: %v", *ts.TrustStoreArn, err)
					success = false
				}
			}
		}
	}

	scrapeDuration := time.Since(now)

	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Exporter metrics
	metrics = append(metrics, prometheus.MustNewConstMetric(c.exporterLastScrapeTimestamp, prometheus.GaugeValue, float64(now.Unix())))
	metrics = append(metrics, prometheus.MustNewConstMetric(c.exporterScrapeDurationSeconds, prometheus.GaugeValue, scrapeDuration.Seconds()))
	metrics = append(metrics, prometheus.MustNewConstMetric(c.exporterScrapeInterval, prometheus.GaugeValue, c.scrapeInterval.Seconds()))
	if success {
		metrics = append(metrics, prometheus.MustNewConstMetric(c.collectorSuccess, prometheus.GaugeValue, 1))
	} else {
		metrics = append(metrics, prometheus.MustNewConstMetric(c.collectorSuccess, prometheus.GaugeValue, 0))
	}

	c.metrics = metrics
}

func (c *Collector) collectTrustStoreMetrics(ctx context.Context, svc *elasticloadbalancingv2.Client, ts types.TrustStore, metrics *[]prometheus.Metric) error {
	*metrics = append(*metrics, prometheus.MustNewConstMetric(c.trustStoreInfo, prometheus.GaugeValue, 1, *ts.TrustStoreArn, *ts.Name, c.region))
	*metrics = append(*metrics, prometheus.MustNewConstMetric(c.trustStoreCertificates, prometheus.GaugeValue, float64(*ts.NumberOfCaCertificates), *ts.TrustStoreArn))
	*metrics = append(*metrics, prometheus.MustNewConstMetric(c.trustStoreRevokedEntries, prometheus.GaugeValue, float64(*ts.TotalRevokedEntries), *ts.TrustStoreArn))

	bundle, err := svc.GetTrustStoreCaCertificatesBundle(ctx, &elasticloadbalancingv2.GetTrustStoreCaCertificatesBundleInput{
		TrustStoreArn: ts.TrustStoreArn,
	})
	if err != nil {
		return err
	}

	httpClient := http.Client{
		Timeout: 3 * time.Second,
	}
	resp, err := httpClient.Get(*bundle.Location)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	pemData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	for len(pemData) > 0 {
		var block *pem.Block
		block, pemData = pem.Decode(pemData)
		if block == nil {
			break
		}

		if block.Type != "CERTIFICATE" {
			continue
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			log.Printf("Error parsing certificate: %v", err)
			continue
		}

		keyLength := 0
		switch pub := cert.PublicKey.(type) {
		case *rsa.PublicKey:
			keyLength = pub.N.BitLen()
		case *ecdsa.PublicKey:
			keyLength = pub.Curve.Params().BitSize
		default:
			return errors.New("unknown public key type")
		}

		*metrics = append(*metrics, prometheus.MustNewConstMetric(c.certificateInfo, prometheus.GaugeValue, 1, *ts.TrustStoreArn, cert.SerialNumber.String(), cert.Issuer.String(), cert.Subject.String(), cert.SignatureAlgorithm.String(), strconv.Itoa(keyLength)))
		*metrics = append(*metrics, prometheus.MustNewConstMetric(c.certificateNotBefore, prometheus.GaugeValue, float64(cert.NotBefore.Unix()), *ts.TrustStoreArn, cert.SerialNumber.String(), cert.Subject.String()))
		*metrics = append(*metrics, prometheus.MustNewConstMetric(c.certificateExpiry, prometheus.GaugeValue, float64(cert.NotAfter.Unix()), *ts.TrustStoreArn, cert.SerialNumber.String(), cert.Subject.String()))
	}
	return nil
}
