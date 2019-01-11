all:
	mkdir -p ./bin && go build -o ./bin/openshift-must-gather cmd/must-gather.go
