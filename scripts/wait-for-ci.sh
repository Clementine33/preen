#!/usr/bin/env bash
# Require the tagged commit's main push CI, including its aggregate result job.
set -euo pipefail

commit=${1:?commit SHA is required}
run_id=''
# A simultaneous main/tag push may register Release before CI is visible.
for ((attempt = 0; attempt < 12; attempt++)); do
  run_id=$(gh run list --workflow ci.yml --commit "$commit" --branch main --event push \
    --limit 1 --json databaseId --jq '.[0].databaseId // empty')
  if [ -n "$run_id" ]; then break; fi
  sleep 5
done
if [ -z "$run_id" ]; then
  echo "No main push CI found for $commit; run CI on main, then rerun Release." >&2
  exit 1
fi
gh run watch "$run_id" --exit-status --interval 15
test "$(gh run view "$run_id" --json conclusion --jq .conclusion)" = success
