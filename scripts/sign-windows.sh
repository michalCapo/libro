#!/usr/bin/env bash
# Signs Windows programs with Azure Artifact Signing, so Windows shows DA Solutions s.r.o. as their publisher.
# Settings shared by all projects, in ~/.config/code-signing/azure.env (never in the repository):
#   SIGN_ENDPOINT=neu.codesigning.azure.net   # the signing account's region endpoint, without https://
#   SIGN_ACCOUNT=<signing account name>
#   SIGN_PROFILE=<certificate profile name>
# Tools: jsign (7+) and the Azure CLI, signed in with `az login`.
# No settings file: warns and leaves the files unsigned (the release still works).
set -euo pipefail

name=Libro
url=https://github.com/michalCapo/libro
(($#)) || { echo "usage: sign-windows.sh <file.exe>..." >&2; exit 2; }
conf=${SIGN_CONFIG:-$HOME/.config/code-signing/azure.env}
if [[ ! -f "$conf" ]]; then
  echo "!! not signed, no $conf (Windows will show 'Unknown publisher'): $*" >&2
  exit 0
fi
# shellcheck source=/dev/null
source "$conf"
: "${SIGN_ENDPOINT:?missing in $conf}" "${SIGN_ACCOUNT:?missing in $conf}" "${SIGN_PROFILE:?missing in $conf}"
for tool in jsign az; do
  command -v "$tool" >/dev/null 2>&1 || { echo "required tool not found: $tool" >&2; exit 1; }
done

# a short-lived token from `az login`; passed through the environment, not the command line
SIGN_TOKEN=$(az account get-access-token --resource https://codesigning.azure.net --query accessToken -o tsv)
export SIGN_TOKEN
jsign --storetype TRUSTEDSIGNING --keystore "$SIGN_ENDPOINT" --storepass env:SIGN_TOKEN \
  --alias "$SIGN_ACCOUNT/$SIGN_PROFILE" --tsaurl http://timestamp.acs.microsoft.com --tsmode RFC3161 \
  --name "$name" ${url:+--url "$url"} "$@"
echo "Signed $*"
