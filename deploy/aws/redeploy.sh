#!/usr/bin/env bash
# Pushes code changes to an ALREADY deployed stack (after deploy.sh has run once):
#   1. rebuilds the Go binary, uploads it to S3, restarts the service on EC2 (via SSM)
#   2. makes sure CloudFront routes /weather/* and /reset to the backend
#   3. rebuilds the frontend and uploads it to S3, then invalidates CloudFront
set -euo pipefail
export MSYS_NO_PATHCONV=1 AWS_PAGER=""
REGION="${REGION:-eu-west-1}"; export AWS_DEFAULT_REGION="$REGION"
NAME=cache-demo
HERE="$(cd "$(dirname "$0")" && pwd)"; ROOT="$(cd "$HERE/../.." && pwd)"
STATE="$HERE/.state"; WORK="$HERE/.work"; mkdir -p "$WORK"; cd "$WORK"
get() { grep -E "^$1=" "$STATE" | tail -1 | cut -d= -f2- || true; }
say() { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }

ART_BUCKET="$(get art_bucket)"; WEB_BUCKET="$(get web_bucket)"; IID="$(get instance)"
CF_ID="$(get cf_id)"; CF_DOMAIN="$(get cf_domain)"
[ -n "$IID" ] && [ -n "$CF_ID" ] || { echo "Run deploy.sh first."; exit 1; }

say "1/3 Backend: build, upload, restart on EC2"
( cd "$ROOT" && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o deploy/aws/.work/cache-demo-linux . )
aws s3 cp cache-demo-linux "s3://$ART_BUCKET/cache-demo-linux" --only-show-errors
cat > ssm.json <<EOF
{"commands":[
 "aws s3 cp s3://$ART_BUCKET/cache-demo-linux /usr/local/bin/cache-demo.new --region $REGION",
 "chmod +x /usr/local/bin/cache-demo.new",
 "mv -f /usr/local/bin/cache-demo.new /usr/local/bin/cache-demo",
 "systemctl restart cache-demo",
 "sleep 2 && systemctl is-active cache-demo"]}
EOF
CMD="$(aws ssm send-command --instance-ids "$IID" --document-name AWS-RunShellScript \
  --parameters file://ssm.json --query Command.CommandId --output text)"
for i in $(seq 1 30); do
  ST="$(aws ssm get-command-invocation --command-id "$CMD" --instance-id "$IID" --query Status --output text 2>/dev/null || echo Pending)"
  case "$ST" in Success) break;; Failed|Cancelled|TimedOut) echo "SSM command $ST"; \
    aws ssm get-command-invocation --command-id "$CMD" --instance-id "$IID" --query StandardErrorContent --output text; exit 1;; esac
  sleep 3
done
echo "    service restarted ($ST)"

say "2/3 CloudFront: route /weather/* (was /profile/*)"
aws cloudfront get-distribution-config --id "$CF_ID" > cfget.json
ETAG="$(node -e "console.log(require('./cfget.json').ETag)")"
CHANGED="$(node -e "
const c=require('./cfget.json').DistributionConfig;let n=0;
for(const b of c.CacheBehaviors.Items){if(b.PathPattern==='/profile/*'){b.PathPattern='/weather/*';n++}}
require('fs').writeFileSync('cfnew.json',JSON.stringify(c));console.log(n)")"
if [ "$CHANGED" != "0" ]; then
  aws cloudfront update-distribution --id "$CF_ID" --if-match "$ETAG" --distribution-config file://cfnew.json >/dev/null
  echo "    updated, waiting for CloudFront to deploy (5-10 min)..."
  aws cloudfront wait distribution-deployed --id "$CF_ID"
else
  echo "    already routing /weather/*"
fi

say "3/3 Frontend: build + upload"
( cd "$ROOT/frontend" && VITE_API_URL="https://$CF_DOMAIN" pnpm run build \
  && aws s3 sync dist "s3://$WEB_BUCKET" --delete --only-show-errors )
aws cloudfront create-invalidation --distribution-id "$CF_ID" --paths '/*' >/dev/null

say "Verify"
echo "no-cache x3:"; for i in 1 2 3; do curl -s "https://$CF_DOMAIN/weather/no-cache" | grep -E '"(source|duration_ms)"' | tr -d '\n'; echo; done
curl -s "https://$CF_DOMAIN/reset" >/dev/null
echo "cached x4 (first = miss, rest = hits):"; for i in 1 2 3 4; do curl -s "https://$CF_DOMAIN/weather/cached" | grep -E '"(source|duration_ms)"' | tr -d '\n'; echo; done
say "DONE  ->  https://$CF_DOMAIN"
