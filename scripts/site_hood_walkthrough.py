"""Curated step-by-step walkthroughs above an Under the hood guide's rule reference.

Prose lives in `site/content/hood/<slug>.json`. Numbers that can be derived are
computed here instead of typed: bit diagrams come from their example byte, archive
offsets are recomputed and checked against the rule text, and the palette is read
from the Redux code. A walkthrough cites only rules from its own guide.
"""

import re
from collections.abc import Callable
from dataclasses import dataclass
from typing import cast

from gamefacts_public import JSONValue, require
from site_context import Context, badge, chip, esc, github
from site_data import objects, strings
from site_pages_content import plain
from site_pages_content import render_block as render_common

type Obj = dict[str, JSONValue]

PALETTE_SOURCE = "pkg/graphics/legacy/model.go"
PALETTE_ENTRY = re.compile(r"color\.RGBA\{R: (\d+), G: (\d+), B: (\d+), A: 255\}")
SECTION_ID = re.compile(r"[a-z][a-z0-9-]*")

# Archive, offset-table entry size in bytes, entries, rule that states the offset.
ARCHIVE_OFFSETS = (
    ("Dungeon panels (28 slots)", 4, 28, "GFX-0002"),
    ("ITEMS (20 images + 20 masks)", 2, 40, "GFX-0003"),
    ("MON0–7 (6 images + 6 masks)", 2, 12, "GFX-0003"),
    ("TITLE (10 records)", 2, 10, "GFX-0004"),
    ("BRITISH, WD (1 record)", 2, 1, "GFX-0004"),
    ("PROPORT.PCS (91 glyphs)", 2, 91, "GFX-0004"),
)


@dataclass(frozen=True)
class Walkthrough:
    """A rendered walkthrough: lede, body before the rules, and its contents list."""

    lede: str
    body: str
    contents: str


def rows_of(value: JSONValue) -> list[list[str]]:
    """Narrow a JSON value to a list of string rows."""
    require(isinstance(value, list), "table rows must be a list")
    return [strings(row) for row in cast(list[JSONValue], value)]


def table(head: list[str], rows: list[list[str]]) -> str:
    """Render a plain table whose cells may hold inline Markdown."""
    header = "".join(f'<th scope="col">{esc(cell)}</th>' for cell in head)
    body = "".join(
        "<tr>" + "".join(f"<td>{plain(cell)}</td>" for cell in row) + "</tr>"
        for row in rows
    )
    return (
        f'<div class="table-wrap"><table><thead><tr>{header}</tr></thead>'
        f"<tbody>{body}</tbody></table></div>"
    )


def archive_offsets(ctx: Context) -> str:
    """Recompute first-record offsets and require the rules to state the same."""
    rows = []
    for name, size, entries, rule in ARCHIVE_OFFSETS:
        first = 2 + size * entries
        stated = f"`0x{first:02X}`"
        require(
            stated in ctx.rules[rule]["text"],
            f"{rule} no longer states the first offset {stated} for {name}",
        )
        rows.append([name, f"2 + {size} × {entries}", f"{first} = `0x{first:02X}`"])
    return table(["Archive", "Table ends at", "First record"], rows)


def ega_palette(ctx: Context) -> str:
    """Render the palette Redux's exporter uses, read from its source."""
    source = (ctx.root / PALETTE_SOURCE).read_text(encoding="utf-8")
    start = source.index("var EGAPalette")
    entries = PALETTE_ENTRY.findall(source[start : source.index("\n}", start)])
    require(len(entries) == 16, f"{PALETTE_SOURCE}: expected 16 EGA colours")
    cells = "".join(
        f'<li><span class="swatch" style="background:#{int(r):02x}{int(g):02x}'
        f'{int(b):02x}"></span><span class="mono">{index:>2} '
        f"#{int(r):02X}{int(g):02X}{int(b):02X}</span></li>"
        for index, (r, g, b) in enumerate(entries)
    )
    return (
        f'<ul class="palette" aria-label="EGA palette indexes 0 to 15">{cells}</ul>'
        f'<p class="muted">Read from <a href="{esc(github(PALETTE_SOURCE))}">'
        f"<code>{PALETTE_SOURCE}</code></a> when this page was built.</p>"
    )


def bits(block: Obj) -> str:
    """Draw one example byte's bits and say what they decode to."""
    value = int(str(block["byte"]), 16)
    require(0 <= value <= 0xFF, f"example byte out of range: {block['byte']}")
    digits = f"{value:08b}"
    mode = str(block["mode"])
    if mode == "nibbles":
        cells = (
            '<span class="nibble">'
            + "".join(f'<span class="bit">{d}</span>' for d in digits[:4])
            + '</span><span class="nibble">'
            + "".join(f'<span class="bit">{d}</span>' for d in digits[4:])
            + "</span>"
        )
        result = (
            f"left pixel = high nibble = {value >> 4}; "
            f"right pixel = low nibble = {value & 0xF}"
        )
    else:
        require(mode == "msb", f"unknown bit mode: {mode}")
        cells = "".join(f'<span class="bit">{d}</span>' for d in digits)
        result = "pixels, left to right: " + " ".join(digits)
    return (
        f'<figure class="bits"><div class="bit-row" aria-hidden="true">{cells}</div>'
        f'<figcaption><span class="mono">{esc(block["byte"])}</span>: '
        f"{esc(result)}. {plain(str(block['caption']))}</figcaption></figure>"
    )


def bitfield(block: Obj) -> str:
    """Draw a command byte's fields, high bit first, covering bits 7 to 0."""
    fields = objects(block["fields"])
    expected = 7
    cells = []
    for item in fields:
        high, low = int(str(item["high"])), int(str(item["low"]))
        require(high == expected and low <= high, "bit fields must run 7 down to 0")
        expected = low - 1
        span = high - low + 1
        bits_label = str(high) if span == 1 else f"{high}–{low}"
        cells.append(
            f'<td colspan="{span}"><span class="mono">bit {bits_label}</span>'
            f"<br>{esc(item['label'])}</td>"
        )
    require(expected == -1, "bit fields must end at bit 0")
    header = "".join(f'<th scope="col">{n}</th>' for n in range(7, -1, -1))
    return (
        f'<div class="table-wrap"><table class="bitfield"><thead><tr>{header}</tr>'
        f'</thead><tbody><tr>{"".join(cells)}</tr></tbody></table></div>'
    )


def render_block(ctx: Context, block: Obj) -> str:
    """Render a walkthrough block, falling back to the shared content blocks."""
    computed: dict[str, Callable[[Context], str]] = {
        "archive_offsets": archive_offsets,
        "ega_palette": ega_palette,
    }
    special: dict[str, Callable[[Obj], str]] = {
        "chips": lambda b: (
            f'<div class="chip-group"><span class="label">{esc(b["label"])}</span>'
            + "".join(chip(item) for item in strings(b["items"]))
            + "</div>"
        ),
        "table": lambda b: table(strings(b["head"]), rows_of(b["rows"])),
        "code": lambda b: (
            (f'<p class="muted">{esc(b["caption"])}</p>' if "caption" in b else "")
            + f'<pre><code>{esc(chr(10).join(strings(b["lines"])))}</code></pre>'
        ),
        "bits": bits,
        "bitfield": bitfield,
    }
    kind = str(block["type"])
    if kind == "computed":
        name = str(block["name"])
        require(name in computed, f"unknown walkthrough computation: {name}")
        return computed[name](ctx)
    if kind in special:
        return special[kind](block)
    return render_common(ctx, block)


def section(ctx: Context, item: Obj, own_rules: set[str]) -> tuple[str, str]:
    """Render one walkthrough section and its contents-list entry."""
    identifier = str(item["id"])
    require(SECTION_ID.fullmatch(identifier) is not None, f"bad id: {identifier}")
    rules = strings(item["rules"])
    for rule in rules:
        require(rule in own_rules, f"walkthrough {identifier} cites foreign {rule}")
    step = f'<span class="step">{esc(item["step"])}.</span> ' if "step" in item else ""
    meta = "".join(
        f'<span><a class="mono" href="#{rule.lower()}">{rule}</a> '
        f'{badge("Original", ctx.status(rule))}</span>'
        for rule in rules
    )
    title = esc(item["title"])
    html = (
        f'<section class="walk" id="{identifier}"><h2>{step}{title}</h2>'
        f'<div class="rule-meta">{meta}</div>'
        + "".join(render_block(ctx, b) for b in objects(item["blocks"]))
        + "</section>"
    )
    return html, f'<li><a href="#{identifier}">{step}{title}</a></li>'


def build(ctx: Context, slug: str, own_rules: list[str]) -> Walkthrough | None:
    """Render a guide's walkthrough, or None when the guide has none."""
    if not (ctx.content / "hood" / f"{slug}.json").exists():
        return None
    data = ctx.load(f"hood/{slug}.json")
    for path in strings(data["code"]):
        require((ctx.root / path).is_file(), f"hood/{slug}.json: missing {path}")
    code = " · ".join(
        f'<a href="{esc(github(path))}"><code>{esc(path.rsplit("/", 1)[-1])}</code></a>'
        for path in strings(data["code"])
    )
    pipeline = "".join(
        f'<li><span class="label">{index}. {esc(step["label"])}</span>'
        f"{esc(step['text'])}</li>"
        for index, step in enumerate(objects(data["pipeline"]), start=1)
    )
    notes = "".join(
        f"<li>{plain(note)}</li>" for note in strings(data["pipeline_notes"])
    )
    sections = objects(data["sections"])
    identifiers = [str(item["id"]) for item in sections]
    require(
        len(set(identifiers)) == len(identifiers), f"hood/{slug}.json: duplicate id"
    )
    rendered = [section(ctx, item, set(own_rules)) for item in sections]
    not_yet = "".join(f"<li>{plain(text)}</li>" for text in strings(data["not_yet"]))
    body = (
        f'<p class="code-line">Working code (MIT): {code}</p>'
        f'<ol class="pipeline" aria-label="Decoding steps">{pipeline}</ol>'
        f'<ul class="pipeline-notes">{notes}</ul>'
        + "".join(html for html, _ in rendered)
        + '<section class="walk not-yet" id="not-yet"><h2>Not written up yet</h2>'
        f"<ul>{not_yet}</ul></section>"
    )
    contents = (
        "".join(entry for _, entry in rendered)
        + '<li><a href="#not-yet">Not written up yet</a></li>'
        + '<li><a href="#rule-reference">Rule reference</a></li>'
    )
    return Walkthrough(plain(str(data["intro"])), body, contents)
