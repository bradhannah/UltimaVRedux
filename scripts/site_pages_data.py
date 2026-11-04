"""Data-backed facts pages: Bestiary, Arms and armour, and the Atlas.

Tables are rendered here at build time so they work without JavaScript. The page
script then sorts and filters them using the same records published under /data/.
"""

import json
from collections.abc import Callable

from gamefacts_public import JSONValue
from site_context import (
    Context,
    Entry,
    badge,
    crumbs,
    esc,
    hide_toggle,
    page,
    rule_link,
    spoiler,
)
from site_data import Record, mapping, objects, strings

type Row = dict[str, JSONValue]

MOVEMENT_LABELS = {
    "on_foot": "On foot",
    "water": "Water",
    "flying": "Flying",
    "ether": "Ether",
    "desert": "Desert",
    "deep_water": "Deep water",
    "grass": "Grass",
    "swamp": "Swamp",
}
GROUP_LABELS = {"monster": "Monsters", "person": "People", "field": "Fields"}


def label(key: str) -> str:
    """Turn a snake_case ability key into display text."""
    return key.replace("_", " ").capitalize()


def records(dataset: Record) -> list[Row]:
    """Return a dataset's records with their type narrowed."""
    return objects(dataset["records"])


def derived(row: Row) -> Row:
    """Return a record's derived block."""
    return mapping(row["derived"])


def cell(value: JSONValue) -> str:
    """Format a table value, using a dash for missing data."""
    return "—" if value is None else esc(value)


def header_row(columns: list[tuple[str, str, str]]) -> str:
    """Render a header with a sort key per column: (text, key, css class)."""
    cells = []
    for text, key, css in columns:
        cls = f' class="{css}"' if css else ""
        cells.append(
            f'<th scope="col"{cls} data-sort-key="{esc(key)}">{esc(text)}</th>'
        )
    return f"<thead><tr>{''.join(cells)}</tr></thead>"


def group_buttons(options: list[tuple[str, str]], default: str = "all") -> str:
    """Render the segmented filter used with data-group rows."""
    buttons = "".join(
        f'<button type="button" data-group="{esc(value)}" '
        f'aria-pressed="{"true" if value == default else "false"}">{esc(text)}</button>'
        for value, text in options
    )
    return (
        '<div class="seg" role="group" aria-label="Filter" data-js-only hidden>'
        f"{buttons}</div>"
    )


def tools(options: list[tuple[str, str]], noun: str, default: str = "all") -> str:
    """Render the search box, group filter and result count."""
    return (
        '<div class="tools">'
        f"<label data-js-only hidden>Find {esc(noun)} "
        '<input type="search" data-filter></label>'
        f"{group_buttons(options, default) if options else ''}"
        '<span class="count" data-count aria-live="polite"></span></div>'
    )


def data_panel(name: str, sample: Row, note: str) -> str:
    """Explain that the table is published data and show one real record."""
    sample_json = json.dumps(sample, indent=2, ensure_ascii=False)
    return (
        '<aside class="data-panel" aria-labelledby="data-h">'
        '<h2 id="data-h">This table is data</h2>'
        "<p>The page draws this table from "
        f"<a href='/data/{name}.json'>{name}.json</a>, "
        f"rebuilt from the Game Facts with every release. {esc(note)} "
        "See <a href='/data/'>every dataset</a>.</p>"
        f"<pre><code>{esc(sample_json)}</code></pre></aside>"
    )


def source_badges(ctx: Context, dataset: Record) -> str:
    """Show rule IDs and their Original status for a dataset's sources."""
    items = []
    for item in objects(dataset["source"]):
        identifier = str(item["rule"])
        items.append(
            f"{rule_link(ctx, identifier)} {badge('Original', str(item['original']))}"
        )
    return f'<p class="chips-row">{" ".join(items)}</p>'


def attack_text(row: Row) -> str:
    """Describe how a creature attacks at range, or that it must stand adjacent."""
    if row["missile"] is None:
        return "Melee"
    return f"{str(row['missile']).capitalize()}, range {row['range']}"


def creature_row(row: Row) -> str:
    """Render one Bestiary table row."""
    extra = derived(row)
    abilities = strings(row["abilities"])
    redux = (
        ' <a class="chip yellow" href="/redux/differences/#combat-0009">'
        "Redux differs</a>"
        if "divides" in abilities
        else ""
    )
    movement = extra["movement"]
    moves = MOVEMENT_LABELS[str(movement)] if movement else None
    numbers = [row[k] for k in ("hp",)] + [extra["kill_xp"]]
    stats = [
        row[k]
        for k in ("armour", "damage", "str", "dex", "int", "group_max", "treasure")
    ]
    cells = "".join(f'<td class="num">{cell(v)}</td>' for v in numbers + stats)
    ability_text = ", ".join(label(a) for a in abilities) or "—"
    return (
        f'<tr data-key="{row["index"]}" data-group="{esc(extra["group"])}">'
        f'<th scope="row">{esc(row["name"])}{redux}</th><td>{cell(moves)}</td>'
        f"{cells}<td>{esc(attack_text(row))}</td><td>{esc(ability_text)}</td>"
        f"<td>{esc(row['friend'])}</td></tr>"
    )


def build_bestiary(ctx: Context) -> None:
    """Build the Bestiary page from the creatures dataset."""
    dataset = ctx.datasets["creatures"]
    rows = records(dataset)
    counts = {key: 0 for key in GROUP_LABELS}
    for row in rows:
        counts[str(derived(row)["group"])] += 1
    columns = [
        ("Creature", "name", ""),
        ("Moves", "derived.movement", ""),
        ("HP", "hp", "num"),
        ("XP*", "derived.kill_xp", "num"),
        ("Armour", "armour", "num"),
        ("Damage", "damage", "num"),
        ("STR", "str", "num"),
        ("DEX", "dex", "num"),
        ("INT", "int", "num"),
        ("Group", "group_max", "num"),
        ("Treasure", "treasure", "num"),
        ("Attack", "range", ""),
        ("Abilities", "abilities", ""),
        ("Joined by", "friend", ""),
    ]
    options = [(key, f"{text} {counts[key]}") for key, text in GROUP_LABELS.items()] + [
        ("all", f"All {len(rows)}")
    ]
    body = (
        crumbs(("The original game", "/original/"), ("Bestiary", ""))
        + "<h1>Bestiary</h1>"
        "<p class='lede'>Every entry in the original creature table. The numbers "
        "come straight from the game's data.</p>"
        + source_badges(ctx, dataset)
        + '<div data-table-scope data-default-group="monster">'
        + tools(options, "a creature", "monster")
        + '<div class="table-wrap"><table data-json="/data/creatures.json">'
        + header_row(columns)
        + "<tbody>"
        + "".join(creature_row(row) for row in rows)
        + "</tbody></table></div></div>"
        "<p class='muted'>*XP is worked out from the combat rules, not stored in the "
        "table: HP ÷ 4 + 1, rounded down, to whoever lands the killing blow "
        "(see <a href='/original/levels/'>Levels and experience</a>). Attack shows the "
        "missile and range for creatures that shoot; Melee means they must stand next "
        "to you. Group is the most that appear in one encounter, not how many exist. "
        "A creature in this table is not guaranteed to appear in play. Groups "
        "(monsters, people, fields) are our own grouping by table position.</p>"
        + data_panel(
            "creatures",
            rows[38],
            "Values worked out from other rules sit under 'derived', with the "
            "rule named.",
        )
    )
    page(
        ctx,
        "original/bestiary/index.html",
        "Bestiary",
        body,
        "original",
        "Every creature in the original Ultima V: stats, abilities and XP.",
    )
    ctx.add(
        Entry(
            "facts",
            "bestiary",
            "Bestiary",
            "Every creature: stats, abilities, XP",
            "/original/bestiary/",
            ["ENEMY-0001", "ENEMY-0002", "PROGRESS-0002"],
        )
    )


def build_equipment(ctx: Context) -> None:
    """Build the Arms and armour page from the equipment dataset."""
    dataset = ctx.datasets["equipment"]
    rows = records(dataset)
    slots = sorted({str(row["slot"]) for row in rows})
    columns = [
        ("Item", "name", ""),
        ("Slot", "slot", ""),
        ("Strength", "strength_needed", "num"),
        ("Attack", "attack", "num"),
        ("Defense", "defense", "num"),
        ("Reach", "reach", "num"),
        ("Base price", "base_price", "num"),
    ]
    options = [("all", f"All {len(rows)}")] + [(s, s.capitalize()) for s in slots]
    lines = []
    for row in rows:
        numbers = "".join(
            f'<td class="num">{cell(row[k])}</td>'
            for k in ("strength_needed", "attack", "defense", "reach", "base_price")
        )
        lines.append(
            f'<tr data-key="{row["id"]}" data-group="{esc(row["slot"])}">'
            f'<th scope="row">{esc(row["name"])}</th>'
            f"<td>{esc(str(row['slot']).capitalize())}</td>{numbers}</tr>"
        )
    body = (
        crumbs(("The original game", "/original/"), ("Arms and armour", ""))
        + "<h1>Arms and armour</h1>"
        "<p class='lede'>All 48 pieces of equipment in the original game: where each "
        "is worn, the strength it asks for, and its ratings.</p>"
        + source_badges(ctx, dataset)
        + "<div data-table-scope>"
        + tools(options, "an item")
        + '<div class="table-wrap"><table data-json="/data/equipment.json">'
        + header_row(columns)
        + f"<tbody>{''.join(lines)}</tbody></table></div></div>"
        "<p class='muted'>A dash means the item is not an independently readied "
        "attack, or has no such rating. Attack and Defense are reference ratings, an "
        "input to the game's rules rather than guaranteed damage or protection. Base "
        "price is the merchant's formula input, not a quoted price or a promise of "
        "stock. Reach 0 means adjacent attacks, not a zero-distance attack.</p>"
        + data_panel(
            "equipment",
            rows[3],
            "Every field is copied from the Game Facts table.",
        )
    )
    page(
        ctx,
        "original/equipment/index.html",
        "Arms and armour",
        body,
        "original",
        "Ratings, strength needed and base prices for all 48 pieces of equipment.",
    )
    ctx.add(
        Entry(
            "facts",
            "equipment",
            "Arms and armour",
            "Ratings, strength needed, prices",
            "/original/equipment/",
            ["EQUIP-0001"],
        )
    )


def coordinates(row: Row) -> str:
    """Format a row's X,Y, or a dash when it has no position."""
    return "—" if row["x"] is None else f"{row['x']},{row['y']}"


def plot(ctx: Context) -> str:
    """Draw gates, shrines and dungeon entrances at their coordinates. No map art."""
    parts = [
        '<svg class="plot" viewBox="-8 -8 272 272" role="img" '
        'aria-labelledby="plot-t plot-d">'
        '<title id="plot-t">Plot of moongates, shrines and dungeon entrances</title>'
        '<desc id="plot-d">Positions plotted from coordinates on a 256 by 256 grid, '
        "with no map art. The tables on this page list the same positions.</desc>"
        '<rect x="0" y="0" width="256" height="256" fill="#000" stroke="#5555ff"/>'
    ]
    for line in range(0, 257, 32):
        parts.append(
            f'<path d="M{line} 0V256M0 {line}H256" '
            'stroke="#2a2a6a" stroke-width="0.5"/>'
        )
        parts.append(
            f'<text x="{line}" y="-2" font-size="7" fill="#aaa" '
            f'text-anchor="middle" font-family="monospace">{line}</text>'
        )
    for row in records(ctx.datasets["locations"]):
        if row["kind"] == "dungeon":
            x, y = int(str(row["x"])), int(str(row["y"]))
            parts.append(
                f'<rect x="{x - 2.5}" y="{y - 2.5}" width="5" height="5" '
                'fill="#ff5555"><title>'
                f'{esc(row["name"])} {x},{y}</title></rect>'
            )
    for row in records(ctx.datasets["shrines"]):
        if row["x"] is not None:
            x, y = int(str(row["x"])), int(str(row["y"]))
            parts.append(
                f'<rect x="{x - 2.5}" y="{y - 2.5}" width="5" height="5" '
                f'fill="#55ff55"><title>{esc(row["virtue"])} {x},{y}</title></rect>'
            )
    for row in records(ctx.datasets["moonstones"]):
        x, y = int(str(row["x"])), int(str(row["y"]))
        parts.append(
            f'<circle cx="{x}" cy="{y}" r="3" fill="#ffff55"><title>'
            f'Moongate {row["index"]} {x},{y}</title></circle>'
            f'<text x="{x + 5}" y="{y - 4}" font-size="8" fill="#ffff55" '
            f'font-family="monospace">{row["index"]}</text>'
        )
    parts.append("</svg>")
    legend = (
        '<p class="legend"><span style="color:#ffff55">● Moongate</span>'
        '<span style="color:#55ff55">■ Shrine</span>'
        '<span style="color:#ff5555">■ Dungeon entrance</span></p>'
    )
    return "".join(parts) + legend


def moongate_table(ctx: Context) -> str:
    """List each moonstone's starting position and Sextant reading."""
    lines = "".join(
        f'<tr><th scope="row" class="num">{row["index"]}</th>'
        f"<td class='mono'>{coordinates(row)}</td>"
        f"<td class='mono'>{esc(derived(row)['sextant'])}</td></tr>"
        for row in records(ctx.datasets["moonstones"])
    )
    return (
        '<div class="table-wrap"><table><thead><tr><th scope="col">Stone</th>'
        '<th scope="col">X,Y</th><th scope="col">Sextant</th></tr></thead>'
        f"<tbody>{lines}</tbody></table></div>"
    )


def calendar(ctx: Context) -> str:
    """Lay out the 28-day Trammel and Felucca schedule in two 14-day rows."""
    days = records(ctx.datasets["moon_phases"])
    blocks = []
    for start in (0, 14):
        part = days[start : start + 14]
        head = "".join(f'<th scope="col">{row["day"]}</th>' for row in part)
        trammel = "".join(f'<td class="t">{row["trammel"]}</td>' for row in part)
        felucca = "".join(f'<td class="f">{row["felucca"]}</td>' for row in part)
        blocks.append(
            '<div class="table-wrap"><table class="moon-grid">'
            f"<caption class='muted'>Days {start + 1} to {start + 14}</caption>"
            f'<thead><tr><th scope="col">Day</th>{head}</tr></thead><tbody>'
            f'<tr><th scope="row">Trammel</th>{trammel}</tr>'
            f'<tr><th scope="row">Felucca</th>{felucca}</tr></tbody></table></div>'
        )
    return "".join(blocks)


def shrine_table(ctx: Context) -> str:
    """List shrines with their mantras behind reveal buttons."""
    lines = []
    for row in records(ctx.datasets["shrines"]):
        lines.append(
            f'<tr><th scope="row">{esc(row["virtue"])}</th>'
            f"<td>{spoiler(str(row['mantra']))}</td>"
            f"<td class='mono'>{coordinates(row)}</td>"
            f"<td class='mono'>{cell(derived(row)['sextant'])}</td></tr>"
        )
    return (
        '<div class="table-wrap"><table><thead><tr><th scope="col">Virtue</th>'
        '<th scope="col">Mantra</th><th scope="col">X,Y</th>'
        '<th scope="col">Sextant</th></tr></thead>'
        f"<tbody>{''.join(lines)}</tbody></table></div>"
    )


def dungeon_table(ctx: Context) -> str:
    """List dungeon entrances with their Words of Power behind reveal buttons."""
    lines = []
    for row in records(ctx.datasets["locations"]):
        if row["kind"] != "dungeon":
            continue
        x, y = int(str(row["x"])), int(str(row["y"]))
        where = " and ".join(w.capitalize() for w in strings(row["worlds"]))
        lines.append(
            f'<tr><th scope="row">{esc(row["name"])}</th>'
            f"<td>{spoiler(str(row['word_of_power']))}</td>"
            f"<td class='mono'>{x},{y}</td><td>{esc(where)}</td></tr>"
        )
    return (
        '<div class="table-wrap"><table><thead><tr><th scope="col">Dungeon</th>'
        '<th scope="col">Word of Power</th><th scope="col">X,Y</th>'
        '<th scope="col">Entrance in</th></tr></thead>'
        f"<tbody>{''.join(lines)}</tbody></table></div>"
    )


def quest_table(ctx: Context) -> str:
    """List the four fixed Underworld quest items with positions hidden."""
    lines = "".join(
        f'<tr><th scope="row">{esc(row["name"])}</th>'
        f"<td>{spoiler(f'{row['x']},{row['y']}')}</td></tr>"
        for row in records(ctx.datasets["quest_items"])
    )
    return (
        '<div class="table-wrap"><table><thead><tr><th scope="col">Item</th>'
        '<th scope="col">Underworld X,Y</th></tr></thead>'
        f"<tbody>{lines}</tbody></table></div>"
    )


def section(ctx: Context, anchor: str, title: str, rule: str, content: str) -> str:
    """Wrap an atlas section with its heading and the rule's Original badge."""
    return (
        f'<section aria-labelledby="{anchor}-h"><h2 id="{anchor}-h">{esc(title)} '
        f"{badge('Original', ctx.status(rule))}</h2>{content}</section>"
    )


def build_atlas(ctx: Context) -> None:
    """Build the Atlas page: positions, phases, shrines, words and quest items."""
    note = (
        "<p class='muted'>X,Y is the map square; Sextant is the reading you get on the "
        "surface at night. Positions are tile coordinates, counted from the top left "
        "of the 256 × 256 world.</p>"
    )
    gates = (
        "<p>Gates open where the eight moonstones lie buried. These are the "
        "starting spots; you can dig a stone up and bury it elsewhere.</p>"
        + moongate_table(ctx)
        + rule_link(ctx, "MOON-0001")
    )
    phases = (
        "<p>Before noon, Trammel's phase picks the stone you arrive at; from noon on, "
        "Felucca's does. In the first ten minutes after midnight a gate leads toward "
        "the shrines instead. Phases repeat every 28 days.</p>"
        + calendar(ctx)
        + rule_link(ctx, "MOON-0002")
        + " "
        + rule_link(ctx, "MOON-0005")
    )
    spirit = (
        "<p class='muted'>Spirituality has no position in the table (0,0 is a "
        "placeholder); see the midnight rule under Moongates.</p>"
    )
    shrines = shrine_table(ctx) + spirit + rule_link(ctx, "SHRINE-0001")
    dungeons = (
        "<p>Yell the word outdoors to open or seal a dungeon. "
        "It works from either world where that dungeon has an entrance.</p>"
        + dungeon_table(ctx)
        + rule_link(ctx, "WORD-0001")
    )
    quest = (
        "<p>These lie in the Underworld, where the Sextant does not work.</p>"
        + quest_table(ctx)
        + "<p class='muted'>Incomplete: how the other quest items are placed is "
        "still being worked out.</p>" + rule_link(ctx, "QUEST-0002")
    )
    body = (
        crumbs(("The original game", "/original/"), ("Atlas", ""))
        + "<h1>Atlas</h1><p class='lede'>Where things are.</p>"
        + hide_toggle()
        + note
        + section(ctx, "moongates", "Moongates", "MOON-0001", gates)
        + plot(ctx)
        + section(ctx, "phases", "Which gate takes you where", "MOON-0002", phases)
        + section(ctx, "shrines", "Shrines", "SHRINE-0001", shrines)
        + section(ctx, "dungeons", "Dungeon entrances", "WORD-0001", dungeons)
        + section(ctx, "quest-items", "Quest items below ground", "QUEST-0002", quest)
    )
    page(
        ctx,
        "original/atlas/index.html",
        "Atlas",
        body,
        "original",
        "Moongates, shrines, dungeon entrances and quest items, by coordinates.",
    )
    ctx.add(
        Entry(
            "facts",
            "atlas",
            "Atlas",
            "Moongates, shrines, quest items, hidden finds",
            "/original/atlas/",
            ["MOON-0001", "MOON-0002", "SHRINE-0001", "WORD-0001", "QUEST-0002"],
        )
    )


PAGE_BUILDERS: list[Callable[[Context], None]] = [
    build_bestiary,
    build_equipment,
    build_atlas,
]
