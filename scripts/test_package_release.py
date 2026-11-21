"""Exercise artifact contents and failed-release isolation without game builds."""

# Test names explain assertions; callbacks deliberately match the production run API.
# pylint: disable=missing-function-docstring

from collections.abc import Mapping
import hashlib
import json
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest
from unittest.mock import patch
import zipfile
from typing import Protocol

import package_release as release


class CommandRunner(Protocol):
    """Captured release command callback, optionally supplied by a failure fixture."""

    # This callable protocol models a single subprocess operation.
    # pylint: disable=too-few-public-methods

    def __call__(
        self, command: list[str], cwd: Path, env: Mapping[str, str] | None = None
    ) -> str: ...


class PackagingTests(unittest.TestCase):
    """Exercise atomic release publication with deterministic fake build output."""

    def setUp(self) -> None:
        # unittest keeps this context alive through assertions and cleans up on failure.
        # pylint: disable=consider-using-with
        self.base = Path(self.enterContext(tempfile.TemporaryDirectory()))
        self.root = self.base / "checkout"
        self.root.mkdir()
        self.output = self.base / "release"
        self.commit = "a" * 40

    def seed_source(self, _root: Path, _commit: str, destination: Path) -> None:
        for name in release.PAYLOAD:
            path = destination / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("notice " + name, encoding="utf-8")
        (destination / "go.mod").write_text(
            "module example.com/game\n\ntoolchain go1.27.1\n", encoding="utf-8"
        )
        (destination / "docs/INSTALL.md").write_text(
            "[developer](DEVELOPMENT_GUIDE.md) [setup](#setup) [Go](https://go.dev/)\n",
            encoding="utf-8",
        )
        # These must never appear in the distributable payload.
        (destination / "SAVED.GAM").write_text("private save", encoding="utf-8")
        (destination / "settings.yaml").write_text("private settings", encoding="utf-8")

    def fake_run(
        self, command: list[str], cwd: Path, env: Mapping[str, str] | None = None
    ) -> str:
        self.assertTrue(cwd.is_dir())
        if command == ["go", "version"]:
            return "go version go1.27.1 darwin/arm64"
        if command[:3] == ["go", "list", "-m"]:
            return (
                json.dumps({"Main": True})
                + "\n"
                + json.dumps({"Path": "unused.example/test-only"})
            )
        if command[:3] == ["go", "list", "-mod=readonly"]:
            return "example.com/game"
        if command == ["go", "env", "GOROOT"]:
            goroot = self.base / "go"
            goroot.mkdir(exist_ok=True)
            (goroot / "LICENSE").write_text("Go license", encoding="utf-8")
            return str(goroot)
        if command[:2] == ["go", "build"]:
            assert env is not None
            self.assertEqual(env["CGO_ENABLED"], "0")
            self.assertEqual(env["GOPROXY"], "off")
            self.assertIn("-trimpath", command)
            self.assertIn("-ldflags=-s -w -X main.version=v0.1.0-preview.1", command)
            binary = Path(command[command.index("-o") + 1])
            binary.write_bytes(f"binary {env['GOOS']}/{env['GOARCH']}".encode())
            return ""
        raise AssertionError(f"unexpected command {command}")

    def execute(self, runner: CommandRunner | None = None) -> None:
        with (
            patch.object(release, "source_identity", return_value=self.commit),
            patch.object(release, "export_source", side_effect=self.seed_source),
            patch.object(release, "run", side_effect=runner or self.fake_run),
        ):
            release.package(self.root, self.commit, "v0.1.0-preview.1", self.output)

    def test_complete_matrix_membership_metadata_permissions_and_checksums(
        self,
    ) -> None:
        self.execute()
        self.assertEqual(len(list(self.output.iterdir())), len(release.TARGETS) + 1)
        for line in (
            (self.output / "SHA256SUMS").read_text(encoding="utf-8").splitlines()
        ):
            checksum, name = line.split("  ")
            archive = self.output / name
            self.assertEqual(checksum, release.digest(archive))
            if name.endswith(".zip"):
                with zipfile.ZipFile(archive) as contents:
                    files = {
                        n.split("/", 1)[1]: contents.read(n)
                        for n in contents.namelist()
                    }
                binary = "ultimav.exe"
            else:
                with tarfile.open(archive) as contents:
                    files = {}
                    for member in contents:
                        stream = contents.extractfile(member)
                        assert stream is not None
                        with stream:
                            files[member.name.split("/", 1)[1]] = stream.read()
                    binary_member = next(
                        m for m in contents.getmembers() if m.name.endswith("/ultimav")
                    )
                    self.assertEqual(binary_member.mode & 0o777, 0o755)
                binary = "ultimav"
            expected = set(release.PAYLOAD) | {
                binary,
                "release.json",
                "START-HERE.txt",
                "notices/go/LICENSE",
            }
            self.assertEqual(set(files), expected)
            metadata = json.loads(files["release.json"])
            self.assertEqual(metadata["commit"], self.commit)
            self.assertEqual(metadata["version"], "v0.1.0-preview.1")
            self.assertIn((metadata["goos"], metadata["goarch"]), release.TARGETS)
            self.assertIn(metadata["goos"] + "_" + metadata["goarch"], name)
            self.assertEqual(
                metadata["binary_sha256"], hashlib.sha256(files[binary]).hexdigest()
            )
            instructions = files["docs/INSTALL.md"].decode()
            self.assertIn(
                f"/blob/{self.commit}/docs/DEVELOPMENT_GUIDE.md", instructions
            )
            self.assertIn("[setup](#setup)", instructions)
            self.assertIn("[Go](https://go.dev/)", instructions)
        self.assertFalse(self.output.with_name("release.lock").exists())

    def test_mid_matrix_failure_never_exposes_partial_release(self) -> None:
        def fail_windows(
            command: list[str], cwd: Path, env: Mapping[str, str] | None = None
        ) -> str:
            if command[:2] == ["go", "build"] and env and env["GOOS"] == "windows":
                raise subprocess.CalledProcessError(
                    1, command, stderr="compiler failed"
                )
            return self.fake_run(command, cwd, env)

        with self.assertRaises(subprocess.CalledProcessError):
            self.execute(fail_windows)
        self.assertFalse(self.output.exists())
        self.assertEqual(
            sorted(p.name for p in self.base.iterdir()), ["checkout", "go"]
        )

    def test_existing_output_is_preserved(self) -> None:
        self.output.mkdir()
        marker = self.output / "keep"
        marker.write_text("previous artifacts", encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "already exist"):
            self.execute()
        self.assertEqual(marker.read_text(encoding="utf-8"), "previous artifacts")

    def test_output_inside_checkout_is_rejected(self) -> None:
        self.output = self.root / "release"
        with self.assertRaisesRegex(ValueError, "outside"):
            self.execute()

    def test_concurrent_reservation_is_preserved(self) -> None:
        lock = self.output.with_name("release.lock")
        lock.mkdir()
        with self.assertRaises(FileExistsError):
            self.execute()
        self.assertTrue(lock.is_dir())

    def test_dirty_wrong_revision_and_conflicting_tag_are_rejected(self) -> None:
        cases = [
            ([self.commit, self.commit, "?? secret.txt"], "clean"),
            ([self.commit, "b" * 40], "HEAD"),
            ([self.commit, self.commit, "", "v0.1.0", "b" * 40], "tag"),
        ]
        for responses, message in cases:
            with (
                self.subTest(message=message),
                patch.object(release, "run", side_effect=responses),
            ):
                with self.assertRaisesRegex(ValueError, message):
                    release.source_identity(self.root, "HEAD", "v0.1.0")

    def test_invalid_version_cannot_become_a_path_or_flag(self) -> None:
        for version in ("../oops", "--help", "main", "v1.0"):
            with self.subTest(version=version), self.assertRaises(ValueError):
                release.source_identity(self.root, "HEAD", version)

    def test_missing_notice_fails_closed(self) -> None:
        source = self.base / "source"
        source.mkdir()
        self.seed_source(self.root, self.commit, source)
        (source / "LICENSE").unlink()
        with self.assertRaises(FileNotFoundError):
            release.copy_notices(source, self.base / "payload", {})

    def test_platform_specific_dependency_notices_are_included(self) -> None:
        dependency = self.base / "windows-dependency"
        dependency.mkdir()
        (dependency / "LICENSE.txt").write_text(
            "Windows dependency license", encoding="utf-8"
        )

        def with_dependency(
            command: list[str], cwd: Path, env: Mapping[str, str] | None = None
        ) -> str:
            if (
                command[:3] == ["go", "list", "-mod=readonly"]
                and env
                and env["GOOS"] == "windows"
            ):
                return "example.com/game\nexample.com/windows"
            if command[:3] == ["go", "list", "-m"]:
                return json.dumps(
                    {
                        "Path": "example.com/windows",
                        "Version": "v1.0.0",
                        "Dir": str(dependency),
                    }
                )
            return self.fake_run(command, cwd, env)

        self.execute(with_dependency)
        archive = next(self.output.glob("*darwin*.tar.gz"))
        with tarfile.open(archive) as contents:
            notice = next(
                m
                for m in contents
                if m.name.endswith("notices/example.com/windows@v1.0.0/LICENSE.txt")
            )
            stream = contents.extractfile(notice)
            assert stream is not None
            with stream:
                self.assertEqual(stream.read(), b"Windows dependency license")

    def test_missing_production_dependency_fails_before_build(self) -> None:
        def missing_dependency(
            command: list[str], cwd: Path, env: Mapping[str, str] | None = None
        ) -> str:
            if command[:3] == ["go", "list", "-mod=readonly"]:
                return "example.com/missing"
            if command[:3] == ["go", "list", "-m"]:
                return json.dumps({"Path": "example.com/missing", "Version": "v1.0.0"})
            return self.fake_run(command, cwd, env)

        with self.assertRaisesRegex(ValueError, "not provisioned"):
            self.execute(missing_dependency)
        self.assertFalse(self.output.exists())

    def test_homebrew_go_license_beside_libexec_is_packaged(self) -> None:
        prefix = self.base / "go-prefix"
        goroot = prefix / "libexec"
        goroot.mkdir(parents=True)
        (goroot / "PATENTS").write_text(
            "patent notice is not a license", encoding="utf-8"
        )
        (prefix / "LICENSE").write_text("Go BSD license", encoding="utf-8")

        def homebrew(
            command: list[str], cwd: Path, env: Mapping[str, str] | None = None
        ) -> str:
            if command == ["go", "env", "GOROOT"]:
                return str(goroot)
            return self.fake_run(command, cwd, env)

        self.execute(homebrew)
        with zipfile.ZipFile(next(self.output.glob("*.zip"))) as archive:
            license_name = next(
                name
                for name in archive.namelist()
                if name.endswith("notices/go/LICENSE")
            )
            self.assertEqual(archive.read(license_name), b"Go BSD license")

    def test_patent_notice_without_license_is_rejected(self) -> None:
        goroot = self.base / "go-without-license"
        goroot.mkdir()
        (goroot / "PATENTS").write_text(
            "patent notice is not a license", encoding="utf-8"
        )

        def no_license(
            command: list[str], cwd: Path, env: Mapping[str, str] | None = None
        ) -> str:
            if command == ["go", "env", "GOROOT"]:
                return str(goroot)
            return self.fake_run(command, cwd, env)

        with self.assertRaisesRegex(ValueError, "no dependency license"):
            self.execute(no_license)
        self.assertFalse(self.output.exists())


if __name__ == "__main__":
    unittest.main()
