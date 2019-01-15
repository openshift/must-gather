all:
	mkdir -p ./bin && go build -o openshift-must-gather cmd/must-gather.go
