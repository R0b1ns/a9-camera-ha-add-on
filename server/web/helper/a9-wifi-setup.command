#!/bin/sh
cd "$(dirname "$0")"
if command -v python3 >/dev/null 2>&1; then
  python3 a9-wifi-setup.py
else
  python a9-wifi-setup.py
fi
