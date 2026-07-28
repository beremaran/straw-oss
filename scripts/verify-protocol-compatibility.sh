#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fixture="$root/conformance/fixtures/v1/streaming.json"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

readarray -t wire_hex < <(python3 - "$fixture" <<'PY'
import json
import sys

fixture = json.load(open(sys.argv[1], encoding="utf-8"))["request_start"]
print(fixture["upstream_proxy"]["wire_hex"])
print(fixture["direct"]["wire_hex"])
PY
)
proxy_hex="${wire_hex[0]}"
direct_hex="${wire_hex[1]}"

mkdir -p "$tmp/go-old" "$tmp/go-new"

cat >"$tmp/go-old/go.mod" <<'EOF'
module straw-protocol-old-decoder

go 1.24.0

require (
  github.com/beremaran/straw-protos-go v0.3.0
  google.golang.org/protobuf v1.36.10
)
EOF
cat >"$tmp/go-old/main.go" <<'EOF'
package main

import (
	"encoding/hex"
	"fmt"
	"os"

	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
	"google.golang.org/protobuf/proto"
)

func main() {
	wire, err := hex.DecodeString(os.Args[1])
	if err != nil {
		panic(err)
	}
	var start strawpb.RequestStart
	if err := proto.Unmarshal(wire, &start); err != nil {
		panic(err)
	}
	if start.GetMethod() != "GET" || start.GetUrl() != "https://www.coles.com.au/api/products" ||
		start.GetSelectedRouteId() != "route-proxy" || start.GetSelectedPoolId() != "pool-proxy" ||
		start.GetDestinationPolicy().GetResolutionMode() != strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_UPSTREAM_PROXY_REMOTE {
		panic(fmt.Sprintf("old Go decoder lost known fields: %+v", start))
	}
	roundTrip, err := proto.MarshalOptions{Deterministic: true}.Marshal(&start)
	if err != nil || hex.EncodeToString(roundTrip) != os.Args[1] {
		panic("old Go decoder did not preserve unknown field 16")
	}
}
EOF

cat >"$tmp/go-new/go.mod" <<'EOF'
module straw-protocol-new-decoder

go 1.24.0

require (
  github.com/beremaran/straw-protos-go v0.4.0
  google.golang.org/protobuf v1.36.10
)
EOF
cat >"$tmp/go-new/main.go" <<'EOF'
package main

import (
	"encoding/hex"
	"fmt"
	"os"

	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
	"google.golang.org/protobuf/proto"
)

func main() {
	wire, err := hex.DecodeString(os.Args[1])
	if err != nil {
		panic(err)
	}
	var start strawpb.RequestStart
	if err := proto.Unmarshal(wire, &start); err != nil {
		panic(err)
	}
	if start.GetMethod() != "GET" || start.GetUrl() != "https://www.example.com/api/products" ||
		start.GetUpstreamProxy() != nil ||
		start.GetDestinationPolicy().GetResolutionMode() != strawpb.DestinationResolutionMode_DESTINATION_RESOLUTION_DIRECT_LOCAL {
		panic(fmt.Sprintf("new Go decoder rejected or changed old wire: %+v", start))
	}
}
EOF

(cd "$tmp/go-old" && GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}" go mod tidy >/dev/null && go run . "$proxy_hex")
(cd "$tmp/go-new" && GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}" go mod tidy >/dev/null && go run . "$direct_hex")

uv venv --quiet --python "${PYTHON_VERSION:-3.13}" "$tmp/python-old"
uv pip install --quiet --python "$tmp/python-old/bin/python" \
  'straw-protos @ git+https://github.com/beremaran/straw-protos-python.git@v0.3.0'
"$tmp/python-old/bin/python" - "$proxy_hex" <<'PY'
import sys

from straw_protos.straw.v1 import straw_pb2 as pb

wire = bytes.fromhex(sys.argv[1])
start = pb.RequestStart.FromString(wire)
assert start.method == "GET"
assert start.url == "https://www.coles.com.au/api/products"
assert start.selected_route_id == "route-proxy"
assert start.selected_pool_id == "pool-proxy"
assert start.destination_policy.resolution_mode == pb.DESTINATION_RESOLUTION_UPSTREAM_PROXY_REMOTE
assert start.SerializeToString(deterministic=True) == wire
PY

uv run --directory "$root" --frozen python - "$direct_hex" <<'PY'
import sys

from straw_protos.straw.v1 import straw_pb2 as pb

wire = bytes.fromhex(sys.argv[1])
start = pb.RequestStart.FromString(wire)
assert start.method == "GET"
assert start.url == "https://www.example.com/api/products"
assert not start.HasField("upstream_proxy")
assert start.destination_policy.resolution_mode == pb.DESTINATION_RESOLUTION_DIRECT_LOCAL
PY

echo "protocol compatibility: old decoders preserve field-16 wire; new decoders accept old wire"
