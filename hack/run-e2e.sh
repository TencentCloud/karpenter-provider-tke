#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${NAMESPACE:-karpenter}"
REPORT_CM="${REPORT_CM:-e2e-test-report}"
TEST_SUITES="${TEST_SUITES:-integration lifecycle consolidation scheduling storage}"
TEST_TIMEOUT="${TEST_TIMEOUT:-2h}"
GINKGO_TIMEOUT="${GINKGO_TIMEOUT:-1.5h}"

mkdir -p /tmp/reports
OVERALL_EXIT=0

for suite in $TEST_SUITES; do
  binary="/usr/local/bin/e2e-${suite}"
  if [[ ! -f "$binary" ]]; then
    echo "⚠️  skip: $suite (binary not found)"
    continue
  fi

  echo "▶ Running suite: $suite"
  "$binary" \
    -test.v \
    -test.timeout="$TEST_TIMEOUT" \
    --ginkgo.junit-report="/tmp/reports/${suite}.xml" \
    --ginkgo.timeout="$GINKGO_TIMEOUT" \
    --ginkgo.grace-period=3m \
    --ginkgo.vv 2>&1 | tee "/tmp/reports/${suite}.log" || OVERALL_EXIT=1
done

# Generate Markdown report from JUnit XMLs
echo "📊 Generating Markdown report..."
TEST_SUITES="$TEST_SUITES" python3 /usr/local/bin/gen-report.py /tmp/reports > /tmp/reports/report.md
echo "--- report.md preview (first 40 lines) ---"
head -40 /tmp/reports/report.md
echo "---"

# Write results to ConfigMap.
# Only store report.md (markdown) and exit-code — avoids "Argument list too long"
# and etcd 1MB limit that would occur if we embedded the large JUnit XML files.
echo "📝 Writing report to ConfigMap ${NAMESPACE}/${REPORT_CM}"

kubectl delete configmap "$REPORT_CM" --namespace="$NAMESPACE" --ignore-not-found=true

kubectl create configmap "$REPORT_CM" \
  --namespace="$NAMESPACE" \
  --from-file=report.md=/tmp/reports/report.md \
  --from-literal=exit-code="$OVERALL_EXIT"

echo "✅ Done. Exit code: $OVERALL_EXIT"
exit $OVERALL_EXIT
