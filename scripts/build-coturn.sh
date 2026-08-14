#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC="${ROOT}/third_party/coturn"
BUILD="${ROOT}/build-coturn"

if [[ ! -f "${SRC}/CMakeLists.txt" ]]; then
  echo "coturn submodule missing. Run: git submodule update --init third_party/coturn" >&2
  exit 1
fi

cmake -S "${SRC}" -B "${BUILD}" \
  -DCMAKE_BUILD_TYPE=Release \
  -DBUILD_TESTING=OFF \
  -DFUZZER=OFF \
  -DWITH_MYSQL=OFF
cmake --build "${BUILD}" --target turnserver --parallel

BIN="${BUILD}/bin/turnserver"
if [[ ! -x "${BIN}" ]]; then
  echo "turnserver binary missing: ${BIN}" >&2
  exit 1
fi

echo "${BIN}"
