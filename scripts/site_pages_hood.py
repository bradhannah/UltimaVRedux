"""Under the hood: format guides rendered from Game Facts, and the file catalog.

Guides keep their rule IDs, status badges and links to the Redux code that reads
each format. Only the public Game Facts text is rendered; nothing here quotes the
original game's files.
"""

import posixpath
import re
from collections.abc import Callable
from dataclasses import dataclass

import site_hood_walkthrough as walkthrough
from gamefacts_public import field, require
from site_context import (
    Context,
    Entry,
    crumbs,
    esc,
    github,
    page,
    rule_badges,
)
from site_data import objects, strings
from site_markdown import inline, render

METADATA = re.compile(
    r"^\*\*(Kind|Uses|Original|Redux|Implementation|Tests):\*\*.*$", re.MULTILINE
)
STATUS_LABELS = {"documented": "Documented", "partial": "Partly", "not_yet": "Not yet"}


@dataclass(frozen=True)
class Topic:
    """A Game Facts topic published as an Under the hood guide."""

    slug: str
    stem: str
    title: str
    summary: str


TOPICS = (
    Topic(
        "graphics",
        "GRAPHICS_FORMATS",
        "Decoding graphics",
        "LZW, archives, pixels, palette and the tile sheet",
    ),
    Topic(
        "saved-gam",
        "SAVED_GAM_STRUCTURE",
        "SAVED.GAM byte map",
        "Every region of the save, and the object lists beside it",
    ),
    Topic(
        "map-files",
        "MAP_FILE_FORMATS",
        "Map files and locations",
        "World chunks, settlement floors, location numbers and entrances",
    ),
    Topic(
        "text-files",
        "TEXT_FILE_FORMATS",
        "Text files",
        "Look descriptions, merchant phrases and the ending",
    ),
    Topic(
        "data-ovl",
        "DATA_OVL_LAYOUT",
        "DATA.OVL index",
        "Where each name list, price table and lookup table lives",
    ),
    Topic(
        "combat-maps",
        "BRIT_CBT_FORMAT",
        "Combat maps",
        "Britannia combat records, formations and tactical bounds",
    ),
    Topic(
        "signs-and-fonts",
        "SIGNS_AND_FONTS",
        "Signs and fonts",
        "Character sets, sign records and symbolic displays",
    ),
    Topic(
        "talk-system",
        "TALK_SYSTEM_STRUCTURE",
        "Talk scripts",
        "Records, the word dictionary and the control codes in conversations",
    ),
    Topic(
        "tiles",
        "TILES",
        "Tile properties",
        "Tile identity, movement, occupancy and animation",
    ),
)


def document_path(topic: Topic) -> str:
    """Return the repository path of a topic's Game Facts file."""
    return f"docs/gamefacts/{topic.stem}.md"


def make_resolver(ctx: Context, source: str) -> Callable[[str], str]:
    """Resolve links: published topics stay here, the rest go to the repository."""

    def resolve(target: str) -> str:
        if target.startswith(("http://", "https://", "mailto:", "#")):
            return target
        path, _, fragment = target.partition("#")
        resolved = posixpath.normpath(posixpath.join(posixpath.dirname(source), path))
        suffix = f"#{fragment}" if fragment else ""
        if resolved in ctx.topic_pages:
            return ctx.topic_pages[resolved] + suffix
        return github(resolved) + suffix

    return resolve


def rule_href(ctx: Context, identifier: str) -> str:
    """Link a rule to its guide on this site when published, else to GitHub."""
    document = ctx.rules[identifier]["document"]
    if document in ctx.topic_pages:
        return f"{ctx.topic_pages[document]}#{identifier.lower()}"
    return github(f"{document}#{identifier.lower()}")


def code_links(text: str) -> str:
    """Render a rule's Implementation and Tests paths as links into the repository."""
    blocks = []
    for name in ("Implementation", "Tests"):
        value = field(text, name, required=False)
        paths = re.findall(r"`([^`]+)`", value) if value != "none" else []
        if paths:
            links = "".join(
                f'<li><a href="{esc(github(p))}"><code>{esc(p)}</code></a></li>'
                for p in paths
            )
            blocks.append(f"<div><strong>{name}</strong><ul>{links}</ul></div>")
    return f'<div class="code-links">{"".join(blocks)}</div>' if blocks else ""


def render_rule(ctx: Context, identifier: str, resolve: Callable[[str], str]) -> str:
    """Render one Game Facts rule with its badges, text and code links."""
    text = ctx.rules[identifier]["text"]
    lines = text.split("\n")
    title = lines[2].strip().strip("*") if len(lines) > 2 else identifier
    body = METADATA.sub("", "\n".join(lines[3:]))
    return (
        f'<section class="rule" id="{identifier.lower()}">'
        f'<div class="rule-head"><h2>{inline(title, resolve)}</h2></div>'
        f'<div class="rule-meta"><span class="mono">{identifier}</span>'
        f"{rule_badges(ctx, identifier)}</div>"
        f"{render(body, resolve, heading_offset=1)}{code_links(text)}</section>"
    )


def build_topic(ctx: Context, topic: Topic) -> list[str]:
    """Build one guide page and return the rule IDs it renders."""
    source = document_path(topic)
    resolve = make_resolver(ctx, source)
    document = ctx.documents[source]
    identifiers = list(document["rules"])
    preamble = "\n".join(
        line for line in document["preamble"].split("\n") if not line.startswith("# ")
    )
    toc = "".join(
        f'<li><a href="#{i.lower()}"><span class="mono">{i}</span> '
        f'{inline(ctx.rules[i]["text"].split(chr(10))[2].strip("*"), resolve)}</a></li>'
        for i in identifiers
    )
    rule_toc = (
        '<nav class="toc" aria-label="Rules in this topic">'
        f"<strong>In this topic</strong><ul>{toc}</ul></nav>"
    )
    walk = walkthrough.build(ctx, topic.slug, identifiers)
    if walk is None:
        intro = f'<div class="lede">{render(preamble, resolve)}</div>' + rule_toc
    else:
        intro = (
            f'<p class="lede">{walk.lede}</p>'
            '<nav class="toc" aria-label="On this page"><strong>On this page</strong>'
            f"<ul>{walk.contents}</ul></nav>{walk.body}"
            '<h2 class="reference" id="rule-reference">Rule reference</h2>'
            f"{render(preamble, resolve)}{rule_toc}"
        )
    body = (
        crumbs(
            ("The original game", "/original/"),
            ("Under the hood", "/original/hood/"),
            (topic.title, ""),
        )
        + '<p class="hood-note">FOR MODDERS AND HACKERS</p>'
        + f"<h1>{esc(topic.title)}</h1>"
        + intro
        + "".join(render_rule(ctx, i, resolve) for i in identifiers)
        + '<aside class="sources"><h2>Source</h2><p>This guide is generated from '
        f'<a href="{esc(github(source))}">{esc(source)}</a>. Offsets are in hex unless '
        "stated; multibyte values are little-endian. Nothing here contains the "
        "game's art, maps or text. Point your tools at your own copy.</p></aside>"
    )
    page(
        ctx,
        f"original/hood/{topic.slug}/index.html",
        topic.title,
        body,
        "original",
        topic.summary,
    )
    return identifiers


def derive_status(ctx: Context, row: dict[str, object]) -> str:
    """Documented, partial or not yet, from the Original status of a row's rules."""
    override = row.get("status")
    if override:
        require(str(override) in STATUS_LABELS, f"bad status override: {override}")
        return str(override)
    rules = strings(row["rules"])
    if not rules:
        return "not_yet"
    states = {ctx.status(identifier) for identifier in rules}
    return "documented" if states == {"Established"} else "partial"


def catalog_row(ctx: Context, row: dict[str, object]) -> tuple[str, str]:
    """Render one catalog row and return it with its derived status."""
    status = derive_status(ctx, row)
    rules = strings(row["rules"])
    for identifier in rules:
        require(identifier in ctx.rules, f"file catalog: unknown rule {identifier}")
    links = ", ".join(
        f'<a class="mono" href="{esc(rule_href(ctx, i))}">{i}</a>' for i in rules
    )
    notes = ""
    if row.get("guide"):
        stem = {t.slug: t for t in TOPICS}[str(row["guide"])]
        notes = f' <a href="/original/hood/{stem.slug}/">Guide</a>'
    if row.get("doc"):
        notes = f' <a href="{esc(github(str(row["doc"])))}">Document</a>'
    chip_ = (
        f'<span class="badge {status.replace("_", "-")}">{STATUS_LABELS[status]}</span>'
    )
    html_row = (
        f'<tr><th scope="row" class="mono">{esc(row["file"])}</th>'
        f'<td>{esc(row["holds"])}</td><td>{esc(row["format"])}{notes}</td>'
        f"<td>{chip_}</td><td>{links or '—'}</td></tr>"
    )
    return html_row, status


def catalog(ctx: Context) -> tuple[str, dict[str, int]]:
    """Render the full file catalog and tally its statuses."""
    data = ctx.load("file_catalog.json")
    tally = {key: 0 for key in STATUS_LABELS}
    groups = []
    for group in objects(data["groups"]):
        rows = []
        for row in objects(group["rows"]):
            html_row, status = catalog_row(ctx, row)
            tally[status] += 1
            rows.append(html_row)
        groups.append(
            f'<tbody><tr><th scope="rowgroup" colspan="5" class="mono">'
            f'{esc(str(group["name"]).upper())}</th></tr>{"".join(rows)}</tbody>'
        )
    table = (
        '<div class="table-wrap"><table id="file-catalog" data-row-filter-target>'
        '<thead><tr><th scope="col">File</th><th scope="col">Holds</th>'
        '<th scope="col">Format in brief</th><th scope="col">Status</th>'
        '<th scope="col">Rules</th></tr></thead>'
        f'{"".join(groups)}</table></div>'
    )
    return table, tally


def build_hub(ctx: Context) -> None:
    """Build the Under the hood hub with its file catalog."""
    table, tally = catalog(ctx)
    boxes = "".join(
        f'<div class="stat"><div class="value">{tally[key]}</div>'
        f'<div class="label">{label.lower()}</div></div>'
        for key, label in STATUS_LABELS.items()
    )
    cards = "".join(
        f'<a class="item" href="/original/hood/{t.slug}/"><b>{esc(t.title)}</b>'
        f"<span>{esc(t.summary)}</span></a>"
        for t in TOPICS
    )
    body = (
        crumbs(("The original game", "/original/"), ("Under the hood", ""))
        + '<p class="hood-note">FOR MODDERS AND HACKERS</p><h1>Under the hood</h1>'
        "<p class='lede'>How the 1988 DOS game stores its data, in enough detail to "
        "write your own tools. Nothing here contains the game's art, maps or text. "
        "Point your tools at your own copy.</p>"
        f'<div class="stats">{boxes}</div>'
        "<p class='muted'>Little-endian · offsets in hex, sizes in decimal · rule IDs "
        "open the Game Facts entry · code links open Redux's MIT-licensed readers. "
        "Counts are of the rows below.</p>"
        f'<h2>Start here</h2><div class="items">{cards}</div>'
        '<h2 id="every-file">Every file</h2>'
        '<div class="tools" data-js-only hidden><label>Find a file '
        '<input type="search" data-row-filter="file-catalog"></label></div>'
        f"{table}"
        "<p class='muted'><b>Not here:</b> the game itself, or other platforms. The "
        "Amiga, Apple II, C64 and other versions use different files; this covers "
        "the MS-DOS release.</p>"
        f'<p>Spotted a wrong byte? <a href="{esc(github("docs/gamefacts/README.md"))}">'
        "Tell us</a>, with the file and offset.</p>"
    )
    page(
        ctx,
        "original/hood/index.html",
        "Under the hood",
        body,
        "original",
        "File formats of the MS-DOS Ultima V, catalogued, with guides for modders.",
    )
    ctx.add(
        Entry(
            "hood",
            "hood",
            "Every file, catalogued",
            "Which files are documented, and where",
            "/original/hood/",
        )
    )


def build_all(ctx: Context) -> None:
    """Register every guide, then render guides and the hub."""
    for topic in TOPICS:
        require(document_path(topic) in ctx.documents, f"{topic.stem} not cataloged")
        ctx.topic_pages[document_path(topic)] = f"/original/hood/{topic.slug}/"
    for topic in TOPICS:
        identifiers = build_topic(ctx, topic)
        ctx.add(
            Entry(
                "hood",
                topic.slug,
                topic.title,
                topic.summary,
                f"/original/hood/{topic.slug}/",
                identifiers,
            )
        )
    build_hub(ctx)
