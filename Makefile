all:
	mkdir -p ./bin && \
		go build -o bin/openshift-must-gather cmd/must-gather.go && \
		go build -o bin/openshift-dev-helpers dev-helpers/cmd/dev-helpers.go

update-deps:
	glide update --strip-vendor
