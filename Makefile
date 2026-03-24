.PHONY: build test test-integration lint sidecar docker-build docker-up doctor clean

build:
	go build -o bin/symphony ./cmd/symphony

test:
	go test ./... -count=1

test-integration:
	go test -tags=integration ./... -count=1

lint:
	golangci-lint run

sidecar:
	cd sidecar/claude && npm install

docker-build:
	docker build -t symphony .

docker-up:
	docker compose up -d

docker-down:
	docker compose down

doctor:
	go build -o bin/symphony ./cmd/symphony && bin/symphony --doctor

clean:
	rm -rf bin/ sidecar/claude/node_modules
