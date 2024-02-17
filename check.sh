#!/bin/bash
# write_mailmap > CONTRIBUTORS
go mod tidy
go build ./
golangci-lint run ./...
