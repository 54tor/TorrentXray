STATICCHECK = honnef.co/go/tools/cmd/staticcheck@v0.8.2
GOLANGCI_LINT = github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
GOVULNCHECK = golang.org/x/vuln/cmd/govulncheck@v1.10.0
GOSEC = github.com/securego/gosec/v2/cmd/gosec@v2.30.0
HADOLINT_IMAGE = hadolint/hadolint:v2.15.1
GITLEAKS_IMAGE = zricethezav/gitleaks:v8.30.0
IMAGE = mkvcheck

.PHONY: assets build test quality docker

assets:
	bash scripts/fetch-assets.sh

build: assets
	CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o bin/mkvcheck ./cmd/mkvcheck

test:
	nice -n 10 ionice -c3 go test -p 2 ./...

quality:
	test -z "$$(gofmt -l .)"
	go vet ./...
	go run $(STATICCHECK) ./...
	go run $(GOLANGCI_LINT) run
	nice -n 10 ionice -c3 go test -p 2 -race -cover ./...
	go run $(GOVULNCHECK) ./...
	go run $(GOSEC) -quiet ./...
	go mod tidy -diff
	go mod verify
	docker run -i $(HADOLINT_IMAGE) < Dockerfile
	docker run -v "$(CURDIR):/repo" $(GITLEAKS_IMAGE) git --no-banner /repo

docker:
	docker build -t $(IMAGE) .
