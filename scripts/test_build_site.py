"""Regression tests for the public site generator (scripts/build_site.py)."""

import copy
import json
import posixpath
import re
import shutil
import tempfile
import unittest
from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import urlsplit

import build_site
import site_combat
import site_context
import site_data
import site_data_play
import site_hood_walkthrough
import site_markdown
import site_pages_content
from gamefacts_public import JSONValue

ROOT = Path(__file__).resolve().parents[1]
KEY_PAGES = [
    "index.html",
    "404.html",
    "original/index.html",
    "original/bestiary/index.html",
    "original/equipment/index.html",
    "original/atlas/index.html",
    "original/levels/index.html",
    "original/camping/index.html",
    "original/spells/index.html",
    "original/shops/index.html",
    "original/commands/index.html",
    "original/how/levelling/index.html",
    "original/how/camping/index.html",
    "original/how/combat/index.html",
    "original/karma/index.html",
    "original/myths/index.html",
    "original/hood/index.html",
    "original/hood/graphics/index.html",
    "original/hood/saved-gam/index.html",
    "redux/index.html",
    "redux/differences/index.html",
    "redux/options/index.html",
    "data/index.html",
    "data/index.json",
    "assets/style.css",
    "assets/site.js",
]
PLAYER_PAGES = [
    "original/bestiary/index.html",
    "original/equipment/index.html",
    "original/atlas/index.html",
    "original/levels/index.html",
    "original/camping/index.html",
    "original/spells/index.html",
    "original/shops/index.html",
    "original/commands/index.html",
    "original/how/levelling/index.html",
    "original/how/camping/index.html",
    "original/how/combat/index.html",
    "original/karma/index.html",
]
HOOD_TOPICS = [
    "graphics",
    "saved-gam",
    "map-files",
    "text-files",
    "data-ovl",
    "combat-maps",
    "signs-and-fonts",
    "talk-system",
    "tiles",
]


class PageIndex(HTMLParser):
    """Collect the ids and local link targets of one HTML page."""

    def __init__(self) -> None:
        super().__init__()
        self.ids: set[str] = set()
        self.links: list[str] = []

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        for name, value in attrs:
            if name == "id" and value:
                self.ids.add(value)
            if name in {"href", "src"} and value:
                self.links.append(value)
            if name == "data-json" and value:
                self.links.append(value)


def read_json(path: Path) -> dict[str, JSONValue]:
    """Read a JSON object from disk."""
    data = json.loads(path.read_text(encoding="utf-8"))
    assert isinstance(data, dict)
    return data


def visible_text(document: str) -> str:
    """Strip tags, scripts and styles from a page."""
    document = re.sub(r"<(script|style)\b.*?</\1>", "", document, flags=re.DOTALL)
    return re.sub(r"<[^>]+>", " ", document)


def without_spoilers(document: str) -> str:
    """Remove every reveal element, leaving only text outside them."""
    return re.sub(
        r'<span class="spoiler".*?</span></span>', "", document, flags=re.DOTALL
    )


class SiteBuildCase(unittest.TestCase):
    """Shared fixture: build the whole site once per test class."""

    out: Path
    context: site_context.Context

    @classmethod
    def setUpClass(cls) -> None:
        directory = tempfile.mkdtemp()
        cls.addClassCleanup(shutil.rmtree, directory)
        cls.out = Path(directory) / "site"
        cls.context = build_site.build(ROOT, cls.out)

    def page(self, path: str) -> str:
        """Read one built page."""
        return (self.out / path).read_text(encoding="utf-8")


class SiteStructureTest(SiteBuildCase):
    """Pages, datasets and links."""

    def test_key_pages_exist(self) -> None:
        """The home page, both doors, every depth and the data index are built."""
        for path in KEY_PAGES:
            self.assertTrue((self.out / path).is_file(), path)
        for topic in HOOD_TOPICS:
            self.assertTrue((self.out / f"original/hood/{topic}/index.html").is_file())

    def test_dataset_counts_match_game_facts(self) -> None:
        """Creatures, equipment and locations have the published counts."""
        expected = {
            "creatures": 48,
            "equipment": 48,
            "locations": 40,
            "spells": 48,
            "reagents": 8,
            "shops": 46,
            "commands": 31,
        }
        index = read_json(self.out / "data/index.json")
        listed = {d["dataset"]: d for d in site_data.objects(index["datasets"])}
        for name, count in expected.items():
            data = read_json(self.out / "data" / f"{name}.json")
            self.assertEqual(len(site_data.objects(data["records"])), count, name)
            self.assertEqual(listed[name]["records"], count)
        for name, data in {
            n: read_json(self.out / "data" / f"{n}.json") for n in listed
        }.items():
            for key in ("dataset", "schema_version", "source", "records"):
                self.assertIn(key, data, f"{name} lacks {key}")
            for source in site_data.objects(data["source"]):
                self.assertIn(source["original"], ("Established", "Incomplete"))
                self.assertIn(source["rule"], self.context.rules)

    def test_creature_values(self) -> None:
        """Derived XP, ability names and missiles follow the Game Facts."""
        data = read_json(self.out / "data/creatures.json")
        rows = {r["name"]: r for r in site_data.objects(data["records"])}
        daemon = rows["Daemon"]
        self.assertEqual(site_data.mapping(daemon["derived"])["kill_xp"], 19)
        self.assertEqual(daemon["missile"], "red")
        self.assertIn("summons_daemons", site_data.strings(daemon["abilities"]))
        self.assertIsNone(rows["Orc"]["missile"], "range 1 is melee, not a bow")
        dragon = site_data.mapping(rows["Dragon"]["derived"])
        self.assertEqual(dragon["kill_xp"], 25)
        self.assertEqual(
            (dragon["chest_percent"], dragon["trapped_percent"]), (100, 98)
        )
        self.assertIsNone(site_data.mapping(daemon["derived"])["chest_percent"])
        self.assertIn("derived_from", data)

    def test_play_datasets(self) -> None:
        """Spells, reagents, shops and commands follow their Game Facts tables."""
        spells = read_json(self.out / "data/spells.json")
        rows = {r["name"]: r for r in site_data.objects(spells["records"])}
        self.assertEqual(rows["In Lor"]["reagents"], ["Sulfurous Ash"])
        self.assertEqual(rows["In Lor"]["contexts"], ["outdoors", "town", "dungeon"])
        self.assertEqual(site_data.mapping(rows["An Tym"]["derived"])["mp_cost"], 8)
        reagents = read_json(self.out / "data/reagents.json")
        ginseng = site_data.objects(reagents["records"])[1]
        sold = site_data.objects(site_data.mapping(ginseng["derived"])["sold_at"])
        self.assertEqual(len(sold), 3)
        shops = read_json(self.out / "data/shops.json")
        families: dict[str, int] = {}
        for shop in site_data.objects(shops["records"]):
            families[str(shop["family"])] = families.get(str(shop["family"]), 0) + 1
        self.assertEqual(
            families,
            {
                "arms": 9,
                "reagents": 5,
                "guild": 3,
                "healer": 7,
                "horse": 3,
                "ship": 4,
                "inn": 6,
                "tavern": 9,
            },
        )
        self.assertEqual(site_data_play.adjusted_buy(100, 10), 170)
        self.assertEqual(site_data_play.adjusted_buy(100, 30), 110)
        self.assertEqual(site_data_play.prices(20)["20"], 28)

    def test_internal_links_resolve(self) -> None:
        """Every relative link and fragment points at a built file and an id."""
        indexes: dict[str, PageIndex] = {}
        for path in sorted(self.out.rglob("*.html")):
            parser = PageIndex()
            parser.feed(path.read_text(encoding="utf-8"))
            indexes[path.relative_to(self.out).as_posix()] = parser
        self.assertGreater(len(indexes), 20)
        for page, parser in indexes.items():
            if page == "404.html":
                continue
            for link in parser.links:
                parts = urlsplit(link)
                if parts.scheme or parts.netloc:
                    continue
                self.assertFalse(link.startswith("/"), f"{page}: absolute {link}")
                target = posixpath.normpath(
                    posixpath.join(posixpath.dirname(page), parts.path)
                )
                if parts.path == "":
                    target = page
                elif parts.path.endswith("/") or (self.out / target).is_dir():
                    target = posixpath.join(target, "index.html")
                self.assertTrue((self.out / target).is_file(), f"{page}: {link}")
                if parts.fragment and target.endswith(".html"):
                    self.assertIn(
                        parts.fragment, indexes[target].ids, f"{page}: {link}"
                    )

    def test_facts_and_how_pages_hide_developer_pointers(self) -> None:
        """Implementation and Tests lines belong only under the hood."""
        for path in PLAYER_PAGES:
            text = visible_text(self.page(path))
            self.assertNotIn("Implementation", text, path)
            self.assertNotRegex(text, r"\bTests\b", path)
            self.assertNotIn("internal/", self.page(path), path)


class SiteContentTest(SiteBuildCase):
    """Page content, spoilers and curated-content guards."""

    def test_hood_pages_show_status_and_code_links(self) -> None:
        """Guides keep badges, rule IDs and links to Redux code."""
        text = self.page("original/hood/graphics/index.html")
        self.assertIn("GFX-0008", text)
        self.assertIn("badge established", text)
        self.assertIn("Original: Incomplete", text)
        self.assertIn(
            "https://github.com/bradhannah/UltimaVRedux/blob/main/internal/", text
        )
        saved = self.page("original/hood/saved-gam/index.html")
        self.assertIn("<h2", saved)
        self.assertNotIn("](", saved, "unrendered Markdown link")

    def test_graphics_walkthrough_precedes_rule_reference(self) -> None:
        """The decoding walkthrough's steps come first, then every rule in full."""
        text = self.page("original/hood/graphics/index.html")
        anchors = [
            'id="compressed"',
            'id="lzw"',
            'id="records"',
            'id="pixels"',
            'id="colours"',
            'id="tile-sheet"',
            'id="signature"',
            'id="not-yet"',
            'id="rule-reference"',
            'id="gfx-0001"',
        ]
        positions = [text.index(anchor) for anchor in anchors]
        self.assertEqual(positions, sorted(positions))
        self.assertIn("2 + 4 × 28", text)
        self.assertIn("<code>0x72</code>", text)
        self.assertEqual(text.count('class="swatch"'), 16)
        self.assertIn("high nibble = 10; right pixel = low nibble = 7", text)
        self.assertNotIn("](", text, "unrendered Markdown link")

    def test_walkthrough_offsets_must_match_the_rules(self) -> None:
        """A recomputed offset that a rule no longer states fails the build."""
        context = copy.deepcopy(self.context)
        rule = context.rules["GFX-0002"]
        rule["text"] = rule["text"].replace("`0x72`", "`0x74`")
        with self.assertRaisesRegex(ValueError, "GFX-0002 no longer states"):
            site_hood_walkthrough.archive_offsets(context)

    def test_combat_odds_must_match_the_rules(self) -> None:
        """Odds a rule no longer quotes, or a loot table missing an item, fail."""
        context = copy.deepcopy(self.context)
        rule = context.rules["EQUIP-0012"]
        rule["text"] = rule["text"].replace("51% at 15", "50% at 15")
        with self.assertRaisesRegex(ValueError, "EQUIP-0012 odds at 15"):
            site_combat.hit_table(context)
        rule = context.rules["CMD-0009"]
        rule["text"] = rule["text"].replace("| 28 | Magic Shield |\n", "")
        with self.assertRaisesRegex(ValueError, "every EQUIP-0001 item"):
            site_combat.chest_arms(context)

    def test_spoiler_answers_only_inside_reveal_elements(self) -> None:
        """Mantras, Words of Power and quest item positions are never in plain text."""
        answers: list[str] = []
        for row in site_data.objects(
            read_json(self.out / "data/shrines.json")["records"]
        ):
            answers.append(str(row["mantra"]))
        for row in site_data.objects(
            read_json(self.out / "data/locations.json")["records"]
        ):
            if row["word_of_power"]:
                answers.append(str(row["word_of_power"]))
        for row in site_data.objects(
            read_json(self.out / "data/quest_items.json")["records"]
        ):
            answers.append(f"{row['x']},{row['y']}")
        atlas = self.page("original/atlas/index.html")
        revealed = visible_text(atlas)
        stripped = visible_text(without_spoilers(atlas))
        for answer in answers:
            pattern = rf"(?<![\w,]){re.escape(answer)}(?![\w,])"
            self.assertRegex(revealed, pattern, f"{answer} missing from reveal")
            self.assertNotRegex(stripped, pattern, f"{answer} outside a reveal")
        self.assertEqual(atlas.count('class="spoiler"'), len(answers))
        for path in self.out.rglob("*.html"):
            relative = path.relative_to(self.out).as_posix()
            if relative == "original/atlas/index.html":
                continue
            text = visible_text(without_spoilers(path.read_text(encoding="utf-8")))
            for answer in answers:
                if len(answer) > 4:
                    self.assertNotIn(answer, text, f"{answer} leaks into {relative}")

    def test_spoiler_markup_works_without_javascript(self) -> None:
        """Answers start hidden, and a noscript rule reveals them without a script."""
        atlas = self.page("original/atlas/index.html")
        self.assertIn('<span class="answer" hidden>', atlas)
        self.assertIn("<noscript>", atlas)
        self.assertIn("data-hide-answers", atlas)

    def test_pages_work_without_javascript(self) -> None:
        """Tables are pre-rendered with every row present."""
        bestiary = self.page("original/bestiary/index.html")
        self.assertEqual(bestiary.count("<tr data-key="), 48)
        self.assertIn("data-json=", bestiary)
        self.assertNotIn('<tr data-key="0" hidden', bestiary)

    def test_bestiary_opens_on_monsters_with_everything_one_click_away(self) -> None:
        """The default group is Monsters; People, Fields and All remain buttons."""
        bestiary = self.page("original/bestiary/index.html")
        self.assertIn('data-default-group="monster"', bestiary)
        self.assertRegex(bestiary, r'data-group="monster" aria-pressed="true"')
        for group in ("person", "field", "all"):
            self.assertIn(f'data-group="{group}"', bestiary)

    def test_no_original_game_content(self) -> None:
        """Pages carry no image files or tile art."""
        files = [p.suffix for p in self.out.rglob("*") if p.is_file()]
        for suffix in (".png", ".gif", ".jpg", ".16", ".dat", ".ovl", ".gam"):
            self.assertNotIn(suffix, files)

    def test_options_come_from_the_dialog(self) -> None:
        """Every documented option label exists in the Configure dialog source."""
        text = self.page("redux/options/index.html")
        for label in ("Repeated Monster Division", "Combat Speed"):
            self.assertIn(label, text)

    def test_known_gaps_page_lists_curated_gaps(self) -> None:
        """The Redux page names each listed gap by rule ID."""
        text = self.page("redux/index.html")
        for identifier in ("COMBAT-0008", "MOON-0005", "SPELL-0005", "MIX-0003"):
            self.assertIn(identifier, text)
        self.assertNotIn("TALK-0002", text)

    def test_stale_known_gap_fails_the_build(self) -> None:
        """A curated gap whose rule is no longer a Known discrepancy is an error."""
        changed = copy.deepcopy(self.context)
        rule = changed.rules["COMBAT-0008"]
        rule["text"] = rule["text"].replace(
            "**Redux:** Known discrepancy", "**Redux:** Matches"
        )
        gaps = site_data.objects(changed.load("known_gaps.json")["gaps"])
        with self.assertRaisesRegex(ValueError, "COMBAT-0008"):
            site_pages_content.check_gaps(changed, gaps)

    def test_uncurated_known_gap_fails_the_build(self) -> None:
        """A new Known discrepancy must be added to the curated list."""
        gaps = site_data.objects(self.context.load("known_gaps.json")["gaps"])
        with self.assertRaisesRegex(ValueError, "missing from known_gaps"):
            site_pages_content.check_gaps(self.context, gaps[1:])

    def test_myths_rest_on_established_rules(self) -> None:
        """A myth cited against a non-Established rule fails the build."""
        myths = site_data.objects(self.context.load("myths.json")["myths"])
        site_pages_content.check_myths(self.context, myths)
        bad = [dict(myths[0], rules=["QUEST-0002"])]
        with self.assertRaisesRegex(ValueError, "not Established"):
            site_pages_content.check_myths(self.context, bad)

    def test_prepare_output_protects_other_directories(self) -> None:
        """Only a previous site build may be wiped."""
        with tempfile.TemporaryDirectory() as raw:
            target = Path(raw) / "keep"
            target.mkdir()
            (target / "note.txt").write_text("mine", encoding="utf-8")
            with self.assertRaises(ValueError):
                build_site.prepare_output(target)
            self.assertTrue((target / "note.txt").exists())

    def test_output_directory_is_ignored(self) -> None:
        """The default `make site` output directory is git-ignored."""
        ignored = (ROOT / ".gitignore").read_text(encoding="utf-8").splitlines()
        self.assertIn("/_site/", ignored)


class MarkdownTest(unittest.TestCase):
    """The small Markdown renderer covers the Game Facts subset."""

    @staticmethod
    def render(text: str) -> str:
        """Render with links left unchanged."""
        return site_markdown.render(text, lambda target: target)

    def test_blocks(self) -> None:
        """Headings, lists, tables and fenced code render."""
        html = self.render(
            "# Title\n\nText with `code`, **bold** and *italic*.\n\n"
            "- one\n- two\n  wrapped\n\n1. first\n2. second\n\n"
            "| A | B |\n| --- | ---: |\n| 1 | 2 |\n\n```text\nx < y\n```\n"
        )
        self.assertIn('<h1 id="title">Title</h1>', html)
        self.assertIn("<code>code</code>", html)
        self.assertIn("<strong>bold</strong>", html)
        self.assertIn("<em>italic</em>", html)
        self.assertIn("<li>two wrapped</li>", html)
        self.assertIn("<ol><li>first</li>", html)
        self.assertIn('<th class="num" scope="col">B</th>', html)
        self.assertIn("x &lt; y", html)

    def test_links_and_escaping(self) -> None:
        """Link targets pass through the resolver and text is escaped."""
        html = site_markdown.render(
            "See [GFX-0004](GRAPHICS_FORMATS.md#gfx-0004) <b>& more</b>.",
            lambda target: "/x/" + target,
        )
        self.assertIn('<a href="/x/GRAPHICS_FORMATS.md#gfx-0004">GFX-0004</a>', html)
        self.assertIn("&lt;b&gt;&amp; more&lt;/b&gt;", html)

    def test_pipes_inside_code_stay_in_the_cell(self) -> None:
        """A pipe inside inline code does not split a table cell."""
        self.assertEqual(site_markdown.split_row("| `a|b` | c |"), ["`a|b`", "c"])

    def test_multiplication_is_not_italics(self) -> None:
        """A lone asterisk between words is left alone."""
        html = self.render("Set HP to 30*level for each.")
        self.assertNotIn("<em>", html)


class LinkTest(unittest.TestCase):
    """Site-absolute links become relative to their page."""

    def test_relative(self) -> None:
        """Depth decides the number of parent segments."""
        relative = site_context.relative
        self.assertEqual(relative("/", "index.html"), "./")
        self.assertEqual(relative("/original/", "index.html"), "original/")
        self.assertEqual(relative("/", "original/hood/index.html"), "../../")
        self.assertEqual(
            relative("/assets/site.js", "original/hood/x/index.html"),
            "../../../assets/site.js",
        )
        self.assertEqual(
            relative("/redux/differences/#camp-0001", "original/camping/index.html"),
            "../../redux/differences/#camp-0001",
        )


if __name__ == "__main__":
    unittest.main()
