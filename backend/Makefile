SQLC := go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1

.PHONY: check test integration generate build
check:
	@test -z "$$(gofmt -l cmd internal schema tests)"
	go vet ./...
	go vet -tags=integration ./...
	$(SQLC) diff
	go test -race ./...
	go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...

test:
	go test -race ./...

integration:
	go test -race -tags=integration ./...

generate:
	$(SQLC) generate

build:
	go build -trimpath -o bin/api ./cmd/api
	go build -trimpath -o bin/bootstrap ./cmd/bootstrap
