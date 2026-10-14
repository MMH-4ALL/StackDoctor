#!/usr/bin/env python3
"""Build four dependency-free binary archives and SHA-256 checksums.

Maintainer tooling only: Python is not needed to run StackDoctor.
Run from a checkout: python3 scripts/package.py 0.1.0
"""
import hashlib
import io
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tarfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]
TARGETS = [("linux", "amd64"), ("linux", "arm64"),
           ("windows", "amd64"), ("windows", "arm64")]


def package(version: str) -> None:
    if not re.fullmatch(r"\d{1,4}\.\d{1,4}\.\d{1,4}", version):
        raise SystemExit("Expected a release version such as 0.1.0")
    if not shutil.which("go"):
        raise SystemExit("Go 1.24 or later is required for packaging")
    dist = ROOT / "dist"
    dist.mkdir(exist_ok=True)
    common = {
        "LICENSE": (ROOT / "LICENSE").read_bytes(),
        "THIRD_PARTY_NOTICES.md": (ROOT / "THIRD_PARTY_NOTICES.md").read_bytes(),
        "README.md": (ROOT / "README.md").read_bytes(),
        "RELEASE_NOTES.md": (ROOT / "docs" / f"RELEASE_NOTES_v{version}.md").read_bytes(),
    }
    checksums = []
    for target_os, arch in TARGETS:
        binary_name = "stackdoctor.exe" if target_os == "windows" else "stackdoctor"
        binary = dist / f"stackdoctor-{target_os}-{arch}{'.exe' if target_os == 'windows' else ''}"
        env = os.environ.copy()
        env.update(GOOS=target_os, GOARCH=arch, CGO_ENABLED="0")
        subprocess.run([
            "go", "build", "-mod=readonly", "-trimpath",
            "-ldflags", f"-s -w -X main.version={version}",
            "-o", str(binary), "./cmd/stackdoctor",
        ], cwd=ROOT, env=env, check=True)
        files = {binary_name: binary.read_bytes(), **common}
        basename = f"stackdoctor_{version}_{target_os}_{arch}"
        if target_os == "windows":
            archive = dist / (basename + ".zip")
            with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED) as z:
                for name, data in files.items():
                    info = zipfile.ZipInfo(name, date_time=(1980, 1, 1, 0, 0, 0))
                    info.compress_type = zipfile.ZIP_DEFLATED
                    z.writestr(info, data)
        else:
            archive = dist / (basename + ".tar.gz")
            with tarfile.open(archive, "w:gz") as tar:
                for name, data in files.items():
                    info = tarfile.TarInfo(name)
                    info.size = len(data)
                    info.mode = 0o755 if name == binary_name else 0o644
                    info.mtime = 0
                    tar.addfile(info, io.BytesIO(data))
        digest = hashlib.sha256(archive.read_bytes()).hexdigest()
        checksums.append(f"{digest}  {archive.name}")
        print(archive.name)
    (dist / "checksums.txt").write_text("\n".join(checksums) + "\n", encoding="utf-8")


if __name__ == "__main__":
    if len(sys.argv) != 2:
        raise SystemExit("Usage: python3 scripts/package.py 0.1.0")
    package(sys.argv[1])
