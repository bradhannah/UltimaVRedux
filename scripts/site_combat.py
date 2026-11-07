"""Computed tables for the combat page: hit odds, one worked fight, and chest loot.

Every number is worked out at build time from Game Facts tables and formulas, and
the hit odds are checked against the figures EQUIP-0012 quotes. Like the rules'
own quoted odds, they assume a uniform random source.
"""

import re
from fractions import Fraction

from gamefacts_public import JSONValue, require
from site_context import Context, esc
from site_data import d30_at_most, mapping, objects, strings, tables, whole_percent

type Obj = dict[str, JSONValue]
type Tiers = list[tuple[int, list[str]]]
type Step = tuple[str, str]

# EQUIP-0012 names the bludgeoning category; those weapons use Strength to hit.
BLUDGEONING = ("Spiked Helm", "Spiked Shield", "Club", "Mace", "Two-Handed Hammer")
HIT_GAPS = (-20, -10, -5, 0, 5, 10, 15, 20, 28)
CHEST_VALUES = (5, 10, 15, 20, 25, 30)
LOOT_LABELS = {
    "Child chest": "Another chest",
    "Gold": "Gold, 1 to 3 × value",
    "Potion": "A potion",
    "Scroll": "A scroll",
}
NEVER = 0xFF
CHEST_ROLL = 30


def percent(chance: Fraction) -> str:
    """Show a probability as a whole percent, never rounding to a false 0 or 100."""
    value = whole_percent(chance)
    if chance in (0, 1):
        return f"{value}%"
    if value >= 100:
        return ">99%"
    return "<1%" if value < 1 else f"{value}%"


def number(row: Obj, key: str) -> int:
    """Read a whole-number field."""
    value = row[key]
    require(isinstance(value, int), f"{key} must be a whole number")
    return int(str(value))


def threshold(attack: int, defence: int) -> int:
    """EQUIP-0012: (30 + defender DEX - attacking attribute) / 2, truncated."""
    return int(Fraction(30 + defence - attack, 2))


def hit_chance(attack: int, defence: int) -> Fraction:
    """EQUIP-0012: the transformed d30 must meet or beat the threshold."""
    return 1 - d30_at_most(threshold(attack, defence) - 1)


def chest_roll(minimum: int) -> Fraction:
    """CMD-0009: a direct 1..30 draw at least the item's threshold."""
    return Fraction(max(CHEST_ROLL + 1 - minimum, 0), CHEST_ROLL)


def table(head: list[str], rows: list[list[str]]) -> str:
    """Render a table whose first column labels each row; cells are plain text."""
    header = "".join(
        f'<th scope="col"{"" if i == 0 else ' class="num"'}>{esc(text)}</th>'
        for i, text in enumerate(head)
    )
    body = "".join(
        f'<tr><th scope="row">{esc(row[0])}</th>'
        + "".join(f'<td class="num">{esc(value)}</td>' for value in row[1:])
        + "</tr>"
        for row in rows
    )
    return (
        f'<div class="table-wrap"><table><thead><tr>{header}</tr></thead>'
        f"<tbody>{body}</tbody></table></div>"
    )


def hit_table(ctx: Context) -> str:
    """Hit chance by attribute gap, checked against EQUIP-0012's quoted odds."""
    quoted = re.findall(r"(\d+)% at (\d+)", ctx.rules["EQUIP-0012"]["text"])
    require(len(quoted) == 3, "EQUIP-0012 no longer quotes three example odds")
    for chance, target in quoted:
        actual = whole_percent(1 - d30_at_most(int(target) - 1))
        require(actual == int(chance), f"EQUIP-0012 odds at {target} disagree")
    return table(
        ["Your attribute minus their DEX"]
        + [f"{gap:+d}".replace("-", "−") if gap else "0" for gap in HIT_GAPS],
        [
            ["Target"] + [str(threshold(gap, 0)) for gap in HIT_GAPS],
            ["Chance to hit"] + [percent(hit_chance(gap, 0)) for gap in HIT_GAPS],
        ],
    )


def record(ctx: Context, dataset: str, name: str) -> Obj:
    """Find one dataset record by name."""
    for row in objects(ctx.datasets[dataset]["records"]):
        if row["name"] == name:
            return row
    raise ValueError(f"{dataset} has no {name}")


def encounter_dex(base: int) -> list[int]:
    """ENEMY-0003: base DEX + R(0,7) - 4, reset to base when outside 0-30."""
    return [d if 0 <= d <= 30 else base for d in (base + r - 4 for r in range(8))]


def spread(values: list[int]) -> str:
    """Show a range of whole numbers, or one number when they agree."""
    low, high = min(values), max(values)
    return str(low) if low == high else f"{low} to {high}"


def odds_spread(chances: list[Fraction], outcome: str) -> str:
    """Show the lowest and highest of several equally likely chances and their mean."""
    low, high = percent(min(chances)), percent(max(chances))
    if low == high:
        return f"{low} {outcome}"
    average = percent(sum(chances, Fraction(0)) / len(chances))
    return f"{low} to {high} {outcome}, {average} on average"


def damage(rating: int, rolled: bool, armour: int) -> dict[int, Fraction]:
    """EQUIP-0013: a 1..rating roll (or the fixed rating) minus a 1..armour roll."""
    attacks = range(1, rating + 1) if rolled and rating > 1 else [rating]
    blocks = range(1, armour + 1) if armour > 0 else [0]
    share = Fraction(1, len(attacks) * len(blocks))
    result: dict[int, Fraction] = {}
    for attack in attacks:
        for block in blocks:
            left = max(attack - block, 0)
            result[left] = result.get(left, Fraction(0)) + share
    return result


def mean(outcomes: dict[int, Fraction]) -> str:
    """Average damage to one decimal place."""
    return f"{float(sum(v * p for v, p in outcomes.items())):.1f}"


def your_blows(foe: Obj, attack: int, you: int) -> list[Step]:
    """Your hit chance against the foe's rerolled Dexterity, and your damage."""
    name, dexes = foe["name"], encounter_dex(number(foe, "dex"))
    armour, hp = number(foe, "armour"), number(foe, "hp")
    dealt = damage(attack, True, armour)
    kills = sum((p for v, p in dealt.items() if v >= hp), Fraction(0))
    return [
        (
            "Your swing.",
            f"The {name}'s Dexterity this fight is {spread(dexes)}, so your target "
            f"is {spread([threshold(you, d) for d in dexes])}: "
            f"{odds_spread([hit_chance(you, d) for d in dexes], 'to hit')}.",
        ),
        (
            "Your damage.",
            f"1–{attack}, minus 1–{armour} for its armour. "
            f"{percent(dealt.get(0, Fraction(0)))} of hits graze; the average hit "
            f"does {mean(dealt)}, and {percent(kills)} of hits kill it outright.",
        ),
    ]


def its_blows(foe: Obj, you: int, worn: int) -> list[Step]:
    """The foe's hit chance (Strength when it bludgeons) and its fixed damage."""
    name, dexes = foe["name"], encounter_dex(number(foe, "dex"))
    their, how = dexes, f"The {name} uses its Dexterity ({spread(dexes)})"
    if "bludgeons" in strings(foe["abilities"]):
        their = [number(foe, "str")] * len(dexes)
        how = f"The {name} bludgeons, so it uses STR {their[0]}"
    taken = damage(number(foe, "damage"), False, worn)
    return [
        (
            "Its swing.",
            f"{how} against your DEX {you}. Its target is "
            f"{spread([threshold(a, you) for a in their])}: "
            f"{odds_spread([hit_chance(a, you) for a in their], 'to hit you')}.",
        ),
        (
            "Its damage.",
            f"Always {foe['damage']}, minus 1–{worn} for your armour: "
            f"{spread(list(taken))}, {mean(taken)} on average.",
        ),
    ]


def rewards(foe: Obj) -> list[Step]:
    """XP for the kill and the chance of a chest."""
    extra = mapping(foe["derived"])
    loot = "It never leaves a chest."
    if extra["chest_percent"] is not None:
        loot = (
            f"A chest {extra['chest_percent']}% of the time; "
            f"{extra['trapped_percent']}% of those chests are trapped."
        )
    return [
        (
            "The kill.",
            f"{foe['hp']} ÷ 4, rounded down, + 1 = {extra['kill_xp']} XP, all to "
            "whoever lands the last blow.",
        ),
        ("The loot.", loot),
    ]


def fight_example(ctx: Context, block: Obj) -> str:
    """Work one exchange of blows through every rule, from the content's choices."""
    foe = record(ctx, "creatures", str(block["creature"]))
    weapon = record(ctx, "equipment", str(block["weapon"]))
    you, worn = number(block, "dex"), number(block, "armour")
    attack = number(weapon, "attack")
    require(weapon["name"] not in BLUDGEONING, "the example weapon must use DEX")
    require(
        not set(strings(foe["abilities"])) & {"undead", "immortal"},
        "the example foe must take full damage",
    )
    for weapon_name in BLUDGEONING:
        require(weapon_name in ctx.rules["EQUIP-0012"]["text"], "bludgeons changed")
    steps = your_blows(foe, attack, you) + its_blows(foe, you, worn) + rewards(foe)
    items = "".join(f"<li><b>{esc(t)}</b> {esc(x)}</li>" for t, x in steps)
    return (
        f"<p>A party member with DEX {you}, armour {worn} and a {esc(weapon['name'])}"
        f" (Attack {attack}) meets the {esc(foe['name'])} (DEX {foe['dex']}, STR "
        f"{foe['str']}, Armour {foe['armour']}, Damage {foe['damage']}, "
        f"HP {foe['hp']}).</p>"
        f'<ol class="steps">{items}</ol>'
    )


def arms_tiers(ctx: Context) -> Tiers:
    """Read CMD-0009's armament thresholds, requiring every EQUIP-0001 item once."""
    header, rows = tables(ctx.rules["CMD-0009"]["text"])[1]
    require(header == ["Threshold", "Equipment"], "CMD-0009 armament table changed")
    tiers = [(int(row[0], 0), row[1].split(", ")) for row in rows]
    listed = sorted(item for _, items in tiers for item in items)
    names = sorted(
        str(r["name"]) for r in objects(ctx.datasets["equipment"]["records"])
    )
    require(listed == names, "CMD-0009 must list every EQUIP-0001 item once")
    return tiers


def at_least_one(chance: Fraction, value: int) -> Fraction:
    """CMD-0009: a value-v chest makes floor(v/2)+1 armament attempts."""
    return 1 - (1 - chance) ** (value // 2 + 1)


def arms_attempt(tiers: Tiers, value: int) -> Fraction:
    """One attempt picks any item, then must pass that item's value and roll tests."""
    count = sum(len(items) for _, items in tiers)
    return sum(
        (len(items) * chest_roll(low) / count for low, items in tiers if low <= value),
        Fraction(0),
    )


def chest_contents(ctx: Context) -> str:
    """Chance of each find, by chest value, from CMD-0009's two tables."""
    header, rows = tables(ctx.rules["CMD-0009"]["text"])[0]
    require(header[:2] == ["Category", "Threshold"], "CMD-0009 loot table changed")
    lines = []
    for category, low, result in sorted(rows, key=lambda row: int(row[1])):
        if re.fullmatch(r"\d+–\d+", result):
            label = f"{category}, {result}"
        else:
            require(category in LOOT_LABELS, f"no label for {category}")
            label = LOOT_LABELS[category]
        chance = percent(chest_roll(int(low)))
        lines.append([label] + [chance if int(low) <= v else "—" for v in CHEST_VALUES])
    tiers = arms_tiers(ctx)
    lines.append(
        ["Arms or armour (tries)"]
        + [
            f"{percent(at_least_one(arms_attempt(tiers, v), v))} ({v // 2 + 1})"
            for v in CHEST_VALUES
        ]
    )
    return table(["Chest value"] + [str(v) for v in CHEST_VALUES], lines)


def chest_arms(ctx: Context) -> str:
    """Each armament tier, and its chance in the richest creature's chest."""
    tiers = arms_tiers(ctx)
    count = sum(len(items) for _, items in tiers)
    creatures = objects(ctx.datasets["creatures"]["records"])
    value = max(number(c, "treasure") for c in creatures)
    richest = " or ".join(str(c["name"]) for c in creatures if c["treasure"] == value)
    lines = []
    for low, items in tiers:
        chance = Fraction(len(items), count) * chest_roll(low)
        lines.append(
            [
                "Never" if low == NEVER else str(low),
                ", ".join(items),
                "—" if low == NEVER else percent(at_least_one(chance, value)),
            ]
        )
    body = "".join(
        f'<tr><th scope="row" class="num">{esc(low)}</th><td>{esc(items)}</td>'
        f'<td class="num">{esc(chance)}</td></tr>'
        for low, items, chance in lines
    )
    return (
        '<div class="table-wrap"><table><thead><tr>'
        '<th scope="col" class="num">Minimum</th><th scope="col">Arms and armour</th>'
        f'<th scope="col" class="num">In a {esc(richest)} chest (value {value})</th>'
        f"</tr></thead><tbody>{body}</tbody></table></div>"
    )
