#!/bin/sh
# Registers the site's Application in the account center and gives the server its sign-in
# settings. Run once, as root, on the server:  sh register-application.sh
#
# The client secret and the session key are made here and written only to api.env; nothing
# secret is printed. The secret is stored hashed in the account center and cannot be read
# back: to replace it, reset it in the account center's admin and edit api.env.
#
# Afterwards recreate the container so that it reads the new settings:
#   docker compose -f /opt/ourtaiko-net/compose.production.yml up -d --wait --no-deps --force-recreate net
set -eu
umask 077

env=/etc/ourtaiko-net/docker/api.env
sso=ourtaiko-sso
client=OurTaikoNet
origin=$(sed -n 's/^APP_ORIGIN=//p' "$env")
issuer=https://sso.ourtaiko.org
avatars=https://distrib.ourtaiko.org/sso/avatars

[ -n "$origin" ] || { echo "APP_ORIGIN is not in $env" >&2; exit 1; }
if grep -q '^SSO_' "$env"; then
  echo "$env already has sign-in settings; nothing done" >&2
  exit 1
fi

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
secret=$(openssl rand -hex 32)
session=$(openssl rand -hex 32)
printf 'NET_CLIENT_ID=%s\nNET_CLIENT_SECRET=%s\nNET_REDIRECT_URI=%s\n' \
  "$client" "$secret" "$origin/api/v1/auth/sso/callback" > "$work/exec.env"

cat > "$work/register.py" <<'PY'
import os
from django.contrib.auth.hashers import check_password
from django.db import transaction
from oauth2_provider.models import Application
from accounts.models import ApplicationAccess

client_id = os.environ["NET_CLIENT_ID"]
secret = os.environ["NET_CLIENT_SECRET"]
redirect = os.environ["NET_REDIRECT_URI"]
if len(secret) != 64 or not redirect.startswith("https://"):
    raise SystemExit("refusing: a 64-character secret and an HTTPS callback are needed")
with transaction.atomic():
    if Application.objects.filter(client_id=client_id).exists():
        raise SystemExit("refusing: an Application with this client ID exists already")
    app = Application.objects.create(
        client_id=client_id,
        name="OurTaikoNET",
        client_secret=secret,
        client_type=Application.CLIENT_CONFIDENTIAL,
        authorization_grant_type=Application.GRANT_AUTHORIZATION_CODE,
        algorithm="RS256",
        redirect_uris=redirect,
        skip_authorization=False,
    )
    # The website signs in, asks about its tokens and reads profiles; the game signs in and
    # exchanges its account token for a session of this Application.
    ApplicationAccess.objects.update_or_create(
        application=app,
        defaults={
            "enabled": True,
            "allow_game": True,
            "allow_profiles": True,
            "allow_web": True,
            "allow_account_tokens": True,
            "allow_external_link": False,
        },
    )
    app.refresh_from_db()
    if app.client_secret == secret or not check_password(secret, app.client_secret):
        raise SystemExit("refusing: the secret was not stored hashed as expected")
print("registered Application", app.id, app.client_id, "callback", app.redirect_uris)
PY

docker exec -i --env-file "$work/exec.env" "$sso" python manage.py shell < "$work/register.py" 2>&1 |
  grep -v 'objects imported' > "$work/out.txt" || true
cat "$work/out.txt"
grep -q '^registered Application' "$work/out.txt" || { echo "not registered; $env is unchanged" >&2; exit 1; }

cp -p "$env" "$env.before-sso"
cat >> "$env" <<SETTINGS

# The account center: one Application for the whole site (registered $(date -u +%Y-%m-%d)).
SSO_ISSUER=$issuer
SSO_SERVICE_ID=$client
SSO_SERVICE_KEY=$secret
SSO_CLIENT_ID=$client
SSO_CLIENT_SECRET=$secret
SSO_REDIRECT_URL=$origin/api/v1/auth/sso/callback
SSO_AVATAR_BASE_URL=$avatars
SESSION_ENCRYPTION_KEY=$session
SETTINGS
chmod 600 "$env" "$env.before-sso"
echo "settings written; names now in $env:"
cut -s -d= -f1 "$env" | grep -v '^#' | tr '\n' ' '
echo
