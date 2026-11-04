"""Facts pages for play: Spells and reagents, Shops and prices, Commands and time."""

from gamefacts_public import JSONValue
from site_context import Context, Entry, crumbs, esc, page, sources_footer
from site_data import Record, mapping, objects, strings, tables
from site_data_play import SHOP_RULES, adjusted_buy, find_table
from site_pages_data import (
    cell,
    data_panel,
    derived,
    header_row,
    records,
    source_badges,
    tools,
)

type Row = dict[str, JSONValue]

INTELLIGENCES = ("10", "20", "30")
CONTEXT_LABELS = {
    "outdoors": "Outdoors",
    "town": "Town",
    "dungeon": "Dungeon",
    "combat": "Combat",
}


def table_html(head: list[str], lines: list[str], css: str = "") -> str:
    """Wrap pre-rendered rows in an accessible table."""
    cols = "".join(f'<th scope="col">{esc(h)}</th>' for h in head)
    return (
        f'<div class="table-wrap"><table{css}><thead><tr>{cols}</tr></thead>'
        f"<tbody>{''.join(lines)}</tbody></table></div>"
    )


def spell_row(row: Row) -> str:
    """Render one spell row."""
    where = ", ".join(CONTEXT_LABELS[c] for c in strings(row["contexts"]))
    return (
        f'<tr data-key="{row["index"]}" data-group="{row["circle"]}">'
        f'<td class="num">{row["circle"]}</td><th scope="row">{esc(row["name"])}</th>'
        f"<td>{esc(', '.join(strings(row['reagents'])))}</td><td>{esc(where)}</td>"
        f"<td>{esc(row['effect'])}</td></tr>"
    )


def reagent_table(ctx: Context) -> str:
    """List each reagent with the shops that sell it."""
    lines = []
    for row in records(ctx.datasets["reagents"]):
        sold = objects(derived(row)["sold_at"])
        where = "; ".join(
            f"{s['location']}, {s['shop']}: {s['base_price']} gold for {s['units']}"
            for s in sold
        )
        lines.append(
            f'<tr><th scope="row">{esc(row["name"])}</th>'
            f"<td class='mono'>{esc(row['abbreviation'])}</td>"
            f"<td>{esc(where) if where else '—'}</td></tr>"
        )
    return table_html(["Reagent", "Short", "Sold at (base price for a bundle)"], lines)


def build_spells(ctx: Context) -> None:
    """Build the Spells and reagents page."""
    dataset = ctx.datasets["spells"]
    rows = records(dataset)
    columns = [
        ("Circle", "circle", "num"),
        ("Spell", "name", ""),
        ("Reagents", "reagents", ""),
        ("Where", "contexts", ""),
        ("What it does", "effect", ""),
    ]
    options = [(str(c), f"Circle {c}") for c in range(1, 9)] + [
        ("all", f"All {len(rows)}")
    ]
    body = (
        crumbs(("The original game", "/original/"), ("Spells and reagents", ""))
        + "<h1>Spells and reagents</h1>"
        "<p class='lede'>All 48 spells: the reagents each one needs, where it can "
        "be cast, and what it does in the original.</p>"
        + source_badges(ctx, dataset)
        + "<p>Mix a spell with the M command, then cast it with C. A spell costs "
        "MP equal to its circle. Each ingredient you pick uses one unit per mixture. "
        "Order does not matter, but a missing or extra ingredient makes a wrong "
        "recipe. Prepared stock tops out at 99.</p>"
        "<p class='muted'>Where: you can cast it outdoors, in town, in a dungeon or "
        "in combat. Being able to cast a spell somewhere does not promise the effect "
        "works there. The effects are quoted from the Game Facts table and are "
        "technical: D30 and R(a,b) stand for random rolls.</p>"
        + '<div data-table-scope data-default-group="all">'
        + tools(options, "a spell", "all")
        + '<div class="table-wrap"><table data-json="/data/spells.json">'
        + header_row(columns)
        + f"<tbody>{''.join(spell_row(r) for r in rows)}</tbody></table></div></div>"
        + "<h2>Reagents</h2>"
        + reagent_table(ctx)
        + "<p class='muted'>Base prices are before your Intelligence adjusts them; "
        "see <a href='/original/shops/#reagents'>Shops and prices</a>.</p>"
        + data_panel("spells", rows[4], "Reagents come from MIX-0001's recipe table.")
        + sources_footer(ctx, ["SPELL-0002", "SPELL-0001", "MIX-0001", "MIX-0002"])
    )
    page(
        ctx,
        "original/spells/index.html",
        "Spells and reagents",
        body,
        "original",
        "All 48 spells, their reagents and where to buy reagents.",
    )
    ctx.add(
        Entry(
            "facts",
            "spells",
            "Spells and reagents",
            "All 48 spells and what each needs",
            "/original/spells/",
            ["SPELL-0002", "MIX-0001", "MIX-0002"],
        )
    )


def price_cells(base: int) -> str:
    """Render a base price and what buyers of Intelligence 10, 20 and 30 pay."""
    cells = "".join(
        f'<td class="num">{adjusted_buy(base, int(i))}</td>' for i in INTELLIGENCES
    )
    return f'<td class="num">{base}</td>{cells}'


PRICE_HEAD = ["Base", "INT 10", "INT 20", "INT 30"]


def details(row: Row) -> Record:
    """Return a shop's family-specific details."""
    return mapping(row["details"])


def shops_of(ctx: Context, family: str) -> list[Row]:
    """List the shops of one family in table order."""
    return [r for r in records(ctx.datasets["shops"]) if r["family"] == family]


def name_cells(row: Row) -> str:
    """Render the town and the shop with its owner."""
    return (
        f'<th scope="row">{esc(row["location"])}</th>'
        f'<td>{esc(row["shop"])} <span class="muted">({esc(row["owner"])})</span></td>'
    )


def arms_section(ctx: Context) -> str:
    """Arms shops with each item's base price."""
    lines = []
    for row in shops_of(ctx, "arms"):
        stock = ", ".join(
            f"{s['item']} <span class='muted'>{s['base_price']}</span>"
            for s in objects(details(row)["stock"])
        )
        lines.append(f"<tr>{name_cells(row)}<td>{stock}</td></tr>")
    return (
        "<p>Each item shows its base price in gold. Arrows and Quarrels refill "
        "your stack to 99 for a single price. Selling pays a share of the base "
        "price that grows with your Intelligence.</p>"
        + table_html(["Town", "Shop", "Stock and base price"], lines)
    )


def reagent_section(ctx: Context) -> str:
    """Reagent bundles: base gold for a number of units."""
    lines = []
    for row in shops_of(ctx, "reagents"):
        for bundle in objects(details(row)["bundles"]):
            lines.append(
                f"<tr>{name_cells(row)}<td>{esc(bundle['reagent'])}</td>"
                f"<td class='num'>{bundle['units']}</td>"
                f"{price_cells(int(str(bundle['base_price'])))}</tr>"
            )
    return table_html(["Town", "Shop", "Reagent", "Units"] + PRICE_HEAD, lines)


def guild_section(ctx: Context) -> str:
    """Guild bundles of keys, gems and torches."""
    lines = []
    for row in shops_of(ctx, "guild"):
        for offer in objects(details(row)["offers"]):
            lines.append(
                f"<tr>{name_cells(row)}<td>{esc(offer['item'])}</td>"
                f"<td class='num'>{offer['units']}</td>"
                f"{price_cells(int(str(offer['base_price'])))}</tr>"
            )
    return table_html(["Town", "Shop", "Item", "Units"] + PRICE_HEAD, lines)


def healer_section(ctx: Context) -> str:
    """Healer fees, which do not depend on Intelligence."""
    lines = []
    for row in shops_of(ctx, "healer"):
        fees = details(row)
        cells = "".join(
            f'<td class="num">{"Free" if fees[k] is None else fees[k]}</td>'
            for k in ("cure", "heal", "resurrect")
        )
        lines.append(f"<tr>{name_cells(row)}{cells}</tr>")
    return (
        "<p>Healers charge a fixed fee in gold per person. Your Intelligence "
        "makes no difference.</p>"
        + table_html(["Town", "Shop", "Cure", "Heal", "Resurrect"], lines)
    )


def horse_ship_section(ctx: Context) -> str:
    """Horses and ships with their base prices."""
    horses = []
    for row in shops_of(ctx, "horse"):
        horses.append(
            f"<tr>{name_cells(row)}"
            f"{price_cells(int(str(details(row)['base_price'])))}</tr>"
        )
    ships = []
    for row in shops_of(ctx, "ship"):
        info = details(row)
        ships.append(
            f"<tr>{name_cells(row)}<td>Frigate</td>"
            f"{price_cells(int(str(info['frigate_base_price'])))}</tr>"
            f"<tr>{name_cells(row)}<td>Skiff</td>"
            f"{price_cells(int(str(info['skiff_base_price'])))}</tr>"
        )
    return (
        "<h3 id='horses'>Horses</h3>"
        + table_html(["Town", "Dealer"] + PRICE_HEAD, horses)
        + "<h3 id='ships'>Ships</h3>"
        + table_html(["Town", "Shipwright", "Vessel"] + PRICE_HEAD, ships)
    )


def inn_tavern_section(ctx: Context) -> str:
    """Inns and taverns."""
    inns = []
    for row in shops_of(ctx, "inn"):
        info = details(row)
        inns.append(
            f"<tr>{name_cells(row)}<td class='num'>{info['nightly_base_price']}</td>"
            f"<td class='num'>{info['guest_capacity']}</td></tr>"
        )
    taverns = []
    for row in shops_of(ctx, "tavern"):
        info = details(row)
        pack = (
            "Not offered"
            if info["pack"] is None
            else f"{esc(info['pack'])} {info['pack_base_price']}"
        )
        taverns.append(
            f"<tr>{name_cells(row)}<td>{esc(info['meal'])}</td>"
            f"<td class='num'>{info['meal_price_per_diner']}</td>"
            f"<td>{esc(info['drink'])}</td><td>{pack}</td></tr>"
        )
    bottles = find_table(ctx.rules, "MERCH-0012", "West Brittany key")[1]
    bottle_lines = [
        f"<tr><th scope='row'>{esc(r[1])}</th><td class='num'>{esc(r[2])}</td></tr>"
        for r in bottles
    ]
    return (
        "<h3 id='inns'>Inns</h3>"
        "<p>The base price is per person for a night. The inn charges it for "
        "everyone in your party, after your Intelligence adjusts it. An inn has "
        "room for only so many guests.</p>"
        + table_html(["Town", "Inn", "Base per night", "Guest capacity"], inns)
        + "<h3 id='taverns'>Taverns</h3>"
        "<p>Meals cost the stated gold per diner and are not adjusted by "
        "Intelligence. The provision pack's base price is adjusted like the rest.</p>"
        + table_html(
            ["Town", "Tavern", "Meal", "Per diner", "Drink", "Provision pack base"],
            taverns,
        )
        + "<p>West Brittany's tavern also sells bottles at fixed prices.</p>"
        + table_html(["Bottle", "Price"], bottle_lines)
    )


def build_shops(ctx: Context) -> None:
    """Build the Shops and prices page."""
    dataset = ctx.datasets["shops"]
    example = "".join(
        f'<td class="num">{adjusted_buy(100, int(i))}</td>' for i in INTELLIGENCES
    )
    families = {
        "arms": "Arms and armour",
        "reagents": "Reagents",
        "guild": "Guild provisions",
        "healers": "Healers",
    }
    sections = "".join(
        f'<h2 id="{key}">{label}</h2>{content}'
        for (key, label), content in zip(
            families.items(),
            (
                arms_section(ctx),
                reagent_section(ctx),
                guild_section(ctx),
                healer_section(ctx),
            ),
            strict=True,
        )
    )
    body = (
        crumbs(("The original game", "/original/"), ("Shops and prices", ""))
        + "<h1>Shops and prices</h1>"
        "<p class='lede'>Who sells what, and for how much. Every price here is a "
        "base price in gold; what you pay depends on your Intelligence.</p>"
        + source_badges(ctx, dataset)
        + "<h2>How prices work</h2>"
        "<p>The character who talks to the merchant is the buyer. A smarter buyer "
        "pays a smaller markup on the base price. Healer fees, tavern meals, "
        "ordinary drinks and wine do not change with Intelligence. Prices round "
        "down to whole gold.</p>"
        + table_html(
            ["Base price"] + [f"Buyer INT {i}" for i in INTELLIGENCES],
            [f'<tr><th scope="row" class="num">100</th>{example}</tr>'],
        )
        + "<p class='muted'>The INT columns in the tables below show the price "
        "for buyers of Intelligence 10, 20 and 30. The Game Facts establish no "
        "haggling and no karma markup.</p>"
        + sections
        + "<h2>Horses, ships, inns and taverns</h2>"
        + horse_ship_section(ctx)
        + inn_tavern_section(ctx)
        + data_panel(
            "shops",
            shops_of(ctx, "reagents")[0],
            "Prices are base prices; derived values show the adjusted price.",
        )
        + sources_footer(ctx, list(SHOP_RULES))
    )
    page(
        ctx,
        "original/shops/index.html",
        "Shops and prices",
        body,
        "original",
        "Base prices at every arms shop, reagent seller, healer, inn and more.",
    )
    ctx.add(
        Entry(
            "facts",
            "shops",
            "Shops and prices",
            "Who sells what, and for how much",
            "/original/shops/",
            ["MERCH-0002", "MERCH-0004", "MERCH-0005", "MERCH-0007", "MERCH-0009"],
        )
    )


def slow_terrain(ctx: Context) -> str:
    """Show TIME-0004's slow-step table with plain headings."""
    _, rows = tables(ctx.rules["TIME-0004"]["text"])[0]
    lines = [
        f"<tr><th scope='row'>{esc(r[0])}</th><td class='num'>{esc(r[1])}</td>"
        f"<td class='num'>{esc(r[2])}</td><td class='num'>{esc(r[3])}</td></tr>"
        for r in rows
    ]
    return table_html(
        ["Terrain you step onto", "Extra minutes", "Minutes per step", "Enemy turns"],
        lines,
    )


def build_commands(ctx: Context) -> None:
    """Build the Commands and time page."""
    dataset = ctx.datasets["commands"]
    rows = records(dataset)
    columns = [("Key", "key", ""), ("Command", "command", "")] + [
        (label, key, "") for key, label in CONTEXT_LABELS.items()
    ]
    lines = "".join(
        f'<tr data-key="{esc(r["key"])}"><th scope="row" class="mono">'
        f'{esc(r["key"])}</th><td>{esc(r["command"])}</td>'
        + "".join(f"<td>{cell(r[k])}</td>" for k in CONTEXT_LABELS)
        + "</tr>"
        for r in rows
    )
    body = (
        crumbs(("The original game", "/original/"), ("Commands and time", ""))
        + "<h1>Commands and time</h1>"
        "<p class='lede'>What each key does where you are, and the minutes the "
        "game charges for it.</p>" + source_badges(ctx, dataset) + "<h2>Commands</h2>"
        "<p>Dir means the game asks which way. Current means the dungeon square "
        "you stand on. Not here means the key is refused.</p>"
        + "<div data-table-scope>"
        + tools([], "a command")
        + '<div class="table-wrap"><table data-json="/data/commands.json">'
        + header_row(columns)
        + f"<tbody>{lines}</tbody></table></div></div>"
        "<p class='muted'>The original does not list a separate cost for each "
        "key. An accepted command costs one step of the place you are in; see "
        "below. Cancelling inside a command does not always make it free.</p>"
        + "<h2>Time</h2>"
        '<div class="stats">'
        '<div class="stat"><div class="label">Calendar</div>'
        '<div class="value">60 · 24 · 28 · 13</div>'
        '<div class="note">Minutes per hour, hours per day, days per month, '
        "months per year.</div></div>"
        '<div class="stat"><div class="label">Outdoors</div>'
        '<div class="value">2 min</div><div class="note">Per accepted command, '
        "such as a step on grass.</div></div>"
        '<div class="stat"><div class="label">Town</div>'
        '<div class="value">1 min</div><div class="note">Per accepted '
        "command.</div></div>"
        '<div class="stat"><div class="label">Dungeon</div>'
        '<div class="value">1 min</div><div class="note">Normally charged '
        "before you choose.</div></div>"
        "</div>"
        "<ul>"
        "<li>Pass (Space) takes a normal step, except an outdoor attack Pass, "
        "which is free.</li>"
        "<li>Quick save (Q) takes no time outdoors or in towns.</li>"
        "<li>In combat, one minute passes before every tenth creature or "
        "character acts. It is not every tenth round.</li>"
        "<li>Quickness halves each request, to at least 1 minute. Time Stop "
        "stops the clock.</li>"
        "<li>Camping runs in 5-minute steps. A bed runs in 10-minute steps. "
        "Repairing a ship takes about 27 minutes if nothing interrupts it.</li>"
        "<li>Sailing with the wind waits 2 minutes at a time, or 1 minute with "
        "enhanced rigging.</li>"
        "</ul>"
        "<h3>Slow terrain on the world map</h3>"
        + slow_terrain(ctx)
        + data_panel(
            "commands",
            rows[0],
            "Each cell is the table entry for that place, as written.",
        )
        + sources_footer(
            ctx,
            [
                "CMD-0001",
                "CMD-0002",
                "CMD-0003",
                "TIME-0001",
                "TIME-0002",
                "TIME-0003",
                "TIME-0004",
                "TIME-0006",
            ],
        )
    )
    page(
        ctx,
        "original/commands/index.html",
        "Commands and time",
        body,
        "original",
        "What each key does in each place, and what time it costs.",
    )
    ctx.add(
        Entry(
            "facts",
            "commands",
            "Commands and time",
            "What each key does, and the minutes it costs",
            "/original/commands/",
            ["CMD-0001", "CMD-0002", "TIME-0002", "TIME-0004"],
        )
    )


PAGE_BUILDERS = [build_spells, build_shops, build_commands]
