FROM gcr.io/distroless/static-debian12

COPY elb-trust-store-exporter /go/bin/elb-trust-store-exporter
CMD ["/go/bin/elb-trust-store-exporter"]
