"""Shared state and HTML helpers for the public site generator.

Pages are written with site-absolute links such as `/original/bestiary/`. When a
page is stored, those links become relative so the site works from any base path
(GitHub project pages live below `/UltimaVRedux/`).
"""

import html
import json
import posixpath
import re
from dataclasses import dataclass, field
from pathlib import Path

from gamefacts_public import Document, JSONValue, Rule, require
from gamefacts_public import field as rule_field
from site_data import Record

REPO_URL = "https://github.com/bradhannah/UltimaVRedux"
BLOB_URL = f"{REPO_URL}/blob/main/"
NAV = (
    ("home", "Home", "/"),
    ("original", "The original game", "/original/"),
    ("redux", "Ultima V Redux", "/redux/"),
    ("myths", "Myths", "/original/myths/"),
)
ATTRIBUTE = re.compile(r"""(href|src|data-json)=(["'])(/[^"']*)\2""")


@dataclass
class Entry:
    """A built page listed on a landing page."""

    tier: str
    slug: str
    title: str
    summary: str
    path: str
    rules: list[str] = field(default_factory=list)


@dataclass
class Context:  # pylint: disable=too-many-instance-attributes
    """Everything a page builder needs, plus the pages built so far.

    It is one shared build record; splitting it would only scatter the same state.
    """

    root: Path
    rules: dict[str, Rule]
    documents: dict[str, Document]
    datasets: dict[str, Record]
    content: Path
    pages: dict[str, str] = field(default_factory=dict)
    entries: list[Entry] = field(default_factory=list)
    topic_pages: dict[str, str] = field(default_factory=dict)
    notice: str = ""

    def load(self, name: str) -> dict[str, JSONValue]:
        """Read one curated JSON content file."""
        data = json.loads((self.content / name).read_text(encoding="utf-8"))
        require(isinstance(data, dict), f"{name} must hold an object")
        return data

    def status(self, identifier: str, name: str = "Original") -> str:
        """Return a rule's Original or Redux assessment."""
        return rule_field(self.rules[identifier]["text"], name)

    def add(self, entry: Entry) -> None:
        """Register a built page for the landing pages."""
        self.entries.append(entry)

    def tier(self, tier: str) -> list[Entry]:
        """List registered pages of one tier in registration order."""
        return [entry for entry in self.entries if entry.tier == tier]


def esc(value: object) -> str:
    """Escape text for HTML."""
    return html.escape(str(value), quote=True)


def slug(value: str) -> str:
    """Make a CSS-friendly slug from a status word."""
    return re.sub(r"[^a-z0-9]+", "-", value.lower()).strip("-")


def badge(kind: str, value: str) -> str:
    """Render a status badge whose text states the assessment, not just a colour."""
    return f'<span class="badge {slug(value)}">{esc(kind)}: {esc(value)}</span>'


def rule_badges(ctx: Context, identifier: str) -> str:
    """Render a rule's Original and Redux badges (groups have none)."""
    if ctx.rules[identifier]["kind"] != "rule":
        return ""
    return badge("Original", ctx.status(identifier)) + badge(
        "Redux", ctx.status(identifier, "Redux")
    )


def github(path: str) -> str:
    """Link to a file or anchor in the repository's main branch."""
    return BLOB_URL + path


def rule_url(ctx: Context, identifier: str) -> str:
    """Link to a rule's heading in its Game Facts topic on GitHub."""
    document = ctx.rules[identifier]["document"]
    return github(f"{document}#{identifier.lower()}")


def rule_link(ctx: Context, identifier: str) -> str:
    """Render a rule ID as a link to its Game Facts entry."""
    return f'<a class="mono" href="{esc(rule_url(ctx, identifier))}">{identifier}</a>'


def sources_footer(ctx: Context, identifiers: list[str]) -> str:
    """List the rules behind a curated page, flagging any not fully settled."""
    items = []
    for identifier in identifiers:
        flag = ""
        if ctx.rules[identifier]["kind"] == "rule" and (
            ctx.status(identifier) != "Established"
        ):
            flag = " " + badge("Original", ctx.status(identifier))
        items.append(f"<li>{rule_link(ctx, identifier)}{flag}</li>")
    return (
        '<aside class="sources" aria-labelledby="sources-h">'
        '<h2 id="sources-h">Where this comes from</h2>'
        '<p class="muted">Rule IDs open the Game Facts entry on GitHub.</p>'
        f"<ul>{''.join(items)}</ul></aside>"
    )


def spoiler(answer: str) -> str:
    """Wrap a puzzle answer in a reveal control. Answers appear nowhere else."""
    return (
        '<span class="spoiler" data-open="false">'
        '<button type="button" class="reveal" aria-expanded="false" hidden>'
        "Reveal</button>"
        f'<span class="answer" hidden>{esc(answer)}</span></span>'
    )


def hide_toggle() -> str:
    """The global 'hide puzzle answers' switch; it needs JavaScript to act."""
    return (
        '<label class="spoiler-toggle" data-js-only hidden>'
        '<input type="checkbox" data-hide-answers checked> Hide puzzle answers'
        "</label>"
    )


def relative(target: str, source: str) -> str:
    """Rewrite a site-absolute link as a link relative to the page's file."""
    path, hash_, fragment = target.partition("#")
    start = posixpath.dirname(source) or "."
    if path == "/":
        result = posixpath.relpath(".", start)
    else:
        result = posixpath.relpath(path.lstrip("/"), start)
    if path.endswith("/"):
        result = "./" if result == "." else result.rstrip("/") + "/"
    return result + hash_ + fragment


def localize(document: str, source: str) -> str:
    """Make every site-absolute href and src in a page relative."""
    return ATTRIBUTE.sub(
        lambda match: f'{match.group(1)}="{relative(match.group(3), source)}"', document
    )


def shell(ctx: Context, title: str, body: str, section: str, desc: str) -> str:
    """Wrap a page body in the site header, footer and metadata."""
    nav = "".join(
        f'<a href="{href}"'
        + (' aria-current="page"' if key == section else "")
        + f">{esc(label)}</a>"
        for key, label, href in NAV
    )
    notice = esc(ctx.notice)
    full_title = title if title == "Ultima V Redux" else f"{title} | Ultima V Redux"
    return (
        "<!doctype html>\n"
        '<html lang="en"><head><meta charset="utf-8">'
        '<meta name="viewport" content="width=device-width, initial-scale=1">'
        f"<title>{esc(full_title)}</title>"
        f'<meta name="description" content="{esc(desc)}">'
        '<link rel="preconnect" href="https://fonts.googleapis.com">'
        '<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>'
        '<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family='
        "IBM+Plex+Mono:wght@400;600;700&amp;family=IBM+Plex+Sans:wght@400;600"
        '&amp;display=swap">'
        '<link rel="stylesheet" href="/assets/style.css">'
        "<noscript><style>.answer[hidden]{display:inline !important}"
        ".reveal{display:none !important}</style></noscript>"
        '<script src="/assets/site.js" defer></script></head><body>'
        '<a class="skip" href="#main">Skip to content</a>'
        '<header class="site-header"><div class="wrap">'
        '<a class="brand" href="/">ULTIMA V REDUX</a>'
        f'<nav class="site-nav" aria-label="Main">{nav}</nav></div></header>'
        f'<main id="main"><div class="wrap">{body}</div></main>'
        '<footer class="site-footer"><div class="wrap">'
        f"<p>{notice}</p>"
        '<div class="footer-links">'
        '<a href="/redux/#credits">Credits</a>'
        '<a href="/data/">Data</a>'
        f'<a href="{REPO_URL}">Source on GitHub</a>'
        f'<a href="{REPO_URL}/issues/new/choose">Report a bug</a>'
        "</div></div></footer></body></html>\n"
    )


# Every builder passes these six facts positionally; a wrapper type would only
# restate them, so the argument-count checks are waived here.
# pylint: disable-next=too-many-arguments,too-many-positional-arguments
def page(
    ctx: Context,
    path: str,
    title: str,
    body: str,
    section: str,
    description: str,
) -> None:
    """Store a finished page, localizing its links."""
    ctx.pages[path] = localize(shell(ctx, title, body, section, description), path)


def crumbs(*trail: tuple[str, str]) -> str:
    """Render a breadcrumb trail of (label, href) pairs; the last has no link."""
    parts = [
        f'<a href="{href}">{esc(label)}</a>' if href else f"<span>{esc(label)}</span>"
        for label, href in trail
    ]
    return f'<nav class="crumbs" aria-label="Breadcrumb">{" / ".join(parts)}</nav>'


def chip(text: str, colour: str = "") -> str:
    """Render a small bordered label."""
    return f'<span class="chip {colour}">{esc(text)}</span>'
