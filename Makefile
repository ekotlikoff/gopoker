webclient_package := github.com/ekotlikoff/gopoker/internal/client/web
run_local_package := github.com/ekotlikoff/gopoker/cmd/gopoker

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

web:
	GOARCH=wasm GOOS=js go build \
		-o ~/bin/gopokerclient.wasm \
		-tags webclient $(webclient_package)

runweb: web
	go run $(run_local_package)

.PHONY: \
	all \
	clean \
	test \
	testrace \
	vet \
	web \
	runweb \
