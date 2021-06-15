BIN = $(CURDIR)/bin
PKGS     = $(or $(PKG),$(shell env GO111MODULE=on $(GO) list ./...))

GO = go
Q = $(if $(filter 1,$V),,@)
M = $(shell printf "\033[34;1m▶\033[0m")

.PHONY: all
all: fmt $(BIN) ; $(info $(M) building executable) @ ## Build program binary
				$Q go build -o ./bin/thoth main.go

.PHONY: fmt
fmt: ; $(info $(M) running gofmt...) @ ## Run gofmt on all source files
				$Q $(GO) fmt $(PKGS)

.PHONY: run
run: 
				go run main.go