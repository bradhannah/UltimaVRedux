"""Curated pages: facts and how-it-works, Myths, and the Redux section.

Prose lives in `site/content/`. This module validates every curated rule reference
against the Game Facts so stale content fails the build instead of shipping.
"""

import re
from collections.abc import Callable
from typing import cast

import site_combat
from gamefacts_public import JSONValue, require
from site_context import (
    REPO_URL,
    Context,
    Entry,
    badge,
    chip,
    crumbs,
    esc,
    github,
    page,
    rule_link,
    sources_footer,
)
from site_data import mapping, objects, strings, tables
from site_markdown import inline

type Obj = dict[str, JSONValue]

VERDICTS = {
    "busted": ("Busted", "busted"),
    "confirmed": ("Confirmed", "confirmed"),
    "depends": ("It depends", "depends"),
    "unknown": ("Unknown", "unknown"),
}
TAG_COLOURS = {"original": "green", "helper": "link", "look": ""}
TIER_DIRS = {"facts": "original", "how": "original/how"}
OPTIONS_FILE = "cmd/ultimav/audio_settings_dialog_rendering.go"
TEASER_RULES = ("COMBAT-0009", "DNG-0009", "ASTRO-0006")
TEASER_OPTIONS = (
    "Repeated Monster Division",
    "Show Subdued Unlit Dungeons",
    "Flexible Sextant Use",
    "Batch Merchant Theft",
    "Smooth Surface Movement",
    "Combat Speed",
)


def plain(text: str) -> str:
    """Render inline Markdown from curated content. Site paths pass through."""
    return inline(text, lambda target: target)


def level_for(experience: int) -> int:
    """PROGRESS-0001: 1 plus the bit length of XP ÷ 100."""
    return 1 + (experience // 100).bit_length()


def level_table() -> str:
    """Compute the XP needed for levels 1-8 and check them against the formula."""
    lines = []
    for level in range(1, 9):
        needed = 0 if level == 1 else 100 << (level - 2)
        require(level_for(needed) == level, f"level {level} threshold is wrong")
        require(level == 1 or level_for(needed - 1) == level - 1, "bad boundary")
        hit_points = "—" if level == 1 else str(30 * level)
        lines.append(
            f'<tr><th scope="row" class="num">{level}</th>'
            f'<td class="num">{needed:,}</td><td class="num">{hit_points}</td></tr>'
        )
    return (
        '<div class="table-wrap"><table><thead><tr><th scope="col" class="num">'
        'Level</th><th scope="col" class="num">XP needed</th>'
        '<th scope="col" class="num">HP set on reaching it</th></tr></thead>'
        f"<tbody>{''.join(lines)}</tbody></table></div>"
    )


def ambush_table(ctx: Context) -> str:
    """Show CAMP-0002's selection table as 'N in 8' for outdoors and dungeons."""
    header, rows = tables(ctx.rules["CAMP-0002"]["text"])[0]
    require(len(header) == 3, "CAMP-0002 table shape changed")

    def odds(value: str) -> str:
        return f"{value} in 8" if int(value) else '<span class="muted">—</span>'

    lines = "".join(
        f'<tr><th scope="row">{esc(row[0])}</th><td>{odds(row[1])}</td>'
        f"<td>{odds(row[2])}</td></tr>"
        for row in rows
    )
    return (
        '<div class="table-wrap"><table><thead><tr><th scope="col">Creature</th>'
        '<th scope="col">Outdoors</th><th scope="col">In a dungeon</th></tr></thead>'
        f"<tbody>{lines}</tbody></table></div>"
    )


def myth_card(ctx: Context, myth: Obj, compact: bool = False) -> str:
    """Render one myth as a card with its verdict, answer and rule links."""
    text, css = VERDICTS[str(myth["verdict"])]
    rules = " ".join(rule_link(ctx, r) for r in strings(myth["rules"]))
    more = ""
    if not compact:
        more = f' <a href="{esc(myth["link"])}">{esc(myth["link_label"])}</a>'
    return (
        f'<article class="card" id="{esc(myth["id"])}" '
        f'data-verdict="{esc(myth["verdict"])}">'
        f'<div class="meta">{chip(text.upper(), css)} {rules}{more}</div>'
        f'<h3>{esc(myth["claim"])}</h3><p>{esc(myth["answer"])}</p></article>'
    )


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


def block_h2(block: Obj) -> str:
    """Render a heading, with an anchor when the block names one."""
    anchor = f' id="{esc(block["id"])}"' if "id" in block else ""
    return f"<h2{anchor}>{esc(block['text'])}</h2>"


def block_steps(block: Obj) -> str:
    """Render numbered steps."""
    items = "".join(
        f"<li><b>{esc(i['title'])}</b> {plain(str(i['text']))}</li>"
        for i in objects(block["items"])
    )
    return f'<ol class="steps">{items}</ol>'


def block_stats(block: Obj) -> str:
    """Render a row of headline figures."""
    items = "".join(
        f'<div class="stat"><div class="label">{esc(i["label"])}</div>'
        f'<div class="value">{esc(i["value"])}</div>'
        f'<div class="note">{esc(i["note"])}</div></div>'
        for i in objects(block["items"])
    )
    return f'<div class="stats">{items}</div>'


def block_callout(block: Obj) -> str:
    """Render a highlighted note."""
    return (
        f'<div class="callout"><span class="label">{esc(block["label"])}</span>'
        f"<p>{plain(str(block['text']))}</p></div>"
    )


def block_myths(ctx: Context, block: Obj) -> str:
    """Render the named myths as compact cards."""
    myths = {str(m["id"]): m for m in objects(ctx.load("myths.json")["myths"])}
    cards = "".join(
        myth_card(ctx, myths[i], compact=True) for i in strings(block["ids"])
    )
    return f'<div class="cards">{cards}</div>'


def render_block(ctx: Context, block: Obj) -> str:
    """Render one curated content block."""
    simple: dict[str, Callable[[Obj], str]] = {
        "p": lambda b: f"<p>{plain(str(b['text']))}</p>",
        "h2": block_h2,
        "ul": lambda b: "<ul>"
        + "".join(f"<li>{plain(t)}</li>" for t in strings(b["items"]))
        + "</ul>",
        "steps": block_steps,
        "stats": block_stats,
        "table": lambda b: table(strings(b["head"]), rows_of(b["rows"])),
        "callout": block_callout,
        "myths": lambda b: block_myths(ctx, b),
        "computed": lambda b: computed_block(ctx, b),
    }
    kind = str(block["type"])
    require(kind in simple, f"unknown block type: {kind}")
    return simple[kind](block)


def computed_block(ctx: Context, block: Obj) -> str:
    """Render a table computed from Game Facts at build time."""
    computed: dict[str, Callable[[], str]] = {
        "level_table": level_table,
        "ambush_table": lambda: ambush_table(ctx),
        "hit_table": lambda: site_combat.hit_table(ctx),
        "fight_example": lambda: site_combat.fight_example(ctx, block),
        "chest_contents": lambda: site_combat.chest_contents(ctx),
        "chest_arms": lambda: site_combat.chest_arms(ctx),
    }
    name = str(block["name"])
    require(name in computed, f"unknown computed block: {name}")
    return computed[name]()


def build_curated(ctx: Context) -> None:
    """Build every page described in site/content/pages/."""
    for path in sorted((ctx.content / "pages").glob("*.json")):
        data = ctx.load(f"pages/{path.name}")
        tier, name = str(data["tier"]), str(data["slug"])
        folder = f"{TIER_DIRS[tier]}/{name}"
        rules = strings(data["rules"])
        for identifier in rules:
            require(identifier in ctx.rules, f"{path.name}: unknown rule {identifier}")
        body = (
            crumbs(("The original game", "/original/"), (str(data["title"]), ""))
            + f"<h1>{esc(data['title'])}</h1>"
            + f'<p class="lede">{plain(str(data["lede"]))}</p>'
            + "".join(render_block(ctx, b) for b in objects(data["blocks"]))
            + sources_footer(ctx, rules)
        )
        page(
            ctx,
            f"{folder}/index.html",
            str(data["title"]),
            body,
            "original",
            str(data["summary"]),
        )
        ctx.add(
            Entry(
                tier,
                name,
                str(data["title"]),
                str(data["summary"]),
                f"/{folder}/",
                rules,
            )
        )


def check_myths(ctx: Context, myths: list[Obj]) -> None:
    """Fail the build if a myth is settled by anything but an Established rule."""
    seen: set[str] = set()
    for myth in myths:
        require(str(myth["id"]) not in seen, f"duplicate myth id: {myth['id']}")
        seen.add(str(myth["id"]))
        require(str(myth["verdict"]) in VERDICTS, f"bad verdict: {myth['id']}")
        for identifier in strings(myth["rules"]):
            require(identifier in ctx.rules, f"myth {myth['id']}: no rule {identifier}")
            require(
                ctx.status(identifier) == "Established",
                f"myth {myth['id']}: {identifier} is not Established",
            )


def build_myths(ctx: Context) -> None:
    """Build the Myths index page."""
    myths = objects(ctx.load("myths.json")["myths"])
    check_myths(ctx, myths)
    buttons = [("all", "All")] + [(k, v[0]) for k, v in VERDICTS.items()]
    present = {str(m["verdict"]) for m in myths}
    filters = "".join(
        f'<button type="button" data-verdict-filter="{key}" '
        f'aria-pressed="{"true" if key == "all" else "false"}">{esc(text)}</button>'
        for key, text in buttons
        if key == "all" or key in present
    )
    body = (
        crumbs(("The original game", "/original/"), ("Myths, tested", ""))
        + "<h1>Myths, tested</h1>"
        "<p class='lede'>Things players believe, checked against the game's own "
        "rules. Each verdict rests on a rule the Game Facts mark as Established.</p>"
        f'<div class="seg" role="group" aria-label="Filter by verdict" data-js-only '
        f"hidden>{filters}</div>"
        f'<div class="cards">{"".join(myth_card(ctx, m) for m in myths)}</div>'
    )
    page(
        ctx,
        "original/myths/index.html",
        "Myths, tested",
        body,
        "myths",
        "Things players believe about Ultima V, checked against the game's own rules.",
    )


def check_differences(ctx: Context, differences: list[Obj]) -> None:
    """Require every difference to cite a rule that still records a deviation."""
    for item in differences:
        identifier = str(item["rule"])
        require(identifier in ctx.rules, f"difference cites unknown {identifier}")
        text = ctx.rules[identifier]["text"]
        require(
            ctx.status(identifier, "Redux") == "Intentional deviation"
            or f"REDUX DEVIATION: {identifier}" in text,
            f"{identifier} no longer records a Redux deviation",
        )


def difference_card(ctx: Context, item: Obj, kinds: Obj) -> str:
    """Render one difference with the original and Redux behaviour side by side."""
    identifier = str(item["rule"])
    kind = str(item["kind"])
    where = ""
    if item.get("setting"):
        where = f"<span>Switch in Configure: <b>{esc(item['setting'])}</b></span>"
    note = f"<span>{esc(item['note'])}</span>" if item.get("note") else ""
    colour = "green" if kind == "switchable" else "yellow"
    original = ctx.status(identifier)
    flag = badge("Original", original) if original != "Established" else ""
    return (
        f'<article class="card" id="{identifier.lower()}" data-kind="{esc(kind)}">'
        f'<div class="meta">{esc(str(item["area"]).upper())} '
        f"{chip(str(kinds[kind]).upper(), colour)}"
        f" {rule_link(ctx, identifier)} {flag}</div>"
        f'<h3>{esc(item["title"])}</h3><div class="versus">'
        f'<div><b>Original</b>{esc(item["original"])}</div>'
        f'<div><b>Redux</b>{esc(item["redux"])}</div></div>'
        f'<div class="meta">{where}{note}</div></article>'
    )


def build_differences(ctx: Context) -> None:
    """Build the full list of differences from the original."""
    data = ctx.load("differences.json")
    differences = objects(data["differences"])
    kinds = mapping(data["kinds"])
    check_differences(ctx, differences)
    body = (
        crumbs(("Ultima V Redux", "/redux/"), ("Differences you'll notice", ""))
        + "<h1>Differences you'll notice</h1>"
        "<p class='lede'>Each change, the original next to Redux. Switchable ones say "
        "where to turn the original rule back on.</p>"
        f'<div class="cards">'
        f'{"".join(difference_card(ctx, d, kinds) for d in differences)}</div>'
        "<p class='muted'>This list is curated. The full record, including smaller "
        "deviations, is in the Game Facts: look for REDUX DEVIATION in "
        f'<a href="{github("docs/gamefacts/README.md")}">the topic list</a>.</p>'
    )
    page(
        ctx,
        "redux/differences/index.html",
        "Differences you'll notice",
        body,
        "redux",
        "Where Ultima V Redux plays differently from the 1988 original.",
    )


def check_gaps(ctx: Context, gaps: list[Obj]) -> None:
    """Require curated gaps to match the Game Facts' Known discrepancy rules."""
    curated = {str(g["rule"]) for g in gaps}
    for identifier in curated:
        require(identifier in ctx.rules, f"known gap cites unknown {identifier}")
        require(
            ctx.status(identifier, "Redux") == "Known discrepancy",
            f"{identifier} is no longer a Known discrepancy; update known_gaps.json",
        )
    actual = {
        identifier
        for identifier, rule in ctx.rules.items()
        if rule["kind"] == "rule"
        and ctx.status(identifier, "Redux") == "Known discrepancy"
    }
    require(
        actual <= curated,
        "Known discrepancy rules missing from known_gaps.json: "
        f"{sorted(actual - curated)}",
    )


def check_options(ctx: Context, options: list[Obj]) -> None:
    """Require each documented option label to exist in the Configure dialog."""
    source = (ctx.root / OPTIONS_FILE).read_text(encoding="utf-8")
    for option in options:
        pattern = r'label:\s+"' + re.escape(str(option["label"])) + '"'
        require(
            re.search(pattern, source) is not None,
            f"option '{option['label']}' is not in {OPTIONS_FILE}",
        )


def option_item(option: Obj, tags: Obj, full: bool) -> str:
    """Render one option, briefly or with its explanation."""
    tag = chip(str(tags[str(option["tag"])]).upper(), TAG_COLOURS[str(option["tag"])])
    detail = f"<p>{esc(option['effect'])}</p>" if full else ""
    name = f'<span class="mono"><b>{esc(option["label"])}</b></span>'
    return f"<li>{name} {tag}{detail}</li>"


def build_options(ctx: Context) -> None:
    """Build the page listing every gameplay option and what it changes."""
    data = ctx.load("redux.json")
    options = objects(data["options"])
    check_options(ctx, options)
    tags = mapping(data["option_tags"])
    items = "".join(option_item(o, tags, True) for o in options)
    body = (
        crumbs(("Ultima V Redux", "/redux/"), ("Options", "")) + "<h1>Options</h1>"
        "<p class='lede'>Gameplay settings in Configure on the title menu. Some bring "
        "back the original rule.</p>"
        f'<ul class="option-list">{items}</ul>'
        "<p class='muted'>Wording follows the in-game descriptions. Configure also "
        "has Audio and Display sections.</p>"
    )
    page(
        ctx,
        "redux/options/index.html",
        "Options",
        body,
        "redux",
        "Every gameplay option in Ultima V Redux and what it changes.",
    )


def start_steps(steps: list[Obj]) -> str:
    """Render the numbered get-started steps."""
    items = "".join(
        f"<li><b>{esc(s['title'])}</b> {esc(s['text'])} "
        f'<a href="{esc(s["link"])}">{esc(s["link_label"])}</a></li>'
        for s in steps
    )
    return f'<ol class="steps">{items}</ol>'


def build_redux(ctx: Context) -> None:
    """Build the Redux landing page."""
    data = ctx.load("redux.json")
    steps = objects(data["steps"])
    options = {str(o["label"]): o for o in objects(data["options"])}
    tags = mapping(data["option_tags"])
    differences = {
        str(d["rule"]): d for d in objects(ctx.load("differences.json")["differences"])
    }
    gaps = objects(ctx.load("known_gaps.json")["gaps"])
    check_gaps(ctx, gaps)
    teaser = "".join(
        f'<article class="card"><h3>{esc(differences[r]["title"])}</h3>'
        '<div class="versus">'
        f'<div><b>Original</b>{esc(differences[r]["original"])}</div>'
        f'<div><b>Redux</b>{esc(differences[r]["redux"])}</div></div></article>'
        for r in TEASER_RULES
    )
    option_list = "".join(
        option_item(options[label], tags, False) for label in TEASER_OPTIONS
    )
    gap_list = "".join(
        f"<li>{esc(g['summary'])} {rule_link(ctx, str(g['rule']))}</li>"
        for g in gaps
        if g["listed"]
    )
    quiet = sum(1 for g in gaps if not g["listed"])
    developer_note = (
        f"<p class='muted'>{quiet} more affect{'s' if quiet == 1 else ''} only "
        "rare cases and stay in the developer notes.</p>"
        if quiet
        else ""
    )
    credit_text = "".join(
        f"<p><b>{esc(c['title'])}</b> {esc(c['text'])}</p>"
        for c in objects(data["credits"])
    )
    body = (
        "<h1>Ultima V Redux</h1>"
        "<p class='lede'>How to start playing, and every place Redux plays "
        "differently from the 1988 original. Free, non-commercial, and in public "
        "preview.</p>"
        '<section aria-labelledby="start-h"><h2 id="start-h">Get started</h2>'
        f"<p class='muted'>Unsigned preview builds. "
        f'<a href="{esc(data["platform_status"])}">Platform status</a> says what has '
        "been checked on each system.</p>"
        f"{start_steps(steps)}</section>"
        '<section aria-labelledby="diff-h"><h2 id="diff-h">Differences you\'ll notice'
        f'</h2><div class="cards">{teaser}</div>'
        '<p><a href="/redux/differences/">All differences</a></p></section>'
        '<section aria-labelledby="opt-h"><h2 id="opt-h">Options</h2>'
        "<p>Gameplay settings in Configure. Some bring back the original rule.</p>"
        f'<ul class="option-list">{option_list}</ul>'
        '<p><a href="/redux/options/">Every option, what it changes</a></p></section>'
        '<section aria-labelledby="gap-h"><h2 id="gap-h">Known gaps</h2>'
        "<p>Where Redux is not faithful yet. These are bugs to fix, not choices.</p>"
        f"<ul>{gap_list}</ul>"
        f"{developer_note}</section>"
        '<section aria-labelledby="credits"><h2 id="credits">Credits and licences</h2>'
        f"{credit_text}"
        '<div class="actions">'
        f'<a class="button" href="{github("ASSET_CREDITS.md")}">Asset credits</a>'
        f'<a class="button" href="{REPO_URL}/issues/new/choose">Report a bug</a>'
        f'<a class="button" href="{REPO_URL}">Source on GitHub</a></div></section>'
    )
    page(
        ctx,
        "redux/index.html",
        "Ultima V Redux",
        body,
        "redux",
        "Download Ultima V Redux and see what plays differently.",
    )


def build_all(ctx: Context) -> None:
    """Build every curated page."""
    build_curated(ctx)
    build_myths(ctx)
    build_differences(ctx)
    build_options(ctx)
    build_redux(ctx)
