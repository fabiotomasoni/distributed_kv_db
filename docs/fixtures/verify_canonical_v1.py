"""Check static documentation bytes, not a WAL codec or recovery implementation.

Read-only, standard-library-only, bounded to the supplied small fixture file.
Run: python3 docs/fixtures/verify_canonical_v1.py
Recovery expectations are specifications for future tests, never executed here.
"""
from pathlib import Path
import base64
import hashlib
import json
import struct


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True).encode("ascii")


def crc32c(data):
    crc = 0xFFFFFFFF
    for byte in data:
        crc ^= byte
        for _ in range(8):
            crc = (crc >> 1) ^ (0x82F63B78 if crc & 1 else 0)
    return crc ^ 0xFFFFFFFF


def digest_input(entry):
    opcode = {"noop": 0, "put": 1, "delete": 2, "cas": 3, "read": 4}[entry["kind"]]
    output = b"QCMD\x01" + bytes([opcode])
    if opcode == 0:
        assert "command" not in entry
        return output
    command = entry["command"]

    def prefixed(raw):
        return struct.pack(">I", len(raw)) + raw

    output += prefixed(command["key"].encode("ascii"))
    if opcode == 1:
        output += prefixed(base64.b64decode(command["value_b64"], validate=True))
    if opcode == 3:
        expected = command["expected"]
        output += bytes([int(expected["exists"])])
        if expected["exists"]:
            output += prefixed(base64.b64decode(expected["value_b64"], validate=True))
        output += prefixed(base64.b64decode(command["new_value_b64"], validate=True))
    return output


def main():
    path = Path(__file__).with_name("canonical-v1.json")
    assert path.stat().st_size < 128 * 1024
    fixture = json.loads(path.read_text())
    assert fixture["fixture_version"] == 1
    assert crc32c(b"123456789") == 0xE3069283
    for vector in fixture["commands"]:
        entry = json.loads(vector["entry_json"])
        assert canonical(entry).decode() == vector["entry_json"]
        raw = digest_input(entry)
        assert raw.hex() == vector["digest_input_hex"]
        assert hashlib.sha256(raw).hexdigest() == vector["command_sha256"]
    commands = {v["id"]: v for v in fixture["commands"]}
    assert commands["cas-absent"]["command_sha256"] != commands["cas-present-empty"]["command_sha256"]
    for vector in fixture["wal"]:
        raw = bytes.fromhex(vector["frame_hex"])
        magic, version, kind, length, sequence, checksum = struct.unpack(">4sHHIQI", raw[:24])
        assert (magic, version, kind) == (b"QWAL", 1, 1)
        assert sequence == int(vector["sequence"])
        assert raw[24:] == vector["payload_json"].encode("ascii")
        assert len(raw) == length + 24 and length <= 393216
        assert canonical(json.loads(vector["payload_json"])) == raw[24:]
        assert crc32c(raw[:20] + raw[24:]) == checksum
    for name in ["snapshot", "stateful_snapshot"]:
        vector = fixture[name]
        image = bytes.fromhex(vector["image_hex"])
        assert image[:4] == b"QSN1" and len(image) <= 8388608
        assert struct.unpack(">Q", image[4:12])[0] == len(image) - 44
        assert image[12:-32] == vector["payload_json"].encode("ascii")
        assert hashlib.sha256(image[:-32]).digest() == image[-32:]
        assert hashlib.sha256(image).hexdigest() == vector["file_sha256"]
        state = json.loads(vector["payload_json"])
        assert canonical(state) == image[12:-32]
        assert int(state["key_count"]) == len(state["kv"])
        assert int(state["session_count"]) == len(state["sessions"])
        assert int(state["logical_bytes"]) == sum(len(k["key"]) + len(base64.b64decode(k["value_b64"])) for k in state["kv"])
        assert [k["key"] for k in state["kv"]] == sorted(k["key"] for k in state["kv"])
    for name in ["identity_json", "current_json"]:
        text = fixture[name]
        record = json.loads(text)
        assert canonical(record).decode() == text and len(text) <= 4096
        checksum = record.pop("checksum")
        assert hashlib.sha256(canonical(record)).hexdigest() == checksum
    current = json.loads(fixture["current_json"])
    assert current["snapshot_sha256"] == fixture["snapshot"]["file_sha256"]
    assert current["base_index"] == current["base_term"] == "0"
    for text in fixture["result_json"]:
        assert canonical(json.loads(text)).decode() == text
    cas = json.loads(commands["cas-present-empty"]["entry_json"])
    cas.update(index=str(2**64-1), term=str(2**64-1))
    cas["command"].update(key="k"*256, sequence=2**53-1, new_value_b64=base64.b64encode(bytes(16384)).decode())
    cas["command"]["expected"]["value_b64"] = cas["command"]["new_value_b64"]
    assert len(canonical(cas)) == fixture["bounds"]["maximum_cas_entry_bytes"] == 44184
    assert len(canonical(cas)) < fixture["bounds"]["entry_limit_bytes"]
    cases = {case["id"]: case for case in fixture["recovery_cases"]}
    valid = bytes.fromhex(cases["valid-committed"]["wal_hex"])
    assert cases["complete-unsynced"]["wal_hex"] == valid.hex()
    assert bytes.fromhex(cases["partial-final-header"]["wal_hex"]) == valid + b"QWA"
    assert bytes.fromhex(cases["partial-final-payload"]["wal_hex"]) == valid[:-1]
    corrupt = bytes.fromhex(cases["complete-bad-crc"]["wal_hex"])
    assert corrupt[:-1] == valid[:-1] and corrupt[-1] == (valid[-1] ^ 1)
    assert len(cases) == 18
    print("PASS: 6 command/digests, 6 results, 2 WAL frames, 2 snapshots, 2 manifests, max-CAS length and selected fixture mutations.")
    print("18 recovery expectations and 6 invalid-JSON expectations are specifications; no recovery/parser implementation was tested.")


if __name__ == "__main__":
    main()
