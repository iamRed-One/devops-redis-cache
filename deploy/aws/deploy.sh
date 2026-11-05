#!/usr/bin/env bash
# Deploys cache-demo to AWS (run from Git Bash):
#
#   Browser --HTTPS--> CloudFront --+--> S3 (private, OAC)        static frontend
#                                   |
#                                   +--> EC2 :8080 (Go binary)    /weather/* and /reset
#                                           |
#                                           +--> ElastiCache Redis (same VPC, port 6379)
#
# Safe to re-run: finished steps are recorded in deploy/aws/.state and skipped.
# Delete everything with deploy/aws/teardown.sh.
#
# Optional env vars:
#   REGION=eu-west-1            ALERT_EMAIL=you@example.com
set -euo pipefail

export MSYS_NO_PATHCONV=1          # stop Git Bash rewriting /paths passed to aws.exe
export AWS_PAGER=""
REGION="${REGION:-eu-west-1}"
export AWS_DEFAULT_REGION="$REGION" # overrides whatever is in ~/.aws/config
ALERT_EMAIL="${ALERT_EMAIL:-oladelebidemiweb@gmail.com}"
NAME=cache-demo
TAGS="Key=project,Value=$NAME"

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
WORK="$HERE/.work"; mkdir -p "$WORK"
STATE="$HERE/.state"; touch "$STATE"
cd "$WORK"   # keep file:// and s3 paths relative; aws.exe cannot read /c/... paths

get() { grep -E "^$1=" "$STATE" | tail -1 | cut -d= -f2- || true; }
put() { echo "$1=$2" >> "$STATE"; }
say() { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }

ACCOUNT="$(aws sts get-caller-identity --query Account --output text)"
ART_BUCKET="$NAME-$ACCOUNT-artifacts"
WEB_BUCKET="$NAME-$ACCOUNT-web"
say "Account $ACCOUNT, region $REGION"

# ---------------------------------------------------------------- 1. budget alert
if [ -z "$(get budget)" ]; then
  say "1/9 Budget alert -> $ALERT_EMAIL (alerts on GROSS usage, even though credits pay for it)"
  cat > budget.json <<EOF
{"BudgetName":"$NAME-guard","BudgetLimit":{"Amount":"25","Unit":"USD"},"TimeUnit":"MONTHLY","BudgetType":"COST",
 "CostTypes":{"IncludeCredit":false,"IncludeRefund":false,"IncludeTax":true,"IncludeSubscription":true,"UseBlended":false}}
EOF
  cat > notif.json <<EOF
[{"Notification":{"NotificationType":"ACTUAL","ComparisonOperator":"GREATER_THAN","Threshold":40,"ThresholdType":"PERCENTAGE"},
  "Subscribers":[{"SubscriptionType":"EMAIL","Address":"$ALERT_EMAIL"}]},
 {"Notification":{"NotificationType":"ACTUAL","ComparisonOperator":"GREATER_THAN","Threshold":100,"ThresholdType":"PERCENTAGE"},
  "Subscribers":[{"SubscriptionType":"EMAIL","Address":"$ALERT_EMAIL"}]}]
EOF
  aws budgets create-budget --region us-east-1 --account-id "$ACCOUNT" \
    --budget file://budget.json --notifications-with-subscribers file://notif.json
  put budget done
fi

# ---------------------------------------------------------------- 2. network + security groups
say "2/9 Security groups (default VPC)"
VPC="$(aws ec2 describe-vpcs --filters Name=isDefault,Values=true --query 'Vpcs[0].VpcId' --output text)"
SUBNETS="$(aws ec2 describe-subnets --filters Name=vpc-id,Values="$VPC" Name=default-for-az,Values=true --query 'Subnets[].SubnetId' --output text)"
SUBNET="$(echo "$SUBNETS" | awk '{print $1}')"
if [ -z "$(get sg_app)" ]; then
  SG_APP="$(aws ec2 create-security-group --group-name "$NAME-app" --description "cache-demo backend, CloudFront only" \
    --vpc-id "$VPC" --tag-specifications "ResourceType=security-group,Tags=[{$TAGS}]" --query GroupId --output text)"
  PL="$(aws ec2 describe-managed-prefix-lists --filters Name=prefix-list-name,Values=com.amazonaws.global.cloudfront.origin-facing \
    --query 'PrefixLists[0].PrefixListId' --output text)"
  # Only CloudFront's own IP ranges can reach the backend port. No SSH at all (use SSM if you need a shell).
  aws ec2 authorize-security-group-ingress --group-id "$SG_APP" \
    --ip-permissions "IpProtocol=tcp,FromPort=8080,ToPort=8080,PrefixListIds=[{PrefixListId=$PL}]" >/dev/null
  put sg_app "$SG_APP"
fi
if [ -z "$(get sg_cache)" ]; then
  SG_CACHE="$(aws ec2 create-security-group --group-name "$NAME-cache" --description "cache-demo redis, backend only" \
    --vpc-id "$VPC" --tag-specifications "ResourceType=security-group,Tags=[{$TAGS}]" --query GroupId --output text)"
  # Redis accepts connections only from the backend's security group.
  aws ec2 authorize-security-group-ingress --group-id "$SG_CACHE" \
    --ip-permissions "IpProtocol=tcp,FromPort=6379,ToPort=6379,UserIdGroupPairs=[{GroupId=$(get sg_app)}]" >/dev/null
  put sg_cache "$SG_CACHE"
fi
SG_APP="$(get sg_app)"; SG_CACHE="$(get sg_cache)"

# ---------------------------------------------------------------- 3. ElastiCache (starts now, takes ~5-10 min)
if [ -z "$(get cache)" ]; then
  say "3/9 ElastiCache Redis cache.t4g.micro (provisioning in background)"
  aws elasticache create-cache-subnet-group --cache-subnet-group-name "$NAME" \
    --cache-subnet-group-description "$NAME" --subnet-ids $SUBNETS >/dev/null
  aws elasticache create-cache-cluster --cache-cluster-id "$NAME" --engine redis --engine-version 7.1 \
    --cache-node-type cache.t4g.micro --num-cache-nodes 1 --cache-subnet-group-name "$NAME" \
    --security-group-ids "$SG_CACHE" --tags "$TAGS" >/dev/null
  put cache started
fi

# ---------------------------------------------------------------- 4. IAM role for the EC2 box
if [ -z "$(get role)" ]; then
  say "4/9 IAM role (read the binary from S3, allow SSM shell)"
  cat > trust.json <<EOF
{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}
EOF
  cat > s3read.json <<EOF
{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"arn:aws:s3:::$ART_BUCKET/*"}]}
EOF
  aws iam create-role --role-name "$NAME-ec2" --assume-role-policy-document file://trust.json --tags "$TAGS" >/dev/null
  aws iam attach-role-policy --role-name "$NAME-ec2" --policy-arn arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore
  aws iam put-role-policy --role-name "$NAME-ec2" --policy-name s3-read-artifacts --policy-document file://s3read.json
  aws iam create-instance-profile --instance-profile-name "$NAME-ec2" >/dev/null
  aws iam add-role-to-instance-profile --instance-profile-name "$NAME-ec2" --role-name "$NAME-ec2"
  put role done
fi

# ---------------------------------------------------------------- 5. build + upload backend
say "5/9 Build Linux/arm64 binary and upload to S3"
( cd "$ROOT" && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o deploy/aws/.work/cache-demo-linux . )
if [ -z "$(get art_bucket)" ]; then
  aws s3api create-bucket --bucket "$ART_BUCKET" --region "$REGION" \
    --create-bucket-configuration LocationConstraint="$REGION" >/dev/null
  aws s3api put-public-access-block --bucket "$ART_BUCKET" \
    --public-access-block-configuration BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true
  put art_bucket "$ART_BUCKET"
fi
aws s3 cp cache-demo-linux "s3://$ART_BUCKET/cache-demo-linux" --only-show-errors

# ---------------------------------------------------------------- 6. EC2
say "6/9 Waiting for ElastiCache to become available..."
aws elasticache wait cache-cluster-available --cache-cluster-id "$NAME"
REDIS_HOST="$(aws elasticache describe-cache-clusters --cache-cluster-id "$NAME" --show-cache-node-info \
  --query 'CacheClusters[0].CacheNodes[0].Endpoint.Address' --output text)"
echo "    redis endpoint: $REDIS_HOST"

if [ -z "$(get instance)" ]; then
  say "6/9 Launching EC2 t4g.micro"
  cat > user-data.sh <<EOF
#!/bin/bash
set -e
aws s3 cp s3://$ART_BUCKET/cache-demo-linux /usr/local/bin/cache-demo --region $REGION
chmod +x /usr/local/bin/cache-demo
cat > /etc/cache-demo.env <<ENV
PORT=8080
REDIS_URL=redis://$REDIS_HOST:6379
ENV
chmod 600 /etc/cache-demo.env
cat > /etc/systemd/system/cache-demo.service <<UNIT
[Unit]
Description=cache-demo backend
After=network-online.target
[Service]
EnvironmentFile=/etc/cache-demo.env
ExecStart=/usr/local/bin/cache-demo
Restart=always
[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable --now cache-demo
EOF
  AMI="$(aws ssm get-parameter --name /aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64 \
    --query Parameter.Value --output text)"
  for i in 1 2 3 4 5 6; do   # IAM instance profiles take a few seconds to propagate
    if IID="$(aws ec2 run-instances --image-id "$AMI" --instance-type t4g.micro --subnet-id "$SUBNET" \
        --security-group-ids "$SG_APP" --iam-instance-profile Name="$NAME-ec2" --user-data file://user-data.sh \
        --metadata-options HttpTokens=required \
        --tag-specifications "ResourceType=instance,Tags=[{Key=Name,Value=$NAME},{$TAGS}]" \
        --query 'Instances[0].InstanceId' --output text 2>run.err)"; then break; fi
    echo "    retrying ($(head -c 120 run.err))"; sleep 10
  done
  [ -n "${IID:-}" ] || { cat run.err; exit 1; }
  put instance "$IID"
fi
IID="$(get instance)"
aws ec2 wait instance-running --instance-ids "$IID"
EC2_DNS="$(aws ec2 describe-instances --instance-ids "$IID" --query 'Reservations[0].Instances[0].PublicDnsName' --output text)"
echo "    backend: $EC2_DNS"

# ---------------------------------------------------------------- 7. S3 + CloudFront
say "7/9 S3 (private) + CloudFront"
if [ -z "$(get web_bucket)" ]; then
  aws s3api create-bucket --bucket "$WEB_BUCKET" --region "$REGION" \
    --create-bucket-configuration LocationConstraint="$REGION" >/dev/null
  aws s3api put-public-access-block --bucket "$WEB_BUCKET" \
    --public-access-block-configuration BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true
  put web_bucket "$WEB_BUCKET"
fi
if [ -z "$(get oac)" ]; then
  put oac "$(aws cloudfront create-origin-access-control --origin-access-control-config \
    "Name=$NAME-oac,SigningProtocol=sigv4,SigningBehavior=always,OriginAccessControlOriginType=s3" \
    --query OriginAccessControl.Id --output text)"
fi
if [ -z "$(get cf_id)" ]; then
  # Managed policies: CachingOptimized (static site), CachingDisabled (API), AllViewerExceptHostHeader.
  API_BEHAVIOR='"TargetOriginId":"ec2-api","ViewerProtocolPolicy":"redirect-to-https","Compress":false,
    "CachePolicyId":"4135ea2d-6df8-44a3-9df3-4b5a84be39ad","OriginRequestPolicyId":"b689b0a8-53d0-40ab-baf2-68738e2966ac",
    "AllowedMethods":{"Quantity":3,"Items":["GET","HEAD","OPTIONS"]}'
  cat > cf.json <<EOF
{
 "CallerReference":"$NAME-$(date +%s)","Comment":"$NAME","Enabled":true,"DefaultRootObject":"index.html",
 "Origins":{"Quantity":2,"Items":[
  {"Id":"s3-web","DomainName":"$WEB_BUCKET.s3.$REGION.amazonaws.com","OriginAccessControlId":"$(get oac)",
   "S3OriginConfig":{"OriginAccessIdentity":""}},
  {"Id":"ec2-api","DomainName":"$EC2_DNS",
   "CustomOriginConfig":{"HTTPPort":8080,"HTTPSPort":443,"OriginProtocolPolicy":"http-only","OriginReadTimeout":30,"OriginKeepaliveTimeout":5}}
 ]},
 "DefaultCacheBehavior":{"TargetOriginId":"s3-web","ViewerProtocolPolicy":"redirect-to-https","Compress":true,
   "CachePolicyId":"658327ea-f89d-4fab-a63d-7e88639e58f6","AllowedMethods":{"Quantity":2,"Items":["GET","HEAD"]}},
 "CacheBehaviors":{"Quantity":2,"Items":[
  {"PathPattern":"/weather/*",$API_BEHAVIOR},
  {"PathPattern":"/reset",$API_BEHAVIOR}
 ]}
}
EOF
  CF="$(aws cloudfront create-distribution --distribution-config file://cf.json --query 'Distribution.[Id,DomainName]' --output text)"
  put cf_id "$(echo "$CF" | cut -f1)"; put cf_domain "$(echo "$CF" | cut -f2)"
  # Only this distribution may read the bucket.
  cat > bucket-policy.json <<EOF
{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"cloudfront.amazonaws.com"},
 "Action":"s3:GetObject","Resource":"arn:aws:s3:::$WEB_BUCKET/*",
 "Condition":{"StringEquals":{"AWS:SourceArn":"arn:aws:cloudfront::$ACCOUNT:distribution/$(get cf_id)"}}}]}
EOF
  aws s3api put-bucket-policy --bucket "$WEB_BUCKET" --policy file://bucket-policy.json
fi
CF_ID="$(get cf_id)"; CF_DOMAIN="$(get cf_domain)"

# ---------------------------------------------------------------- 8. frontend
say "8/9 Build + upload frontend (API = https://$CF_DOMAIN)"
( cd "$ROOT/frontend" && VITE_API_URL="https://$CF_DOMAIN" pnpm run build \
  && aws s3 sync dist "s3://$WEB_BUCKET" --delete --only-show-errors )
aws cloudfront create-invalidation --distribution-id "$CF_ID" --paths '/*' >/dev/null

# ---------------------------------------------------------------- 9. verify
say "9/9 Waiting for CloudFront to deploy (5-10 min)..."
aws cloudfront wait distribution-deployed --id "$CF_ID"
echo "    waiting for the backend to answer..."
for i in $(seq 1 30); do
  curl -fs "https://$CF_DOMAIN/reset" >/dev/null 2>&1 && break || sleep 10
done
echo; echo "no-cache x2:"; for i in 1 2; do curl -s "https://$CF_DOMAIN/weather/no-cache" | grep -E '"(source|duration_ms)"' | tr -d '\n'; echo; done
echo "cached x4 (first = miss, rest = hits):"; for i in 1 2 3 4; do curl -s "https://$CF_DOMAIN/weather/cached" | grep -E '"(source|duration_ms)"' | tr -d '\n'; echo; done
say "DONE  ->  https://$CF_DOMAIN"
