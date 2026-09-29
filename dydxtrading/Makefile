build:
	CGO_ENABLED=1 go build -mod=readonly -tags "netgo" -o build/dydxtrading ./cmd/dydxtrading

test:
	go test -mod=readonly -tags "test_ledger_mock netgo" ./...

image:
	docker build -f dydxtrading/Dockerfile .. -t local:dydxtrading

compose-up:
	docker compose -f docker-compose.yml up --force-recreate $(ARGS)

compose-upd:
	docker compose -f docker-compose.yml up --force-recreate -d $(ARGS)

compose-stop:
	docker compose -f docker-compose.yml down

compose-logs:
	docker compose -f docker-compose.yml logs -f dydxtrading

.PHONY: build test image compose-up compose-upd compose-stop compose-logs
