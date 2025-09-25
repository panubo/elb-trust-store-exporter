snapshot:
	goreleaser build --snapshot --clean

lint:
	golangci-lint run

fmt:
	golangci-lint fmt
