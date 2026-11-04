"""Landing pages: Home, The original game, and the Data index."""

from site_context import (
    REPO_URL,
    Context,
    Entry,
    chip,
    crumbs,
    esc,
    github,
    hide_toggle,
    page,
)
from site_data import objects
from site_pages_content import VERDICTS

DEPTHS = (
    ("facts", "FACTS AND FIGURES", "for playing"),
    ("how", "HOW IT WORKS", "for studying"),
    ("hood", "UNDER THE HOOD", "for modding"),
)
HOME_LIMIT = {"facts": 8, "how": 5, "hood": 3}
HOME_HOOD = ("graphics", "saved-gam", "hood")
REDUX_LINKS = (
    ("Get started", "Download, then point it at your game", "/redux/#start-h"),
    (
        "Differences you'll notice",
        "Each change, original next to Redux",
        "/redux/differences/",
    ),
    ("Options", "What you can switch in Configure", "/redux/options/"),
    ("Known gaps", "Where Redux isn't faithful yet", "/redux/#gap-h"),
    ("Credits", "People, assets and licences", "/redux/#credits"),
)
SETTLED = {"Established"}


def unsettled(ctx: Context, entry: Entry) -> str:
    """Return a chip naming the weakest Original status among an entry's rules.

    Under the hood guides cover whole topics, so they carry no summary chip.
    """
    if entry.tier == "hood":
        return ""
    states = {
        ctx.status(r) for r in entry.rules if ctx.rules[r]["kind"] == "rule"
    } - SETTLED
    for state in ("Conflicting", "Incomplete"):
        if state in states:
            return " " + chip(state, "red" if state == "Conflicting" else "yellow")
    return ""


def item(ctx: Context, entry: Entry) -> str:
    """Render one landing-page link with its summary and any unsettled badge."""
    return (
        f'<a class="item" href="{entry.path}"><b>{esc(entry.title)}'
        f"{unsettled(ctx, entry)}</b><span>{esc(entry.summary)}</span></a>"
    )


def ordered(ctx: Context, tier: str) -> list[Entry]:
    """List a tier's pages with the hub last for Under the hood."""
    entries = ctx.tier(tier)
    return sorted(entries, key=lambda e: e.slug == "hood")


def verdict_chip(verdict: str) -> str:
    """Render a myth verdict label."""
    text, colour = VERDICTS[verdict]
    return chip(text.upper(), colour)


def myth_strip(ctx: Context, count: int) -> str:
    """Render a few myths as one-line verdicts."""
    myths = objects(ctx.load("myths.json")["myths"])[:count]
    rows = "".join(
        f'<div class="card"><div class="meta">'
        f"{verdict_chip(str(m['verdict']))}"
        f' <a href="/original/myths/#{esc(m["id"])}">{esc(m["claim"])}</a></div>'
        f'<p>{esc(m["answer"])}</p></div>'
        for m in myths
    )
    return f'<div class="cards">{rows}</div>'


def build_original(ctx: Context) -> None:
    """Build The original game landing page with its three depths."""
    bands = []
    blurbs = {
        "facts": (
            "Look it up while you play",
            "Tables, numbers and places. Short answers, no formulas.",
        ),
        "how": (
            "Study the rules behind the numbers",
            "Step by step, with exact odds, odd corners, and how sure we are of "
            "each rule.",
        ),
        "hood": (
            "Take it apart",
            "For modders and hackers: file formats and conventions, with links "
            "to the Redux code that reads them.",
        ),
    }
    for tier, chip_text, _ in DEPTHS:
        title, text = blurbs[tier]
        links = "".join(item(ctx, e) for e in ordered(ctx, tier))
        extra = (
            "<p class='muted'>A badge appears only when a fact is not fully "
            "settled.</p>"
            if tier == "facts"
            else ""
        )
        if tier == "how":
            extra = (
                "<p class='muted'><b>ESTABLISHED</b> confirmed · <b>INCOMPLETE</b> "
                "gaps listed · <b>CONFLICTING</b> sources disagree</p>"
            )
        bands.append(
            f'<section class="band" aria-labelledby="{tier}-h">'
            f'<span class="chip yellow">{chip_text}</span>'
            f'<h2 id="{tier}-h">{esc(title)}</h2><p>{esc(text)}</p>'
            f'<nav aria-label="{esc(chip_text.title())}" class="items">{links}</nav>'
            f"{extra}</section>"
        )
    body = (
        "<h1>The original game</h1>"
        "<p class='lede'>Facts about the 1988 game, read from its files and checked "
        "against how it plays. Look something up while you play, dig into exactly how "
        "it works, or take the files apart.</p>"
        + hide_toggle()
        + f'<div class="bands">{"".join(bands)}</div>'
        '<section aria-labelledby="myth-h"><h2 id="myth-h">Myths, tested</h2>'
        "<p>Things players believe, checked against the game's own rules.</p>"
        f"{myth_strip(ctx, 3)}"
        '<p><a href="/original/myths/">All myths</a></p></section>'
    )
    page(
        ctx,
        "original/index.html",
        "The original game",
        body,
        "original",
        "Facts, mechanics and file formats of the 1988 Ultima V.",
    )


def build_home(ctx: Context) -> None:
    """Build the home page with its two doors."""
    depths = []
    for tier, chip_text, audience in DEPTHS:
        entries = ordered(ctx, tier)
        if tier == "hood":
            entries = [e for e in entries if e.slug in HOME_HOOD]
        entries = entries[: HOME_LIMIT[tier]]
        links = "".join(
            f'<li><a href="{e.path}">{esc(e.title)}</a></li>' for e in entries
        )
        depths.append(
            f'<b class="depth">{chip_text} <span>{audience}</span></b><ul>{links}</ul>'
        )
    redux = "".join(
        f'<li><a href="{href}"><b>{esc(title)}</b></a> '
        f'<span class="muted">{esc(detail)}</span></li>'
        for title, detail, href in REDUX_LINKS
    )
    body = (
        '<section class="hero">'
        "<h1>A faithful, modern reimplementation of Ultima V: Warriors of Destiny</h1>"
        "<p class='lede'>Bring your own copy of the original game. This site keeps "
        "two things apart: the facts of the 1988 original, and what Redux changes.</p>"
        '<div class="actions"><a class="button primary" href="/redux/#start-h">'
        'Get started</a><a class="button" href="/original/">'
        "Browse the original game</a></div></section>"
        '<div class="doors">'
        '<section class="door" aria-labelledby="door-original">'
        '<h2 id="door-original">THE ORIGINAL GAME</h2>'
        "<p>Speeds and feeds: the facts that have always been so.</p>"
        f'{"".join(depths)}'
        '<a class="strip" href="/original/myths/"><b>Myths, tested</b>'
        "<span>Things players believe, checked against the game's own rules</span></a>"
        "</section>"
        '<section class="door" aria-labelledby="door-redux">'
        '<h2 id="door-redux">ULTIMA V REDUX</h2>'
        "<p>What is new, and what plays differently.</p>"
        f"<ul>{redux}</ul></section></div>"
    )
    page(
        ctx,
        "index.html",
        "Ultima V Redux",
        body,
        "home",
        "A faithful, modern reimplementation of Ultima V, with the facts of the "
        "original kept apart from what Redux changes.",
    )


def build_data_index(ctx: Context) -> None:
    """Build the human-readable list of published datasets."""
    lines = []
    for name, dataset in ctx.datasets.items():
        rule_ids = ", ".join(str(s["rule"]) for s in objects(dataset["source"]))
        records = objects(dataset["records"])
        lines.append(
            f'<tr><th scope="row"><a href="/data/{name}.json">{name}.json</a></th>'
            f'<td>{esc(dataset["title"])}</td><td class="num">{len(records)}</td>'
            f'<td class="mono">{esc(rule_ids)}</td></tr>'
        )
    body = (
        crumbs(("Home", "/"), ("Data", ""))
        + "<h1>Data</h1><p class='lede'>The tables on this site are published as "
        "JSON, rebuilt from the Game Facts with every release. Use them in your own "
        "tools.</p>"
        '<div class="table-wrap"><table><thead><tr><th scope="col">File</th>'
        '<th scope="col">Contents</th><th scope="col" class="num">Records</th>'
        '<th scope="col">Rules</th></tr></thead>'
        f"<tbody>{''.join(lines)}</tbody></table></div>"
        "<p>The machine-readable list is <a href='/data/index.json'>index.json</a>. "
        "Each dataset has <code>dataset</code>, <code>schema_version</code>, "
        "<code>source</code> (rule IDs and their Original status) and "
        "<code>records</code>. Values worked out from other rules sit in a "
        "<code>derived</code> block, and <code>derived_from</code> names the rule. "
        "Datasets with puzzle answers list them in <code>spoiler_fields</code>.</p>"
        '<p class="muted">Sources: '
        f'<a href="{esc(github("docs/gamefacts/README.md"))}">'
        f'Game Facts</a> on <a href="{REPO_URL}">GitHub</a>. Names and numbers only; '
        "no text, maps, art or code from the original game.</p>"
    )
    page(
        ctx,
        "data/index.html",
        "Data",
        body,
        "original",
        "The JSON datasets behind the tables on this site.",
    )


def build_all(ctx: Context) -> None:
    """Build the landing pages after every page has registered itself."""
    build_original(ctx)
    build_home(ctx)
    build_data_index(ctx)
