    APP=leads-api

    .PHONY: all tidy build run test test-int perf worker

    all: tidy build
    tidy: ; go mod tidy

    build:
	CGO_ENABLED=0 go build -o bin/$(APP) ./cmd/leads-api

    worker:
	CGO_ENABLED=1 go build -o bin/outbox-worker ./cmd/outbox-worker

    run:
	CONFIG_FILE=configs/config.yaml go run ./cmd/leads-api

    test: ; ginkgo -r -p -race -trace -randomize-all
    test-int: ; ginkgo -r -p -label-filter='integration'
    perf: ; ginkgo -r -p -label-filter='perf'
