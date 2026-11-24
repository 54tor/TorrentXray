STATICCHECK = honnef.co/go/tools/cmd/staticcheck@v0.8.2
GOLANGCI_LINT = github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
GOVULNCHECK = golang.org/x/vuln/cmd/govulncheck@v1.10.0
GOSEC = github.com/securego/gosec/v2/cmd/gosec@v2.30.0
GOFUMPT = mvdan.cc/gofumpt@v0.12.0
GOIMPORTS = golang.org/x/tools/cmd/goimports@v0.53.0
DEADCODE = golang.org/x/tools/cmd/deadcode@v0.53.0
GREMLINS = github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0
HADOLINT_IMAGE = hadolint/hadolint:v2.15.1
GITLEAKS_IMAGE = zricethezav/gitleaks:v8.30.0
SHELLCHECK_IMAGE = koalaman/shellcheck:v0.11.0
SHFMT_IMAGE = mvdan/shfmt:v3.12.0
ACTIONLINT_IMAGE = rhysd/actionlint:1.7.8
IMAGE = mkvcheck
SHELL_SCRIPTS = scripts/fetch-assets.sh .githooks/pre-push
JS_SOURCES = web/static/*.js web/static/lib
FUZZ_TIME = 20s
LOW_PRIORITY = nice -n 10 ionice -c3

.PHONY: assets build test quality quality-fast fuzz mutation hooks docker

assets:
	bash scripts/fetch-assets.sh

build: assets
	CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o bin/mkvcheck ./cmd/mkvcheck

test:
	$(LOW_PRIORITY) go test -p 2 ./...

fuzz:
	$(LOW_PRIORITY) go test -p 2 -run '^$$' -fuzz '^FuzzParse$$' -fuzztime=$(FUZZ_TIME) ./internal/findings

mutation:
	$(LOW_PRIORITY) go run $(GREMLINS) unleash --workers 2 ./internal/findings

quality-fast:
	test -z "$$(go run $(GOFUMPT) -l .)"
	test -z "$$(go run $(GOIMPORTS) -l -local mkvcheck .)"
	go vet ./...
	go run $(STATICCHECK) ./...
	npm ci --ignore-scripts --no-audit --no-fund
	npx --no-install prettier --check $(JS_SOURCES)
	npx --no-install eslint web/static

quality: quality-fast
	go run $(GOLANGCI_LINT) run
	test -z "$$(go run $(DEADCODE) ./cmd/mkvcheck)"
	npx --no-install knip
	docker run --rm -v "$(CURDIR):/mnt" -w /mnt $(SHELLCHECK_IMAGE) $(SHELL_SCRIPTS)
	docker run --rm -v "$(CURDIR):/mnt" -w /mnt $(SHFMT_IMAGE) -d $(SHELL_SCRIPTS)
	docker run --rm -v "$(CURDIR):/repo" -w /repo $(ACTIONLINT_IMAGE) -color=false
	$(LOW_PRIORITY) go test -p 2 -race -cover ./...
	$(MAKE) fuzz
	node --test web/static/lib
	go run $(GOVULNCHECK) ./...
	go run $(GOSEC) -quiet ./...
	go mod tidy -diff
	go mod verify
	npm audit --audit-level=high
	docker run --rm -i $(HADOLINT_IMAGE) < Dockerfile
	docker run --rm -v "$(CURDIR):/repo" $(GITLEAKS_IMAGE) git --no-banner /repo

hooks:
	ln -sf ../../.githooks/pre-push .git/hooks/pre-push

docker:
	docker build -t $(IMAGE) .
