# AWS ELB Trust Store Exporter

A Prometheus exporter for AWS Elastic Load Balancer (ELB) trust stores, to enable monitoring of certificate expiry.

## Overview

The AWS ELB Trust Store Exporter is a Prometheus exporter that queries the AWS API for information about certificates within ELB trust stores. By default it will monitor all trust stores in a given region, but can be configured to monitor a specific set of trust stores. It exposes metrics for these certificates, including their expiry dates, allowing you to monitor them and create alerts for expiring certificates.

The exporter is written in Go and uses the official Prometheus client library. It is designed to be lightweight and easy to deploy.

## Usage

The exporter can be configured using command-line flags:

```
usage: elb-trust-store-exporter [<flags>]

Flags:
  --web.listen-address=":9180"  Address to listen on for web interface and telemetry.
  --web.metrics-path="/metrics"
                                Path under which to expose metrics.
  --region                     AWS region to query. If not specified, the region will be
                               auto-discovered from the environment (e.g., EC2 instance metadata).
  --query-interval="60m"       Interval at which to query the AWS API.
  --trust-store-arns           An optional comma-separated list of ELB trust store ARNs to monitor.
                               If not specified, all trust stores in the region will be monitored.
```

### Example

```bash
# Monitor all trust stores in the default region
./elb-trust-store-exporter

# Monitor specific trust stores
./elb-trust-store-exporter \
  --trust-store-arns="arn:aws:elasticloadbalancing:us-east-1:123456789012:truststore/my-trust-store/1234567890abcdef,arn:aws:elasticloadbalancing:us-east-1:123456789012:truststore/another-trust-store/fedcba9876543210"
```

## Metrics

The exporter exposes the following metrics:

| Metric                                     | Description                                                                      | Labels                                                                                                                              |
| ------------------------------------------ | -------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| `elb_trust_store_exporter_build_info` | A metric with a constant '1' value labeled with version, commit, date and builtBy from which the exporter was built. | `version`, `commit`, `date`, `builtBy` |
| `elb_trust_store_certificate_info` | Information about a certificate in a trust store. | `trust_store_arn`, `serial_number`, `issuer`, `subject`, `signature_algo`, `key_length` |
| `elb_trust_store_certificate_not_before` | The timestamp of the start of the certificate's validity (in seconds since epoch). | `trust_store_arn`, `serial_number`, `subject` |
| `elb_trust_store_certificate_expiry` | The timestamp of the certificate's expiry (in seconds since epoch). | `trust_store_arn`, `serial_number`, `subject` |
| `elb_trust_store_info` | Information about the trust store | `trust_store_arn`, `name`, `region` |
| `elb_trust_store_certificates` | The number of CA certificates in the trust store | `trust_store_arn` |
| `elb_trust_store_revoked_entries` | The number of revoked entries in the trust store | `trust_store_arn` |
| `elb_trust_store_exporter_last_scrape_timestamp` | The timestamp of the last successful scrape of the AWS API. | |
| `elb_trust_store_exporter_scrape_duration_seconds` | The duration of the last scrape of the AWS API. | |
| `elb_trust_store_exporter_scrape_interval` | The interval between scraping the AWS API. | |
| `elb_trust_store_collector_success` | Was the last scrape of the collector successful. | |

## How it works

The exporter queries the AWS ELB API on startup and then at a regular interval (configurable with `--aws.query-interval`) to fetch details for the specified trust stores. It then exposes the metrics for each certificate in the trust stores on the `/metrics` endpoint.

If an AWS region is not specified via the `--aws.region` flag, the exporter will attempt to auto-discover it from the environment, for example from EC2 instance metadata. This is useful when running the exporter on an EC2 instance.
