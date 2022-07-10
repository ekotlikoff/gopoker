webclient_package := github.com/ekotlikoff/gopoker/internal/client/web

all: vet test testrace

clean:
	go clean -i github.com/ekotlikoff/gopoker/...

vet:
	./vet.sh -install
	./vet.sh

test:
	go test -cpu 1,4 -timeout 7m github.com/ekotlikoff/gopoker/...

testrace:
	go test -race -cpu 1,4 -timeout 7m github.com/ekotlikoff/gopoker/...

.PHONY: \
	all \
	clean \
	test \
	testrace \
	vet \
