#!/usr/bin/env bash

set -eu

failures=0
successes=0

cd "$(dirname "$0")"

exec_case() {
	local test_case="$1"
	local test_case_err="$1.err"
	local test_case_out="$1.out"
	local ret
	local output
	local err

	ret=0
	kubectl apply --dry-run=server -f "$test_case" > "$test_case_out" 2>&1 || ret=$?
	output=$(cat "$test_case_out")

	if [ $ret == 1 ] && [ ! -f "${test_case_err}" ]; then
		echo "${test_case}: FAIL $output"
		((++failures))
	elif [ $ret == 1 ] && [ -f "$test_case_err" ]; then
		err=$(cat "$test_case_err")
		if grep "$err" "$test_case_out" > /dev/null 2>&1; then
			((++successes))
			echo "$test_case: SUCCESS (expected failure)"
		else
			echo "$test_case: FAIL (unexpected msg): $output"
			((++failures))
		fi
	elif [ $ret == 0 ] && [ -f "$test_case_err" ]; then
		echo "$test_case: FAIL (unexpected success): $output"
		((++failures))
	elif [ $ret == 0 ] && [ ! -f "$test_case_err" ]; then
		echo "$test_case: SUCCESS"
		((++successes))
	fi
}

while IFS= read -r test_case; do
	exec_case "$test_case"
done < <(find cel-tests -name \*.yaml)

echo
echo "SUCCESS: ${successes}"
echo "FAILURES: ${failures}"

if [ "${failures}" != 0 ]; then
	exit 1
fi
