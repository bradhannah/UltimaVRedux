"""Regression tests for publishing the built site to the gh-pages branch."""

import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest import mock

import publish_site

ISOLATED_GIT = {
    "GIT_CONFIG_GLOBAL": os.devnull,
    "GIT_CONFIG_NOSYSTEM": "1",
    "GIT_AUTHOR_NAME": "Site Test",
    "GIT_AUTHOR_EMAIL": "site@example.invalid",
    "GIT_COMMITTER_NAME": "Site Test",
    "GIT_COMMITTER_EMAIL": "site@example.invalid",
}


def run_git(cwd: Path, *args: str) -> str:
    """Run a Git command for fixture setup and return stripped stdout."""
    return subprocess.run(
        ["git", "-C", str(cwd), *args], capture_output=True, text=True, check=True
    ).stdout.strip()


class PublishSiteTest(unittest.TestCase):
    """Publication commits only the built files and only fast-forwards."""

    def setUp(self) -> None:
        self.enterContext(mock.patch.dict(os.environ, ISOLATED_GIT))
        base = Path(tempfile.mkdtemp(prefix="publish-site-test-"))
        self.addCleanup(shutil.rmtree, base)
        self.remote = base / "remote.git"
        self.root = base / "checkout"
        self.site = base / "built"
        run_git(base, "init", "--quiet", "--bare", str(self.remote))
        run_git(base, "init", "--quiet", "--initial-branch=main", str(self.root))
        (self.root / "site").mkdir()
        (self.root / "site" / "page.json").write_text("{}\n", encoding="utf-8")
        run_git(self.root, "add", ".")
        run_git(self.root, "commit", "--quiet", "-m", "Source")
        run_git(self.root, "remote", "add", "origin", str(self.remote))
        self.site.mkdir()
        (self.site / ".nojekyll").write_text("", encoding="utf-8")
        (self.site / "index.html").write_text("<p>one</p>\n", encoding="utf-8")
        (self.site / "data").mkdir()
        (self.site / "data" / "index.json").write_text("{}\n", encoding="utf-8")

    def publish(self) -> str | None:
        """Publish the fixture site to the bare remote."""
        return publish_site.publish(
            self.root,
            self.site,
            publish_site.Destination("origin", "gh-pages"),
            source="abc123",
            push=True,
        )

    def test_first_publish_then_unchanged_then_fast_forward(self) -> None:
        """The branch is created, left alone when unchanged, then advanced."""
        first = self.publish()
        self.assertIsNotNone(first)
        published = run_git(self.remote, "ls-tree", "-r", "--name-only", "gh-pages")
        self.assertEqual(
            published.split(), [".nojekyll", "data/index.json", "index.html"]
        )
        self.assertEqual(
            run_git(self.remote, "log", "-1", "--format=%s", "gh-pages"),
            "Publish site from abc123",
        )
        self.assertIsNone(self.publish(), "an unchanged site must not add a commit")

        (self.site / "index.html").write_text("<p>two</p>\n", encoding="utf-8")
        second = self.publish()
        self.assertEqual(run_git(self.remote, "rev-parse", "gh-pages"), second)
        self.assertEqual(run_git(self.remote, "rev-parse", "gh-pages^"), first)
        self.assertEqual(run_git(self.root, "branch", "--show-current"), "main")
        self.assertEqual(run_git(self.root, "status", "--porcelain"), "")

    def test_uncommitted_site_inputs_are_refused(self) -> None:
        """Publishing never ships text that has not been committed."""
        publish_site.require_clean_inputs(self.root)
        (self.root / "site" / "draft.json").write_text("{}\n", encoding="utf-8")
        with self.assertRaisesRegex(publish_site.PublishError, "uncommitted"):
            publish_site.require_clean_inputs(self.root)


if __name__ == "__main__":
    unittest.main()
