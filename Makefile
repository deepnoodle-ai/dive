
.PHONY: cover
cover:
	go test -coverprofile cover.out ./...
	go tool cover -html=cover.out
