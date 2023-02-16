set -e -u -o pipefail

readonly SCRIPT_DIR=$(dirname "$0")
readonly ARCHITECTURE=$( [[ "$(uname -p)" == "unknown" ]] && echo "$(uname -m)" || echo "$(uname -p)" ) #ternary operator

if [ $ARCHITECTURE == "arm64" ] || [ $ARCHITECTURE == "aarch64" ]; then
     mv -f "$SCRIPT_DIR/extension-launcher-arm64" "$SCRIPT_DIR/extension-launcher"
     mv -f "$SCRIPT_DIR/vm-application-manager-arm64" "$SCRIPT_DIR/vm-application-manager"
else
    rm -f "$SCRIPT_DIR/extension-launcher-arm64"
    rm -f "$SCRIPT_DIR/vm-application-manager-arm64"
fi

"$SCRIPT_DIR/vm-application-manager" install