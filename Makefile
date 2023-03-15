COMMIT_SHA              ?= latest

.PHONY: build-local
build-local:
	# go fmt
	#go mod vendor
	go build -mod=readonly #-mod=vendor

.PHONY: exec-local
exec-local:
	ENV="dev" REDIS_URL="redis://localhost:6379/?dial_timeout=5s&max_retries=3" \
	./looking-binormal

.PHONY: exec-open
exec-open:
	ENV="dev" \
	LISTEN_ADDRESS="0.0.0.0:8080" \
	REDIS_URL="redis://localhost:6379/?dial_timeout=5s&max_retries=3" \
	./looking-binormal

run-local: build-local exec-local

run-open: build-local exec-open

.PHONY: test
test:
	CGO_ENABLED=0 go test -timeout 30s -v -count=1 -run . -cover ./...
