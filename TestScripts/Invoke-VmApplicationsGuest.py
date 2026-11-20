# Copyright (c) Microsoft Corporation.
# Licensed under the MIT license.

"""CustomScript helper; executes as root on the disposable Linux VM."""

import base64
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import sys
import tarfile
import urllib.request


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def handler_directory() -> Path:
    candidates = [
        path
        for path in Path("/var/lib/waagent").iterdir()
        if re.fullmatch(
            r"Microsoft\.CPlat\.Core(?:\.EDP)?\.VMApplicationManagerLinux-[\d.]+",
            path.name,
            re.IGNORECASE,
        )
        and (path / "config/HandlerState").is_file()
        and (path / "config/HandlerState").read_text().strip() == "Enabled"
    ]
    if len(candidates) != 1:
        raise RuntimeError(f"Expected one enabled VM Applications handler: {candidates}")
    return candidates[0]


def main() -> None:
    config = json.loads(base64.b64decode(sys.argv[1]))
    directory = handler_directory()
    action = config["action"]
    if action != "collect":
        anchor = Path("/var/lib/vmapplications-local-validation-anchor")
        if not anchor.is_file() or anchor.read_text() != "local-validation-anchor":
            raise RuntimeError("The bootstrap anchor application is not installed")
    if action == "collect":
        archive = Path("/tmp/vmapplications-validation.tar.gz")
        try:
            with tarfile.open(archive, "w:gz") as bundle:
                for path in (
                    directory / "status",
                    directory / "config",
                    Path("/var/lib/lisa-vmapplications"),
                    Path("/var/log/azure") / directory.name.rsplit("-", 1)[0],
                    Path("/var/log/waagent.log"),
                ):
                    if path.exists():
                        bundle.add(path, arcname=str(path).lstrip("/"))
            request = urllib.request.Request(
                config["upload_uri"],
                data=archive.read_bytes(),
                headers={"x-ms-blob-type": "BlockBlob"},
                method="PUT",
            )
            with urllib.request.urlopen(request, timeout=120) as response:
                if response.status != 201:
                    raise RuntimeError(f"Log upload failed: {response.status}")
        finally:
            archive.unlink(missing_ok=True)
    elif action in ("install", "verify"):
        if str(directory) != config["directory"]:
            raise RuntimeError("The VM agent changed the active handler directory")
        for name, expected in config["hashes"].items():
            destination = directory / "bin" / name
            if action == "install":
                source = Path(config["sources"][name])
                if digest(source) != expected:
                    raise RuntimeError(f"Downloaded binary hash mismatch: {name}")
                staged = destination.with_suffix(".validation-new")
                shutil.copyfile(source, staged)
                os.chown(staged, 0, 0)
                staged.chmod(0o755)
                os.replace(staged, destination)
            if digest(destination) != expected:
                raise RuntimeError(f"Installed binary hash mismatch: {destination}")
    elif action != "discover":
        raise ValueError(f"Unknown action: {action}")
    print(
        "VMAPPS-VALIDATION:"
        + json.dumps(
            {
                "directory": str(directory),
                "version": directory.name.rsplit("-", 1)[1],
                "extension_name": directory.name.rsplit("-", 1)[0],
            }
        ),
        flush=True,
    )


if __name__ == "__main__":
    main()
