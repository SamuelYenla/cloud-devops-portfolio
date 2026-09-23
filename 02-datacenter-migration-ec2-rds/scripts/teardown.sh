#!/usr/bin/env bash
# Destroy both stacks, then check the account for survivors.
#
# This is the project's real cost control. At ~$2.65/day a forgotten stack
# takes about 19 days to trip the account's $50 budget alert, so the budget
# would tell you long after it mattered.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REGION="${AWS_REGION:-us-east-1}"
NAME="${NAME:-dc-migration}"

log() { printf '\n==> %s\n' "$*"; }

# Reverse of the apply order: the aws stack holds the peering connection and a
# route in the data center's route table, so it has to go first.
for stack in aws datacenter; do
  log "Destroying $stack"
  terraform -chdir="$ROOT/terraform/$stack" destroy -auto-approve \
    || log "WARNING: $stack destroy reported an error - check the survivors below"
done

log "Checking for survivors tagged Project=cloud-devops-portfolio"

found=0
report() {
  local label="$1" value="$2"
  if [[ -n "$value" && "$value" != "None" ]]; then
    printf '  LEFT RUNNING  %-18s %s\n' "$label" "$value"
    found=1
  fi
}

report "EC2 instances" "$(aws ec2 describe-instances --region "$REGION" \
  --filters "Name=tag:Name,Values=${NAME}-*" \
            "Name=instance-state-name,Values=running,pending,stopping,stopped" \
  --query 'Reservations[].Instances[].InstanceId' --output text)"

report "NAT gateways" "$(aws ec2 describe-nat-gateways --region "$REGION" \
  --filter "Name=state,Values=available,pending" \
  --query "NatGateways[?Tags[?Key=='Project'&&Value=='cloud-devops-portfolio']].NatGatewayId" \
  --output text)"

report "Load balancers" "$(aws elbv2 describe-load-balancers --region "$REGION" \
  --query "LoadBalancers[?starts_with(LoadBalancerName, '${NAME}')].LoadBalancerArn" \
  --output text)"

report "RDS instances" "$(aws rds describe-db-instances --region "$REGION" \
  --query "DBInstances[?starts_with(DBInstanceIdentifier, '${NAME}')].DBInstanceIdentifier" \
  --output text)"

# Unattached EIPs bill for doing nothing, which makes them easy to miss.
report "Unattached EIPs" "$(aws ec2 describe-addresses --region "$REGION" \
  --query 'Addresses[?AssociationId==null].PublicIp' --output text)"

report "Peering connections" "$(aws ec2 describe-vpc-peering-connections --region "$REGION" \
  --filters "Name=status-code,Values=active,pending-acceptance" \
  --query "VpcPeeringConnections[?Tags[?Key=='Project'&&Value=='cloud-devops-portfolio']].VpcPeeringConnectionId" \
  --output text)"

report "EBS volumes" "$(aws ec2 describe-volumes --region "$REGION" \
  --filters "Name=status,Values=available" \
  --query "Volumes[?Tags[?Key=='Project'&&Value=='cloud-devops-portfolio']].VolumeId" \
  --output text)"

if [[ "$found" == 0 ]]; then
  log "Clean. Nothing from this project is still billing."
else
  log "Survivors listed above are still costing money. Remove them before finishing."
  exit 1
fi
