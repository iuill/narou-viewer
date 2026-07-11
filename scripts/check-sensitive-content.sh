#!/usr/bin/env bash
set -euo pipefail

awk '
  {
    rest = $0
    while (match(rest, /(^|[^0-9])[0-9][0-9]?[0-9]?\.[0-9][0-9]?[0-9]?\.[0-9][0-9]?[0-9]?\.[0-9][0-9]?[0-9]?([^0-9]|$)/)) {
      candidate = substr(rest, RSTART, RLENGTH)
      gsub(/^[^0-9]|[^0-9]$/, "", candidate)
      split(candidate, octet, ".")
      valid = 1
      for (part = 1; part <= 4; part++) {
        if (octet[part] < 0 || octet[part] > 255) valid = 0
      }
      first = octet[1] + 0
      second = octet[2] + 0
      reserved = first == 0 || first == 10 || first == 127 || first >= 224
      if (first == 100 && second >= 64 && second <= 127) reserved = 1
      if (first == 169 && second == 254) reserved = 1
      if (first == 172 && second >= 16 && second <= 31) reserved = 1
      if (first == 192 && second == 168) reserved = 1
      if (first == 192 && second == 0 && octet[3] == 2) reserved = 1
      if (first == 198 && (second == 18 || second == 19)) reserved = 1
      if (first == 198 && second == 51 && octet[3] == 100) reserved = 1
      if (first == 203 && second == 0 && octet[3] == 113) reserved = 1
      if (valid && !reserved) {
        print "公開IPv4 address候補を検出しました（値はredacted）。synthetic addressへ置換してください。" > "/dev/stderr"
        failed = 1
      }
      rest = substr(rest, RSTART + RLENGTH)
    }
  }
  END { exit failed }
'
