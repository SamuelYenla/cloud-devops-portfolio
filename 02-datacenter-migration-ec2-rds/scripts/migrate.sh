#!/usr/bin/env bash
# Migrate the wiki database from the simulated data center to RDS.
#
#   ./migrate.sh dry-run    dump and restore, compare, leave the DC serving
#   ./migrate.sh cutover    final dump, restore, point the app at RDS
#   ./migrate.sh rollback   point the app back at the data center
#   ./migrate.sh status     show which database the app is currently using
#
# Run from a host inside the target VPC (an app instance via SSM Session
# Manager), since the data center is reachable only over the peering link.
set -euo pipefail

STACK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../terraform/aws" && pwd)"
DUMP_DIR="${DUMP_DIR:-/tmp/wiki-migration}"

tf() { terraform -chdir="$STACK_DIR" output -raw "$1"; }

log() { printf '\n[%s] %s\n' "$(date +%H:%M:%S)" "$*"; }
die() { printf '\nERROR: %s\n' "$*" >&2; exit 1; }

load_config() {
  REGION="${AWS_REGION:-us-east-1}"
  DC_HOST="$(tf dc_host_private_ip)"
  RDS_HOST="$(tf rds_endpoint)"
  HOST_PARAM="$(tf db_host_parameter)"
  PASS_PARAM="$(tf db_password_parameter)"
  ASG_NAME="$(tf autoscaling_group_name)"

  DB_NAME="${DB_NAME:-wiki}"
  DB_USER="${DB_USER:-wiki}"
  # The data center credential is the throwaway one set in its tfvars.
  DC_PASSWORD="${DC_PASSWORD:-wiki-dc-password}"
  RDS_PASSWORD="$(aws ssm get-parameter --region "$REGION" --with-decryption \
    --name "$PASS_PARAM" --query 'Parameter.Value' --output text)"
}

# Row counts plus a content checksum per table. Counts alone would not catch
# truncated or mangled rows, which is the failure mode that matters.
fingerprint() {
  local host="$1" password="$2"
  mysql -h "$host" -u "$DB_USER" -p"$password" "$DB_NAME" -N -B <<'SQL'
SELECT 'pages', COUNT(*), COALESCE(SUM(CRC32(CONCAT_WS('|', slug, title, body, author))), 0) FROM pages
UNION ALL
SELECT 'users', COUNT(*), COALESCE(SUM(CRC32(CONCAT_WS('|', username, password_hash))), 0) FROM users;
SQL
}

dump_and_restore() {
  mkdir -p "$DUMP_DIR"
  local dump="$DUMP_DIR/wiki-$(date +%Y%m%d-%H%M%S).sql"

  log "Dumping $DB_NAME from the data center at $DC_HOST"
  local start=$SECONDS
  # --single-transaction keeps InnoDB consistent without locking the source,
  # so the DC keeps serving during a dry run.
  mysqldump -h "$DC_HOST" -u "$DB_USER" -p"$DC_PASSWORD" \
    --single-transaction --routines --triggers --events \
    --set-gtid-purged=OFF --no-tablespaces \
    "$DB_NAME" > "$dump"
  log "Dumped $(wc -c < "$dump" | tr -d ' ') bytes in $((SECONDS - start))s"

  log "Restoring into RDS at $RDS_HOST"
  start=$SECONDS
  mysql -h "$RDS_HOST" -u "$DB_USER" -p"$RDS_PASSWORD" "$DB_NAME" < "$dump"
  log "Restored in $((SECONDS - start))s"
}

compare() {
  log "Comparing source and target"
  local src tgt
  src="$(fingerprint "$DC_HOST" "$DC_PASSWORD")"
  tgt="$(fingerprint "$RDS_HOST" "$RDS_PASSWORD")"

  printf '\n%-10s %-12s %-12s %s\n' TABLE SOURCE TARGET RESULT
  local ok=1
  while IFS=$'\t' read -r table count checksum; do
    local t_line t_count t_checksum
    t_line="$(echo "$tgt" | awk -v t="$table" '$1 == t')"
    t_count="$(echo "$t_line" | cut -f2)"
    t_checksum="$(echo "$t_line" | cut -f3)"

    if [[ "$count" == "$t_count" && "$checksum" == "$t_checksum" ]]; then
      printf '%-10s %-12s %-12s OK\n' "$table" "$count" "$t_count"
    else
      printf '%-10s %-12s %-12s MISMATCH (checksum %s vs %s)\n' \
        "$table" "$count" "$t_count" "$checksum" "$t_checksum"
      ok=0
    fi
  done <<< "$src"

  [[ "$ok" == 1 ]] || die "Source and target do not match. Do not cut over."
  log "Source and target match"
}

point_app_at() {
  local host="$1"
  aws ssm put-parameter --region "$REGION" --name "$HOST_PARAM" \
    --value "$host" --type String --overwrite >/dev/null
  log "Parameter $HOST_PARAM set to $host"

  # The app reads the parameter at startup, so restarting the service is the
  # whole cutover. No new launch template version, no instance refresh.
  local ids
  ids="$(aws autoscaling describe-auto-scaling-groups --region "$REGION" \
    --auto-scaling-group-names "$ASG_NAME" \
    --query 'AutoScalingGroups[0].Instances[].InstanceId' --output text)"
  [[ -n "$ids" ]] || die "No instances found in $ASG_NAME"

  log "Restarting wiki on: $ids"
  aws ssm send-command --region "$REGION" \
    --document-name AWS-RunShellScript \
    --targets "Key=InstanceIds,Values=$(echo "$ids" | tr '\t' ',')" \
    --parameters 'commands=["systemctl restart wiki"]' \
    --query 'Command.CommandId' --output text
}

case "${1:-}" in
  dry-run)
    load_config
    dump_and_restore
    compare
    log "Dry run complete. The data center is still serving; nothing was switched."
    log "The dump/restore timings above are the cutover window to plan for."
    ;;

  cutover)
    load_config
    log "CUTOVER. Stop writes at the data center before continuing."
    read -r -p "Writes stopped? [yes/N] " reply
    [[ "$reply" == "yes" ]] || die "Aborted."

    dump_and_restore
    compare
    point_app_at "$RDS_HOST"
    log "Cutover complete. Every page now shows 'db: $RDS_HOST' in its header."
    log "Rollback stays available until the data center is destroyed."
    ;;

  rollback)
    load_config
    point_app_at "$DC_HOST"
    log "Rolled back to the data center. Writes made against RDS since cutover are NOT"
    log "replayed - re-run cutover to move forward again."
    ;;

  status)
    load_config
    current="$(aws ssm get-parameter --region "$REGION" --name "$HOST_PARAM" \
      --query 'Parameter.Value' --output text)"
    if [[ "$current" == "$RDS_HOST" ]]; then
      echo "Serving from RDS ($current) - migrated"
    elif [[ "$current" == "$DC_HOST" ]]; then
      echo "Serving from the data center ($current) - not yet migrated"
    else
      echo "Serving from an unrecognised host: $current"
    fi
    ;;

  *)
    sed -n '2,10p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
    exit 1
    ;;
esac
