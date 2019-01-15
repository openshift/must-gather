all:
	mkdir -p ./bin && go build -o ./bin/openshift-must-gather main.go
	cp plugins/cee-awesomeness/* bin
