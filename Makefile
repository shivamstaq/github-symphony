.PHONY: build test test-property test-scenario test-contract lint local-lint docker-build docker-up docker-down doctor clean

build:
	go build -o bin/symphony ./cmd/symphony

test:
	go test ./... -count=1

test-property:
	go test ./test/property/ -v

test-scenario:
	go test ./test/scenario/ -v

test-contract:
	go test ./internal/tracker/ -v

lint:
	golangci-lint run

local-lint:
	docker run --rm -v $(CURDIR):/app -w /app golangci/golangci-lint:v2.11.4 golangci-lint run

docker-build:
	docker build -t symphony .

docker-up:
	docker compose up -d

docker-down:
	docker compose down

doctor:
	go build -o bin/symphony ./cmd/symphony && bin/symphony --doctor

clean:
	rm -rf bin/
