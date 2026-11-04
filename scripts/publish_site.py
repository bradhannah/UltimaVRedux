#!/usr/bin/env python3
"""Publish the public site to the gh-pages branch that GitHub Pages serves.

Usage: python scripts/publish_site.py [--remote origin] [--branch gh-pages] [--no-push]

Builds the site from this checkout into a temporary directory, records it as the
next commit on the publication branch without touching the working tree, index or
current branch, and pushes it. The same command runs by hand (make site-publish)
and in .github/workflows/pages.yml, so publishing never depends on GitHub Actions.
See docs/SITE.md.
"""

import argparse
import os
import subprocess
import sys
import tempfile
from dataclasses import dataclass
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
BUILD_MARKER = ".site-build"
# Everything the generator reads; uncommitted edits here would publish unreviewed text.
SITE_INPUTS = ("site", "docs/gamefacts", "scripts")


class PublishError(Exception):
    """A publication precondition or Git operation failed."""


@dataclass(frozen=True)
class Destination:
    """The remote branch that GitHub Pages serves."""

    remote: str = "origin"
    branch: str = "gh-pages"


def git(
    root: Path, *args: str, env: dict[str, str] | None = None, cwd: Path | None = None
) -> str:
    """Run a Git command against root's repository and return stripped stdout."""
    result = subprocess.run(
        ["git", "-C", str(cwd or root), *args],
        capture_output=True,
        text=True,
        env=env,
        check=False,
    )
    if result.returncode != 0:
        raise PublishError(f"git {' '.join(args)} failed: {result.stderr.strip()}")
    return result.stdout.strip()


def require_clean_inputs(root: Path) -> None:
    """Refuse to publish uncommitted or untracked site inputs."""
    status = git(root, "status", "--porcelain", "--", *SITE_INPUTS)
    if status:
        raise PublishError(
            "site inputs have uncommitted changes; commit them first:\n" + status
        )


def remote_tip(root: Path, destination: Destination) -> str | None:
    """Fetch the publication branch and return its tip, or None on first publish."""
    remote, branch = destination.remote, destination.branch
    if not git(root, "ls-remote", "--heads", remote, branch):
        return None
    git(root, "fetch", "--quiet", remote, f"refs/heads/{branch}")
    return git(root, "rev-parse", "FETCH_HEAD")


def tree_for(root: Path, site: Path, scratch: Path) -> str:
    """Write site's files as a Git tree using a private index; return the tree id."""
    git_dir = git(root, "rev-parse", "--absolute-git-dir")
    env = {**os.environ, "GIT_INDEX_FILE": str(scratch / "publish.index")}
    bare = ("--git-dir", git_dir, "--work-tree", str(site))
    # --force keeps files such as .nojekyll that personal ignore rules might hide.
    git(root, *bare, "add", "--all", "--force", ".", env=env, cwd=site)
    return git(root, *bare, "write-tree", env=env, cwd=site)


def publish(
    root: Path, site: Path, destination: Destination, *, source: str, push: bool
) -> str | None:
    """Commit site as the next publication commit; return it, or None if unchanged."""
    parent = remote_tip(root, destination)
    with tempfile.TemporaryDirectory(prefix="site-publish-") as scratch:
        tree = tree_for(root, site, Path(scratch))
    if parent is not None and git(root, "rev-parse", f"{parent}^{{tree}}") == tree:
        return None
    parents = ("-p", parent) if parent is not None else ()
    message = f"Publish site from {source}"
    commit = git(root, "commit-tree", tree, *parents, "-m", message)
    if push:
        # A plain push only fast-forwards; a concurrent publish makes it fail safely.
        target = f"{commit}:refs/heads/{destination.branch}"
        git(root, "push", "--quiet", destination.remote, target)
    return commit


def build(root: Path, out: Path) -> None:
    """Run the site generator into out and drop its local rebuild marker."""
    subprocess.run(
        [sys.executable, "-B", str(root / "scripts" / "build_site.py"), str(out)],
        check=True,
    )
    (out / BUILD_MARKER).unlink(missing_ok=True)


def main(argv: list[str]) -> int:
    """Build and publish the site; return a process exit status."""
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--remote", default="origin", help="remote to publish to")
    parser.add_argument("--branch", default="gh-pages", help="publication branch")
    parser.add_argument(
        "--no-push", action="store_true", help="build and commit, but do not push"
    )
    args = parser.parse_args(argv)
    try:
        require_clean_inputs(ROOT)
        source = git(ROOT, "rev-parse", "--short=12", "HEAD")
        with tempfile.TemporaryDirectory(prefix="site-build-") as temporary:
            out = Path(temporary) / "site"
            build(ROOT, out)
            commit = publish(
                ROOT,
                out,
                Destination(args.remote, args.branch),
                source=source,
                push=not args.no_push,
            )
    except (PublishError, subprocess.CalledProcessError) as error:
        print(f"publish_site: {error}", file=sys.stderr)
        return 1
    if commit is None:
        print(f"{args.remote}/{args.branch} already matches the site from {source}.")
    elif args.no_push:
        print(f"Built publication commit {commit} from {source}; not pushed.")
    else:
        print(f"Published {commit} from {source} to {args.remote}/{args.branch}.")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
