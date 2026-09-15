.PHONY: fmt lint test vulncheck

fmt:
	go tool gofumpt -w .

lint:
	go tool staticcheck -checks=all -show-ignored -tests  ./...

test:
	go clean -testcache
	go test ./...

vulncheck:
	go tool govulncheck ./...
