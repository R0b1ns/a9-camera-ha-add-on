#!/usr/bin/env python3
"""A9 / V720 camera Wi-Fi setup for a computer joined to the camera AP.

No parameters, no extra packages. Standalone version of the add-on's AP client.

Usage:
  1. Join the camera's own Wi-Fi (SSID starts with NAXCLOW / Nax_ / 0800c0).
     Password is usually 34974033A (see the camera label / manual).
  2. Run this script (double-click a9-wifi-setup.command / .bat or:
     python3 a9-wifi-setup.py).
  3. It waits for the camera at 192.168.169.1:6123, asks for the new
     network and configures the camera. Nothing is logged or stored.
"""

import getpass
import json
import socket
import struct
import sys
import time
from datetime import datetime

HOST = "192.168.169.1"
PORT = 6123
DEV_TARGET = "deadbeef"
TOKEN = "This is TEST token"
CMD_JSON = 0
CMD_LIVE_MOTION = 115
CODE_AP_CONNECT = 501
CODE_AP_COMMAND = 502
CODE_SET_WIFI = 204
HEADER = struct.Struct("<LHBB8sL")
FORWARD_ID = b"00000000"


def packet(payload: bytes, cmd: int = CMD_JSON) -> bytes:
    return HEADER.pack(len(payload), cmd, 0, 0, FORWARD_ID, 0) + payload


class Stream:
    def __init__(self, sock):
        self.sock = sock
        self.buf = b""

    def read_packet(self):
        deadline = time.monotonic() + 4
        while time.monotonic() < deadline:
            if len(self.buf) >= HEADER.size:
                length = HEADER.unpack_from(self.buf)[0]
                total = HEADER.size + length
                if total > 1 << 20:
                    raise ValueError("invalid packet size")
                if len(self.buf) >= total:
                    raw, self.buf = self.buf[:total], self.buf[total:]
                    cmd = HEADER.unpack_from(raw)[1]
                    payload = raw[HEADER.size:]
                    if cmd == 100:  # heartbeat
                        continue
                    return json.loads(payload) if payload else {}
            self.buf += self.sock.recv(4096)
        raise TimeoutError("camera did not answer")


def main() -> int:
    print("A9 camera Wi-Fi setup")
    print("=" * 40)
    print("Make sure this computer is connected to the camera's own Wi-Fi")
    print("(SSID starts with NAXCLOW / Nax_ / 0800c0).")
    print("Waiting for the camera at %s:%d ..." % (HOST, PORT))
    while True:
        try:
            sock = socket.create_connection((HOST, PORT), timeout=4)
            break
        except OSError:
            try:
                answer = input("Not found. Are you connected to the camera AP? (y/n/q) ").strip().lower()
            except EOFError:
                return 1
            if answer == "q":
                return 1
            time.sleep(1)
    sock.settimeout(4)
    stream = Stream(sock)
    try:
        sock.sendall(packet(b"", CMD_LIVE_MOTION))
        stream.read_packet()
        connect = {
            "code": CODE_AP_CONNECT,
            "target": DEV_TARGET,
            "token": TOKEN,
            "unixTimer": int(datetime.now().timestamp()),
        }
        sock.sendall(packet(json.dumps(connect, separators=(",", ":")).encode()))
        stream.read_packet()
        ssid = input("\nNew Wi-Fi name (SSID): ").strip()
        while not ssid:
            ssid = input("SSID must not be empty: ").strip()
        password = getpass.getpass("New Wi-Fi password (8-63 chars): ")
        while not 8 <= len(password) <= 63:
            password = getpass.getpass("Password must be 8-63 characters: ")
        command = {
            "code": CODE_AP_COMMAND,
            "content": {
                "code": CODE_SET_WIFI,
                "devTarget": DEV_TARGET,
                "s": ssid,
                "p": password,
            },
        }
        sock.sendall(packet(json.dumps(command, separators=(",", ":")).encode()))
        stream.read_packet()
        print("\nDone. The camera now joins %s and reboots." % ssid)
        print("Keep the DNS rewrites *.naxclow.com and *.naxclow.com.cn")
        print("pointing at your Home Assistant host in the new network.")
        return 0
    except (OSError, ValueError, TimeoutError) as exc:
        print("Camera did not answer: %s" % exc, file=sys.stderr)
        print("Stay connected to the camera AP and try again.", file=sys.stderr)
        return 1
    finally:
        sock.close()


if __name__ == "__main__":
    raise SystemExit(main())
