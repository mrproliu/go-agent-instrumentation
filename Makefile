REPODIR := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

.PHONY: examples
examples:
	cd ${REPODIR}/cmd && go build .
	cd ${REPODIR}/examples && go build -a -work -toolexec ${REPODIR}/cmd/cmd .
	examples/examples

.PHONY: test
test:
	cd ${REPODIR}/cmd && go build .
	cd ${REPODIR}/test && go build -a -work -toolexec ${REPODIR}/cmd/cmd .
	test/test