#!/bin/sh
# Collects into third_party_licenses/<os>/ the license files of Go and of every module linked into
# lazyfwd for that OS. Each release archive ships its OS's directory.
set -eu
rm -rf third_party_licenses
for os in linux darwin windows; do
  out=third_party_licenses/$os
  mkdir -p "$out/go"
  cp "$(go env GOROOT)/LICENSE" "$out/go/"
  GOOS=$os go list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}} {{.Dir}}{{end}}{{end}}' . |
    sort -u | while read -r path dir; do
      mkdir -p "$out/$path"
      found=$(find "$dir" -maxdepth 1 -type f \( -iname 'LICENSE*' -o -iname 'LICENCE*' -o -iname 'COPYING*' -o -iname 'NOTICE*' \))
      if [ -n "$found" ]; then
        echo "$found" | while read -r f; do cp "$f" "$out/$path/"; done
      elif [ "$path" = github.com/mattn/go-localereader ]; then
        # Its README declares MIT, but the repository has no license file.
        echo "MIT, as declared in https://github.com/mattn/go-localereader#license" > "$out/$path/LICENSE"
      else
        echo "no license file in $path ($dir)" >&2
        exit 1
      fi
    done
done
