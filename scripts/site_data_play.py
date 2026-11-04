"""Datasets for the player-facing pages: spells, reagents, shops and commands.

Like site_data, every value comes from a public Game Facts table, each dataset
names its rules, and values computed from another rule sit under `derived`.
"""

import re
from collections.abc import Callable

from gamefacts_public import require
from site_data import Record, Rules, Table, objects, source, table_dicts, tables

CONTEXTS = {"O": "outdoors", "T": "town", "D": "dungeon", "C": "combat"}
# MIX-0001: original shop labels name the same ingredients as the recipe table.
REAGENT_LABELS = {"Sulfur Ash": "Sulfurous Ash", "Mandrake": "Mandrake Root"}
INTELLIGENCES = (10, 20, 30)
SHOP_RULES = (
    "MERCH-0001",
    "MERCH-0002",
    "MERCH-0004",
    "MERCH-0005",
    "MERCH-0006",
    "MERCH-0007",
    "MERCH-0008",
    "MERCH-0009",
    "MERCH-0012",
)


def find_table(rules: Rules, identifier: str, first_header: str) -> Table:
    """Return the table in a rule whose first column has the given header."""
    for table in tables(rules[identifier]["text"]):
        if table[0][0] == first_header:
            return table
    raise ValueError(f"{identifier}: no table starting with {first_header!r}")


def adjusted_buy(base: int, intelligence: int) -> int:
    """MERCH-0002: base plus the Intelligence-dependent markup, truncated."""
    return base + (100 - 3 * intelligence) * base // 100


def prices(base: int) -> dict[str, int]:
    """Show what a buyer of Intelligence 10, 20 and 30 pays for a base price."""
    return {str(i): adjusted_buy(base, i) for i in INTELLIGENCES}


def parse_bundle(entry: str) -> tuple[str, int, int]:
    """Parse 'Ginseng 20/10' into a reagent label, base price and unit count."""
    match = re.fullmatch(r"\s*(.+?) (\d+)/(\d+)\s*", entry)
    if match is None:
        raise ValueError(f"bad reagent bundle: {entry!r}")
    name = REAGENT_LABELS.get(match.group(1), match.group(1))
    return name, int(match.group(2)), int(match.group(3))


def reagents(rules: Rules) -> Record:
    """Build the eight reagents from MIX-0001, with where each is sold (MERCH-0005)."""
    sold: dict[str, list[Record]] = {}
    for row in table_dicts(find_table(rules, "MERCH-0005", "Location")):
        for entry in row["Bundles"].split(";"):
            name, base, units = parse_bundle(entry)
            sold.setdefault(name, []).append(
                {
                    "location": row["Location"],
                    "shop": row["Shop / owner"].split(" / ")[0],
                    "base_price": base,
                    "units": units,
                }
            )
    records: list[Record] = []
    for row in table_dicts(find_table(rules, "MIX-0001", "Inventory index")):
        name = row["Reagent"]
        records.append(
            {
                "index": int(row["Inventory index"]),
                "name": name,
                "abbreviation": row["Abbreviation"],
                "derived": {"sold_at": sold.get(name, [])},
            }
        )
    return {
        "dataset": "reagents",
        "schema_version": 1,
        "title": "Reagents",
        "key": "index",
        "source": source(rules, "MIX-0001", "MERCH-0005"),
        "derived_from": {"sold_at": "MERCH-0005"},
        "records": records,
    }


def spells(rules: Rules) -> Record:
    """Build the 48 prepared spells: SPELL-0002 effects joined to MIX-0001 recipes."""
    names = {
        r["Abbreviation"]: r["Reagent"]
        for r in table_dicts(find_table(rules, "MIX-0001", "Inventory index"))
    }
    recipes = {
        int(r["Spell index"]): r
        for r in table_dicts(find_table(rules, "MIX-0001", "Spell index"))
    }
    records: list[Record] = []
    for row in table_dicts(find_table(rules, "SPELL-0002", "Index")):
        index = int(row["Index"])
        recipe = recipes[index]
        require(recipe["Spell"] == row["Spell"], f"spell {index} names differ")
        require(recipe["Circle"] == row["Circle"], f"spell {index} circles differ")
        records.append(
            {
                "index": index,
                "name": row["Spell"],
                "circle": int(row["Circle"]),
                "contexts": [CONTEXTS[c] for c in row["Context"].split("/")],
                "reagents": [names[a] for a in recipe["Ingredients"].split(", ")],
                "effect": row["Principal original effect"],
                "derived": {"mp_cost": int(row["Circle"])},
            }
        )
    return {
        "dataset": "spells",
        "schema_version": 1,
        "title": "Spells",
        "key": "index",
        "source": source(rules, "SPELL-0002", "MIX-0001", "SPELL-0001"),
        "derived_from": {"mp_cost": "SPELL-0001"},
        "records": records,
    }


def shop(family: str, location: str, text: str, details: Record) -> Record:
    """Build one shop record; `text` is the table's 'Shop / owner' cell."""
    name, _, owner = text.partition(" / ")
    slug = re.sub(r"[^a-z0-9]+", "-", f"{family}-{location}".lower()).strip("-")
    return {
        "id": slug,
        "family": family,
        "location": location,
        "shop": name,
        "owner": owner,
        "details": details,
    }


def arms_shops(rules: Rules, base_prices: dict[str, int]) -> list[Record]:
    """Arms shops: stock in offered order, each with its EQUIP-0001 base price."""
    found = []
    for row in table_dicts(find_table(rules, "MERCH-0004", "Location")):
        stock: list[Record] = []
        for item in row["Stock in offered order"].split(", "):
            require(item in base_prices, f"arms stock {item!r} not in EQUIP-0001")
            stock.append({"item": item, "base_price": base_prices[item]})
        found.append(
            shop("arms", row["Location"], row["Shop / owner"], {"stock": stock})
        )
    return found


def reagent_shops(rules: Rules) -> list[Record]:
    """Reagent shops: bundles priced as base gold per units."""
    found = []
    for row in table_dicts(find_table(rules, "MERCH-0005", "Location")):
        bundles: list[Record] = []
        for entry in row["Bundles"].split(";"):
            name, base, units = parse_bundle(entry)
            bundles.append(
                {
                    "reagent": name,
                    "base_price": base,
                    "units": units,
                    "derived": {"price_at_intelligence": prices(base)},
                }
            )
        found.append(
            shop("reagents", row["Location"], row["Shop / owner"], {"bundles": bundles})
        )
    return found


def guild_shops(rules: Rules) -> list[Record]:
    """Guild provisions from the three mapped rows; the unmapped row is omitted."""
    found = []
    for row in table_dicts(find_table(rules, "MERCH-0006", "Row")):
        if row["Mapping / shop / owner"].startswith("Unmapped"):
            continue
        location, name, owner = row["Mapping / shop / owner"].split(" / ")
        offers: list[Record] = []
        for column, item, units in (
            ("Keys ×3", "Keys", 3),
            ("Gems ×4", "Gems", 4),
            ("Torches ×5", "Torches", 5),
        ):
            base = int(row[column])
            offers.append(
                {
                    "item": item,
                    "units": units,
                    "base_price": base,
                    "derived": {"price_at_intelligence": prices(base)},
                }
            )
        found.append(shop("guild", location, f"{name} / {owner}", {"offers": offers}))
    return found


def healer_shops(rules: Rules) -> list[Record]:
    """Healers: fixed prices that ignore Intelligence; None means free."""

    def amount(value: str) -> int | None:
        return None if value == "Free" else int(value)

    return [
        shop(
            "healer",
            row["Location"],
            row["Shop / owner"],
            {
                "cure": amount(row["Cure"]),
                "heal": amount(row["Heal"]),
                "resurrect": amount(row["Resurrect"]),
            },
        )
        for row in table_dicts(find_table(rules, "MERCH-0007", "Location"))
    ]


def horse_shops(rules: Rules) -> list[Record]:
    """Horse dealers with a supported active merchant."""
    found = []
    for row in table_dicts(find_table(rules, "MERCH-0008", "Source row")):
        if not row["Availability"].startswith("Active"):
            continue
        base = int(row["Base"])
        found.append(
            shop(
                "horse",
                row["Horse location mapping"],
                row["Shop / owner"],
                {
                    "base_price": base,
                    "derived": {"price_at_intelligence": prices(base)},
                },
            )
        )
    return found


def ship_shops(rules: Rules) -> list[Record]:
    """Shipwrights: frigate and skiff base prices (berth coordinates are omitted)."""
    found = []
    for row in table_dicts(find_table(rules, "MERCH-0008", "Ship location")):
        frigate, skiff = int(row["Frigate base"]), int(row["Skiff base"])
        found.append(
            shop(
                "ship",
                row["Ship location"],
                row["Shop / owner"],
                {
                    "frigate_base_price": frigate,
                    "skiff_base_price": skiff,
                    "derived": {
                        "frigate_price_at_intelligence": prices(frigate),
                        "skiff_price_at_intelligence": prices(skiff),
                    },
                },
            )
        )
    return found


def inn_shops(rules: Rules) -> list[Record]:
    """Inns: nightly base price per party member, and guest capacity."""
    return [
        shop(
            "inn",
            row["Location"],
            row["Inn / owner"],
            {
                "nightly_base_price": int(row["Nightly base"]),
                "guest_capacity": int(row["Guest capacity"]),
            },
        )
        for row in table_dicts(find_table(rules, "MERCH-0009", "Location"))
    ]


def tavern_shops(rules: Rules) -> list[Record]:
    """Taverns: meal, drink and provision pack."""
    found = []
    for row in table_dicts(find_table(rules, "MERCH-0012", "Location")):
        meal, _, meal_price = row["Meal / per-diner price"].partition(" / ")
        pack = re.fullmatch(r"(\w+) (\d+)", row["Pack base"])
        found.append(
            shop(
                "tavern",
                row["Location"],
                row["Tavern / owner"],
                {
                    "meal": meal,
                    "meal_price_per_diner": int(meal_price),
                    "drink": row["Drink"],
                    "pack": pack.group(1) if pack else None,
                    "pack_base_price": int(pack.group(2)) if pack else None,
                },
            )
        )
    return found


def shops(rules: Rules, equipment: Record) -> Record:
    """Build every active shop from the MERCH tables."""
    base_prices = {
        str(r["name"]): int(str(r["base_price"]))
        for r in objects(equipment["records"])
        if r["base_price"] is not None
    }
    builders: list[Callable[[], list[Record]]] = [
        lambda: arms_shops(rules, base_prices),
        lambda: reagent_shops(rules),
        lambda: guild_shops(rules),
        lambda: healer_shops(rules),
        lambda: horse_shops(rules),
        lambda: ship_shops(rules),
        lambda: inn_shops(rules),
        lambda: tavern_shops(rules),
    ]
    records = [record for build in builders for record in build()]
    return {
        "dataset": "shops",
        "schema_version": 1,
        "title": "Shops",
        "key": "id",
        "source": source(rules, *SHOP_RULES, "EQUIP-0001"),
        "derived_from": {"price_at_intelligence": "MERCH-0002"},
        "records": records,
    }


def commands(rules: Rules) -> Record:
    """Build the command matrix from CMD-0001."""
    records: list[Record] = []
    for row in table_dicts(find_table(rules, "CMD-0001", "Key")):
        records.append(
            {
                "key": row["Key"],
                "command": row["Command"],
                "outdoors": row["Britannia / Underworld"],
                "town": row["Small map"],
                "dungeon": row["Dungeon exploration"],
                "combat": row["Tactical combat"],
            }
        )
    return {
        "dataset": "commands",
        "schema_version": 1,
        "title": "Commands",
        "key": "key",
        "source": source(rules, "CMD-0001"),
        "records": records,
    }


def build_all(rules: Rules, equipment: Record) -> dict[str, Record]:
    """Build the player-facing datasets."""
    return {
        "spells": spells(rules),
        "reagents": reagents(rules),
        "shops": shops(rules, equipment),
        "commands": commands(rules),
    }
