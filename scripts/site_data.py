"""Build the public JSON datasets from Game Facts tables.

Every dataset records the rule IDs it came from and their Original status. Values
computed from other rules sit in a record's `derived` block, and the dataset names
the rule behind each derived value in `derived_from`. Nothing here reads the
original game files; every number comes from a public Game Facts table.
"""

import re
from collections.abc import Callable
from typing import cast

from gamefacts_public import JSONValue, Rule, field, require
from site_markdown import is_table_start, split_row

type Rules = dict[str, Rule]
type Record = dict[str, JSONValue]
type Table = tuple[list[str], list[list[str]]]

# ENEMY-0001 ability word. Labels without a verified consumer are left out.
MODE_BITS = {
    0x2000: "teleports",
    0x1000: "vanishes_on_death",
    0x0800: "turns_invisible",
    0x0400: "summons_daemons",
    0x0200: "poisons",
    0x0080: "bludgeons",
    0x0040: "possesses",
    0x0020: "undead",
    0x0010: "divides",
    0x0008: "immortal",
    0x0004: "ranged_poison",
    0x0002: "steals_food",
    0x0001: "no_corpse",
}
TERRAIN_CLASSES = {
    "On foot": "on_foot",
    "Water": "water",
    "Flying": "flying",
    "Ether": "ether",
    "Desert": "desert",
    "Deep water": "deep_water",
    "Grass": "grass",
    "Swamp": "swamp",
}
LOCATION_KINDS = {
    "TOWNE.DAT": "town",
    "DWELLING.DAT": "dwelling",
    "CASTLE.DAT": "castle",
    "KEEP.DAT": "keep",
    "DUNGEON.DAT": "dungeon",
}
RUNIC = "ABCDEFGHIJKLMNOP"


def objects(value: JSONValue) -> list[Record]:
    """Narrow a JSON value to a list of objects."""
    require(isinstance(value, list), "expected a JSON list")
    found = [item for item in cast(list[JSONValue], value) if isinstance(item, dict)]
    require(len(found) == len(cast(list[JSONValue], value)), "expected objects")
    return found


def strings(value: JSONValue) -> list[str]:
    """Narrow a JSON value to a list of strings."""
    require(isinstance(value, list), "expected a JSON list")
    items = cast(list[JSONValue], value)
    require(all(isinstance(item, str) for item in items), "expected strings")
    return [str(item) for item in items]


def mapping(value: JSONValue) -> Record:
    """Narrow a JSON value to an object."""
    require(isinstance(value, dict), "expected a JSON object")
    return cast(Record, value)


def tables(text: str) -> list[Table]:
    """Return every pipe table in a rule as a header and string rows."""
    lines = text.split("\n")
    found: list[Table] = []
    index = 0
    while index < len(lines):
        if is_table_start(lines, index):
            header = split_row(lines[index])
            rows: list[list[str]] = []
            index += 2
            while index < len(lines) and lines[index].lstrip().startswith("|"):
                rows.append(split_row(lines[index]))
                index += 1
            found.append((header, rows))
        else:
            index += 1
    return found


def table_dicts(table: Table) -> list[dict[str, str]]:
    """Turn a table with unique headers into one dictionary per row."""
    header, rows = table
    require(len(set(header)) == len(header), f"duplicate table headers: {header}")
    return [dict(zip(header, row, strict=True)) for row in rows]


def source(rules: Rules, *identifiers: str) -> list[JSONValue]:
    """Describe the rules behind a dataset with their Original status."""
    return [
        {"rule": identifier, "original": field(rules[identifier]["text"], "Original")}
        for identifier in identifiers
    ]


def number(value: str) -> int | None:
    """Parse a table number, treating a dash or blank as no value."""
    return None if value in {"-", ""} else int(value)


def coordinate(value: str) -> tuple[int, int] | None:
    """Parse an `X,Y` cell, returning None for a sentinel such as `0,0 sentinel`."""
    match = re.fullmatch(r"(\d+),(\d+)", value.strip())
    return (int(match.group(1)), int(match.group(2))) if match else None


def sextant(x: int, y: int) -> str:
    """Encode a coordinate as the Sextant reads it: Y first, nibble by nibble."""

    # SIGN-0007 fixes the order and the Runic A-P mapping; Redux prints H'L".
    def part(value: int) -> str:
        return f"{RUNIC[value >> 4]}'{RUNIC[value & 15]}\""

    return f"{part(y)}, {part(x)}"


def expand_indexes(text: str) -> set[int]:
    """Expand a list such as `0–12, 14, 20` into a set of integers."""
    result: set[int] = set()
    for chunk in text.rstrip(".").split(","):
        bounds = [int(part) for part in re.split(r"[–-]", chunk.strip())]
        result.update(range(bounds[0], bounds[-1] + 1))
    return result


def terrain_classes(rules: Rules) -> dict[int, str]:
    """Map creature index to its ENEMY-0002 terrain class."""
    classes: dict[int, str] = {}
    for row in table_dicts(tables(rules["ENEMY-0002"]["text"])[0]):
        name = TERRAIN_CLASSES.get(row["Class"])
        if name:
            for index in expand_indexes(row["Indexes"]):
                classes[index] = name
    return classes


def creature_group(index: int) -> str:
    """Editorial grouping by index range; ENEMY-0001 does not name the groups."""
    if index <= 15:
        return "person"
    return "field" if index in {42, 43} else "monster"


def creatures(rules: Rules) -> Record:
    """Build the creature catalog from ENEMY-0001 plus derived values."""
    terrain = terrain_classes(rules)
    records: list[JSONValue] = []
    for row in table_dicts(tables(rules["ENEMY-0001"]["text"])[0]):
        index, mode = int(row["Index"]), int(row["Mode"], 16)
        hit_points, reach = int(row["HP"]), int(row["Range"])
        records.append(
            {
                "index": index,
                "name": row["Semantic identity"],
                "str": int(row["STR"]),
                "dex": int(row["DEX"]),
                "int": int(row["INT"]),
                "armour": int(row["Arm"]),
                "damage": int(row["Dmg"]),
                "hp": hit_points,
                "group_max": int(row["Num"]),
                "treasure": int(row["Tr"]),
                "mode": row["Mode"],
                "abilities": [n for bit, n in MODE_BITS.items() if mode & bit],
                "range": reach,
                # Range 1 is melee-only: the table's Arrow entry is not a bow.
                "missile": row["Missile"].lower() if reach > 1 else None,
                "friend": row["Friend"],
                "derived": {
                    "kill_xp": hit_points // 4 + 1 if hit_points else None,
                    "movement": terrain.get(index),
                    "group": creature_group(index),
                },
            }
        )
    return {
        "dataset": "creatures",
        "schema_version": 1,
        "title": "Creatures",
        "key": "index",
        "source": source(rules, "ENEMY-0001", "ENEMY-0002", "PROGRESS-0002"),
        "derived_from": {
            "kill_xp": "PROGRESS-0002",
            "movement": "ENEMY-0002",
            "group": "ENEMY-0001 (editorial grouping by index range)",
        },
        "records": records,
    }


def equipment(rules: Rules) -> Record:
    """Build the equipment catalog from EQUIP-0001."""
    records: list[JSONValue] = [
        {
            "id": int(row["ID"], 16),
            "id_hex": row["ID"],
            "name": row["Item"],
            "slot": row["Slot"].lower(),
            "strength_needed": number(row["Strength"]),
            "attack": number(row["Attack"]),
            "defense": number(row["Defense"]),
            "reach": number(row["Reach"]),
            "base_price": number(row["Base"]),
        }
        for row in table_dicts(tables(rules["EQUIP-0001"]["text"])[0])
    ]
    return {
        "dataset": "equipment",
        "schema_version": 1,
        "title": "Equipment",
        "key": "id",
        "source": source(rules, "EQUIP-0001"),
        "records": records,
    }


def location_kinds(rules: Rules) -> dict[int, tuple[str, str]]:
    """Map location number to its (kind, file) pair from MAP-0003."""
    result: dict[int, tuple[str, str]] = {}
    for row in table_dicts(tables(rules["MAP-0003"]["text"])[0]):
        match = re.fullmatch(r"(\d+)-(\d+)", row["Number"])
        files = re.findall(r"`([A-Z]+\.DAT)`", row["Stored in"])
        if match and files:
            for number_ in range(int(match.group(1)), int(match.group(2)) + 1):
                result[number_] = (LOCATION_KINDS[files[0]], files[0])
    return result


def entrance_rows(rules: Rules) -> list[tuple[int, str, tuple[int, int]]]:
    """Read the two-column MAP-0004 table and add the dungeon entrances."""
    header, rows = tables(rules["MAP-0004"]["text"])[0]
    require(len(header) == 6, "MAP-0004 table shape changed")
    entries: list[tuple[int, str, tuple[int, int]]] = []
    for row in rows:
        for offset in (0, 3):
            xy = coordinate(row[offset + 2])
            require(xy is not None, f"bad MAP-0004 coordinate: {row}")
            entries.append((int(row[offset]), row[offset + 1], xy))
    for row in table_dicts(tables(rules["WORD-0001"]["text"])[0]):
        xy = coordinate(row["X,Y decimal"])
        require(xy is not None, f"bad WORD-0001 coordinate: {row}")
        entries.append((int(row["Index"]) + 33, row["Dungeon"], xy))
    return sorted(entries)


def locations(rules: Rules) -> Record:
    """Build settlements and dungeons from MAP-0003, MAP-0004 and WORD-0001."""
    kinds = location_kinds(rules)
    words = {
        row["Dungeon"]: row["Word"]
        for row in table_dicts(tables(rules["WORD-0001"]["text"])[0])
    }
    records: list[JSONValue] = []
    for identifier, name, (x, y) in entrance_rows(rules):
        kind, file = kinds[identifier]
        # MAP-0004: Ararat's entrance and Doom's lie in the Underworld only; the
        # other seven dungeons have an entrance at the same spot in both worlds.
        if identifier in {25, 40}:
            worlds = ["underworld"]
        elif kind == "dungeon":
            worlds = ["britannia", "underworld"]
        else:
            worlds = ["britannia"]
        record: Record = {
            "number": identifier,
            "name": name,
            "kind": kind,
            "file": file,
            "x": x,
            "y": y,
            "worlds": list(worlds),
            "word_of_power": words.get(name),
        }
        if kind == "dungeon":
            record["file_record"] = identifier - 33
        records.append(record)
    return {
        "dataset": "locations",
        "schema_version": 1,
        "title": "Locations",
        "key": "number",
        "source": source(rules, "MAP-0003", "MAP-0004", "WORD-0001"),
        "spoiler_fields": ["word_of_power"],
        "records": records,
    }


def moon_phases(rules: Rules) -> Record:
    """Build the 28-day Trammel and Felucca phase schedule from MOON-0002."""
    _, rows = tables(rules["MOON-0002"]["text"])[0]
    days: list[tuple[int, int, int]] = []
    for row in rows:
        days.append((int(row[0]), int(row[1]), int(row[2])))
        days.append((int(row[3]), int(row[4]), int(row[5])))
    records: list[JSONValue] = [
        {"day": day, "trammel": trammel, "felucca": felucca}
        for day, trammel, felucca in sorted(days)
    ]
    return {
        "dataset": "moon_phases",
        "schema_version": 1,
        "title": "Moon phases",
        "key": "day",
        "source": source(rules, "MOON-0002"),
        "records": records,
    }


def moonstones(rules: Rules) -> Record:
    """Build the eight starting moonstone placements from MOON-0001."""
    records: list[JSONValue] = []
    for row in table_dicts(tables(rules["MOON-0001"]["text"])[0]):
        xy = coordinate(row["Decimal X,Y"])
        require(xy is not None, f"bad MOON-0001 coordinate: {row}")
        records.append(
            {
                "index": int(row["Stone/phase index"]),
                "x": xy[0],
                "y": xy[1],
                "derived": {"sextant": sextant(*xy)},
            }
        )
    return {
        "dataset": "moonstones",
        "schema_version": 1,
        "title": "Moonstones at the start of a game",
        "key": "index",
        "source": source(rules, "MOON-0001"),
        "derived_from": {"sextant": "SIGN-0007"},
        "records": records,
    }


def shrines(rules: Rules) -> Record:
    """Build the eight shrines from SHRINE-0001."""
    records: list[JSONValue] = []
    for row in table_dicts(tables(rules["SHRINE-0001"]["text"])[0]):
        xy = coordinate(row["World X,Y"])
        records.append(
            {
                "index": int(row["Index"]),
                "virtue": row["Virtue"],
                "mantra": row["Mantra"],
                "x": xy[0] if xy else None,
                "y": xy[1] if xy else None,
                "derived": {"sextant": sextant(*xy) if xy else None},
            }
        )
    return {
        "dataset": "shrines",
        "schema_version": 1,
        "title": "Shrines",
        "key": "index",
        "source": source(rules, "SHRINE-0001"),
        "derived_from": {"sextant": "SIGN-0007"},
        "spoiler_fields": ["mantra"],
        "records": records,
    }


def quest_items(rules: Rules) -> Record:
    """Build the four fixed Underworld quest items from QUEST-0002."""
    records: list[JSONValue] = []
    for row in table_dicts(tables(rules["QUEST-0002"]["text"])[0]):
        xy = coordinate(row["X,Y"])
        require(xy is not None, f"bad QUEST-0002 coordinate: {row}")
        records.append({"name": row["Item"], "x": xy[0], "y": xy[1]})
    return {
        "dataset": "quest_items",
        "schema_version": 1,
        "title": "Quest items placed in the Underworld",
        "key": "name",
        "source": source(rules, "QUEST-0002"),
        "spoiler_fields": ["x", "y"],
        "records": records,
    }


BUILDERS: dict[str, Callable[[Rules], Record]] = {
    "creatures": creatures,
    "equipment": equipment,
    "locations": locations,
    "moon_phases": moon_phases,
    "moonstones": moonstones,
    "shrines": shrines,
    "quest_items": quest_items,
}


def build_all(rules: Rules) -> dict[str, Record]:
    """Build every dataset keyed by name."""
    return {name: builder(rules) for name, builder in BUILDERS.items()}
