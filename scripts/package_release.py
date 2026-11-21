#!/usr/bin/env python3
"""Build an offline, local multi-platform release from a clean commit."""

import argparse
from collections.abc import Iterator, Mapping
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import zipfile
from urllib.parse import urljoin
from typing import NotRequired, TypedDict

TARGETS = (
    ("darwin", "arm64"),
    ("darwin", "amd64"),
    ("windows", "amd64"),
    ("linux", "amd64"),
    ("linux", "arm64"),
)
PAYLOAD = (
    "LICENSE",
    "ASSET_CREDITS.md",
    "docs/INSTALL.md",
    "internal/audio/assets/library.json",
    "internal/text/assets/ultima-v-warriors-of-destiny/license.txt",
    "internal/text/assets/ultima-v-warriors-of-destiny/readme.txt",
    "internal/ui/widgets/assets/ultima-v-warriors-of-destiny/license.txt",
    "internal/ui/widgets/assets/ultima-v-warriors-of-destiny/readme.txt",
)


class Dependency(TypedDict):
    """Production dependency fields from go list -m -json."""

    Path: str
    Version: str
    Dir: str
    Main: NotRequired[bool]
    Replace: NotRequired[dict[str, str]]


def run(command: list[str], cwd: Path, env: Mapping[str, str] | None = None) -> str:
    """Capture an external command's output while preserving failure details."""
    return subprocess.run(
        command, cwd=cwd, env=env, check=True, capture_output=True, text=True
    ).stdout.strip()


def source_identity(root: Path, revision: str, version: str) -> str:
    """Require a clean selected HEAD and a safe, nonconflicting version name."""
    if not re.fullmatch(
        r"v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?", version
    ):
        raise ValueError(
            "version must be vMAJOR.MINOR.PATCH with an optional prerelease suffix"
        )
    commit = run(
        ["git", "rev-parse", "--verify", "--end-of-options", revision + "^{commit}"],
        root,
    )
    if commit != run(["git", "rev-parse", "HEAD"], root):
        raise ValueError(
            "checkout the selected revision before packaging; it must equal HEAD"
        )
    if run(["git", "status", "--porcelain", "--untracked-files=all"], root):
        raise ValueError("release source must be clean, including untracked files")
    tags = run(["git", "tag", "--list", version], root)
    if (
        tags
        and run(["git", "rev-parse", f"refs/tags/{version}^{{commit}}"], root) != commit
    ):
        raise ValueError("existing version tag points to a different commit")
    return commit


def export_source(root: Path, commit: str, destination: Path) -> None:
    """Export only regular committed source entries into an isolated directory."""
    # Export only committed files; ignored local game data and settings never enter.
    with tempfile.TemporaryFile() as archive:
        subprocess.run(
            ["git", "archive", "--format=tar", commit],
            cwd=root,
            stdout=archive,
            check=True,
        )
        archive.seek(0)
        with tarfile.open(fileobj=archive) as contents:
            for member in contents:
                path = Path(member.name)
                if (
                    path.is_absolute()
                    or ".." in path.parts
                    or not (member.isfile() or member.isdir())
                ):
                    raise ValueError(f"unsupported source archive entry: {member.name}")
                if member.isdir():
                    (destination / path).mkdir(parents=True, exist_ok=True)
                else:
                    target = destination / path
                    target.parent.mkdir(parents=True, exist_ok=True)
                    src = contents.extractfile(member)
                    if src is None:
                        raise ValueError(f"source file unavailable: {member.name}")
                    with src, target.open("wb") as dst:
                        shutil.copyfileobj(src, dst)
                    target.chmod(member.mode & 0o777)


def build_environment(source: Path) -> tuple[dict[str, str], str]:
    """Construct the offline pinned toolchain environment for all targets."""
    match = re.search(
        r"^toolchain (go\S+)$",
        (source / "go.mod").read_text(encoding="utf-8"),
        re.MULTILINE,
    )
    if not match:
        raise ValueError("go.mod must select an explicit toolchain")
    env = os.environ.copy()
    # Do not inherit local build tags, workspaces, architecture tuning or overlays.
    for key in ("GOOS", "GOARCH", "GOAMD64", "GOARM64", "GOEXPERIMENT"):
        env.pop(key, None)
    env.update(
        GOENV="off",
        GOWORK="off",
        GOFLAGS="",
        GOTOOLCHAIN=match[1],
        GOPROXY="off",
        GOSUMDB="off",
        CGO_ENABLED="0",
        GOMAXPROCS="2",
        GOMEMLIMIT="256MiB",
        GOGC="50",
    )
    toolchain = run(["go", "version"], source, env)
    if toolchain.split()[2] != match[1]:
        raise ValueError(f"expected {match[1]}, found {toolchain}")
    return env, toolchain


def json_stream(text: str) -> Iterator[Dependency]:
    """Iterate the adjacent module objects emitted by go list."""
    decoder = json.JSONDecoder()
    while text.strip():
        value, end = decoder.raw_decode(text.lstrip())
        yield value
        text = text.lstrip()[end:]


def copy_notices(source: Path, destination: Path, env: Mapping[str, str]) -> None:
    """Collect notices for the union of actual production imports on all targets."""
    # Discovery and mandatory-license verification form one fail-closed operation.
    # pylint: disable=too-many-locals
    for relative in PAYLOAD:
        target = destination / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source / relative, target)
    # The module graph also contains unused/test-only modules. Collect notices for
    # the union of production imports across all targets, not that whole graph.
    used = set()
    for system, architecture in TARGETS:
        target_env = dict(env, GOOS=system, GOARCH=architecture)
        used.update(
            run(
                [
                    "go",
                    "list",
                    "-mod=readonly",
                    "-deps",
                    "-f",
                    "{{if .Module}}{{.Module.Path}}{{end}}",
                    "./cmd/ultimav",
                ],
                source,
                target_env,
            ).splitlines()
        )
    modules = list(json_stream(run(["go", "list", "-m", "-json", "all"], source, env)))
    locations = [("go", Path(run(["go", "env", "GOROOT"], source, env)))]
    for module in modules:
        if module.get("Main"):
            continue
        if module["Path"] not in used:
            continue
        if module.get("Replace"):
            raise ValueError("release packaging does not support replaced dependencies")
        if not module.get("Dir"):
            raise ValueError(
                f"dependency not provisioned: {module['Path']}; run go mod download"
            )
        locations.append(
            (module["Path"] + "@" + module["Version"], Path(module["Dir"]))
        )
    for name, directory in locations:
        notices = [
            p
            for p in directory.iterdir()
            if p.is_file()
            and re.match(
                r"^(LICENSE|LICENCE|COPYING|NOTICE|AUTHORS|PATENTS)(?:[._-].*)?$",
                p.name,
                re.I,
            )
        ]
        # Homebrew may move the Go license from libexec to the formula prefix.
        if (
            name == "go"
            and directory.name == "libexec"
            and not any(p.name == "LICENSE" for p in notices)
        ):
            license_path = directory.parent / "LICENSE"
            if license_path.is_file():
                notices.append(license_path)
        if not any(
            re.match(r"^(LICENSE|LICENCE|COPYING)(?:[._-].*)?$", p.name, re.I)
            for p in notices
        ):
            raise ValueError(
                f"no dependency license found for {name}; review its terms"
            )
        for notice in notices:
            target = destination / "notices" / name / notice.name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(notice, target)


def package_tree(tree: Path, archive: Path, windows: bool) -> None:
    """Write a platform archive containing the exact assembled payload."""
    files = sorted(p for p in tree.rglob("*") if p.is_file())
    if windows:
        with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED) as output:
            for path in files:
                output.write(path, str(path.relative_to(tree.parent)))
    else:
        with tarfile.open(archive, "w:gz") as output:
            for path in files:
                output.add(
                    path, arcname=str(path.relative_to(tree.parent)), recursive=False
                )


def digest(path: Path) -> str:
    """Stream the artifact SHA-256 without loading the binary into memory."""
    checksum = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            checksum.update(block)
    return checksum.hexdigest()


def build_matrix(
    source: Path,
    output: Path,
    version: str,
    commit: str,
    env: Mapping[str, str],
    toolchain: str,
) -> None:
    """Build all target archives with revision-bound notices and checksums."""
    # These inputs identify the single atomic release; each target shares that identity.
    # pylint: disable=too-many-arguments,too-many-positional-arguments,too-many-locals
    with tempfile.TemporaryDirectory(prefix="u5r-payload-") as temporary:
        work = Path(temporary)
        common = work / "common"
        copy_notices(source, common, env)
        install = common / "docs/INSTALL.md"
        # The compact payload omits developer docs; link to the exact source revision.
        base = f"https://github.com/bradhannah/UltimaVRedux/blob/{commit}/docs/"
        install.write_text(
            re.sub(
                r"\]\(([^)#][^)]*)\)",
                lambda match: "](" + urljoin(base, match[1]) + ")",
                install.read_text(encoding="utf-8"),
            ),
            encoding="utf-8",
        )
        for system, architecture in TARGETS:
            name = f"ultimav_{version}_{system}_{architecture}"
            tree = work / name
            shutil.copytree(common, tree)
            windows = system == "windows"
            binary = tree / ("ultimav.exe" if windows else "ultimav")
            print(f"Building {system}/{architecture} at {commit}", flush=True)
            target_env = dict(env, GOOS=system, GOARCH=architecture)
            run(
                [
                    "go",
                    "build",
                    "-p=1",
                    "-mod=readonly",
                    "-trimpath",
                    "-buildvcs=false",
                    "-ldflags=-s -w -X main.version=" + version,
                    "-o",
                    str(binary),
                    "./cmd/ultimav",
                ],
                source,
                target_env,
            )
            binary.chmod(0o755)
            metadata = {
                "version": version,
                "commit": commit,
                "toolchain": toolchain,
                "goos": system,
                "goarch": architecture,
                "cgo_enabled": False,
                "native_runtime_acceptance": "not established by packaging",
                "binary_sha256": digest(binary),
            }
            (tree / "release.json").write_text(
                json.dumps(metadata, indent=2) + "\n", encoding="utf-8"
            )
            launch = r".\ultimav.exe" if windows else "./ultimav"
            (tree / "START-HERE.txt").write_text(
                f"Ultima V Redux {version}\nSource commit: {commit}\n\n"
                "Extract the entire archive. Open a terminal in this folder "
                f"and run {launch}\n"
                "Use this explicit path, not an older ultimav command on PATH.\n"
                "Go is not required. Supply your own complete original "
                "Ultima V installation.\n"
                "See docs/INSTALL.md for setup, platform limits, "
                "and unsigned-download guidance.\n"
                "These are unsigned development-preview binaries, "
                "not native acceptance proof.\n"
                "LICENSE, ASSET_CREDITS.md, and notices/ retain applicable terms.\n",
                encoding="utf-8",
            )
            archive = output / (name + (".zip" if windows else ".tar.gz"))
            package_tree(tree, archive, windows)
    archives = sorted(output.iterdir())
    (output / "SHA256SUMS").write_text(
        "".join(f"{digest(p)}  {p.name}\n" for p in archives), encoding="utf-8"
    )


def package(root: Path, revision: str, version: str, output: Path) -> None:
    """Publish a complete local directory only after every archive succeeds."""
    root = root.resolve()
    output = output.absolute()
    if output.exists() or output.is_symlink():
        raise ValueError(
            "output must not already exist; choose a new release directory"
        )
    if not output.parent.is_dir():
        raise ValueError("output parent must already exist")
    output = output.resolve()
    if output == root or root in output.parents:
        raise ValueError("release output must be outside the source checkout")
    commit = source_identity(root, revision, version)
    lock = output.with_name(output.name + ".lock")
    # Exclusive reservation; concurrent attempts must not overwrite output.
    lock.mkdir()
    try:
        with tempfile.TemporaryDirectory(prefix="u5r-source-") as temporary:
            source = Path(temporary)
            export_source(root, commit, source)
            env, toolchain = build_environment(source)
            with tempfile.TemporaryDirectory(
                prefix=".u5r-release-", dir=output.parent
            ) as staged:
                build_matrix(source, Path(staged), version, commit, env, toolchain)
                if source_identity(root, revision, version) != commit:
                    raise ValueError("source changed during packaging")
                if output.exists():
                    raise ValueError("output appeared during packaging")
                Path(staged).rename(output)
    finally:
        lock.rmdir()
    print(f"Complete local release: {output}")


def main() -> int:
    """Parse release options and report recoverable packaging failures."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--revision", required=True, help="commit or ref, must equal clean HEAD"
    )
    parser.add_argument(
        "--version", required=True, help="e.g. v0.1.0-preview.1; no tag is created"
    )
    parser.add_argument(
        "--output", type=Path, required=True, help="new directory outside checkout"
    )
    args = parser.parse_args()
    try:
        package(
            Path(__file__).resolve().parents[1],
            args.revision,
            args.version,
            args.output,
        )
    except (
        OSError,
        ValueError,
        subprocess.CalledProcessError,
        tarfile.TarError,
    ) as error:
        print(f"Release packaging failed: {error}", file=sys.stderr)
        if isinstance(error, subprocess.CalledProcessError) and error.stderr:
            print(error.stderr, file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
