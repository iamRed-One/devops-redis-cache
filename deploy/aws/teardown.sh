#!/usr/bin/env bash
# Deletes everything deploy.sh created, so AWS stops billing. Safe to re-run.
set -uo pipefail
export MSYS_NO_PATHCONV=1 AWS_PAGER=""
REGION="${REGION:-eu-west-1}"; export AWS_DEFAULT_REGION="$REGION"
NAME=cache-demo
HERE="$(cd "$(dirname "$0")" && pwd)"; STATE="$HERE/.state"; cd "$HERE"
get() { grep -E "^$1=" "$STATE" 2>/dev/null | tail -1 | cut -d= -f2- || true; }
say() { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }
ACCOUNT="$(aws sts get-caller-identity --query Account --output text)"

CF_ID="$(get cf_id)"
if [ -n "$CF_ID" ]; then
  say "CloudFront: disabling $CF_ID (deletion needs it fully disabled first, ~5-10 min)"
  aws cloudfront get-distribution-config --id "$CF_ID" > .work/cfget.json
  ETAG="$(python -c "import json;print(json.load(open('.work/cfget.json'))['ETag'])" 2>/dev/null || node -e "console.log(require('./.work/cfget.json').ETag)")"
  node -e "const c=require('./.work/cfget.json').DistributionConfig;c.Enabled=false;require('fs').writeFileSync('.work/cfoff.json',JSON.stringify(c))"
  aws cloudfront update-distribution --id "$CF_ID" --if-match "$ETAG" --distribution-config file://.work/cfoff.json >/dev/null
  aws cloudfront wait distribution-deployed --id "$CF_ID"
  ETAG="$(aws cloudfront get-distribution --id "$CF_ID" --query ETag --output text)"
  aws cloudfront delete-distribution --id "$CF_ID" --if-match "$ETAG"
fi
OAC="$(get oac)"; [ -n "$OAC" ] && aws cloudfront delete-origin-access-control --id "$OAC" --if-match "$(aws cloudfront get-origin-access-control --id "$OAC" --query ETag --output text)"

IID="$(get instance)"
if [ -n "$IID" ]; then say "EC2: terminating $IID"; aws ec2 terminate-instances --instance-ids "$IID" >/dev/null; aws ec2 wait instance-terminated --instance-ids "$IID"; fi

say "ElastiCache: deleting"
aws elasticache delete-cache-cluster --cache-cluster-id "$NAME" >/dev/null 2>&1 && aws elasticache wait cache-cluster-deleted --cache-cluster-id "$NAME"
aws elasticache delete-cache-subnet-group --cache-subnet-group-name "$NAME" 2>/dev/null

say "Security groups"
for k in sg_cache sg_app; do g="$(get $k)"; [ -n "$g" ] && aws ec2 delete-security-group --group-id "$g"; done

say "S3 buckets"
for b in "$NAME-$ACCOUNT-web" "$NAME-$ACCOUNT-artifacts"; do aws s3 rb "s3://$b" --force 2>/dev/null; done

say "IAM"
aws iam remove-role-from-instance-profile --instance-profile-name "$NAME-ec2" --role-name "$NAME-ec2" 2>/dev/null
aws iam delete-instance-profile --instance-profile-name "$NAME-ec2" 2>/dev/null
aws iam detach-role-policy --role-name "$NAME-ec2" --policy-arn arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore 2>/dev/null
aws iam delete-role-policy --role-name "$NAME-ec2" --policy-name s3-read-artifacts 2>/dev/null
aws iam delete-role --role-name "$NAME-ec2" 2>/dev/null

say "Budget"
aws budgets delete-budget --region us-east-1 --account-id "$ACCOUNT" --budget-name "$NAME-guard" 2>/dev/null

rm -f "$STATE"; rm -rf "$HERE/.work"
say "Teardown complete. Verify in the console: EC2, ElastiCache, CloudFront, S3."
