PROJECT := github.com/juju/retry

default: check

check-licence:
	@(fgrep -rl "Licensed under the LGPLv3" .;\
		fgrep -rl "MACHINE GENERATED BY THE COMMAND ABOVE; DO NOT EDIT" .;\
		find . -name "*.go") | sed -e 's,\./,,' | sort | uniq -u | \
		xargs -I {} echo FAIL: licence missed: {}

check: check-licence
	go test $(PROJECT)/...