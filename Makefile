install:
	go install -v

fmt:
	go fmt
	cd ./cmd/client && go fmt
	cd ./cmd/server && go fmt

HOSTNAME ?= localhost
certs:
	mkdir -p ./certs
	openssl ecparam \
	    -name secp256r1 \
	    -genkey \
	    -out ./certs/server.key
	openssl req \
		-new -x509 \
		-key ./certs/server.key \
		-out ./certs/server.cert \
		-days 90 \
		-subj /CN=$(HOSTNAME)

protoc:
	protoc \
		./commands.proto \
		--go_out=.

.PHONY: fmt install protoc certs
