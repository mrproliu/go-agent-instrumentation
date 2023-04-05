REPODIR := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

.PHONY: test
test:
	cd ${REPODIR}/cmd && go build .
	cd ${REPODIR}/test && go build -a -work -toolexec ${REPODIR}/cmd/cmd .
	test/test
