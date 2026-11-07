# The public site

The public GitHub Pages site is generated, not hand-written. The public
[Game Facts](gamefacts/README.md) are its only source of facts. Curated prose
lives in a few JSON files and cites rule IDs; the build checks those citations
against the Game Facts, so stale content fails instead of shipping.

## Publishing

GitHub Pages serves the `gh-pages` branch (**Settings, Pages, Build and
deployment: Deploy from a branch, `gh-pages`, `/ (root)`**). That branch holds
only built output; never edit it by hand.

Publish by hand from an up-to-date `main` checkout, which is the normal path:

```sh
make site-publish   # builds, commits to gh-pages, pushes (needs push access)
```

[`scripts/publish_site.py`](../scripts/publish_site.py) refuses uncommitted
changes under `site/`, `docs/gamefacts/` and `scripts/`, builds into a temporary
directory, and records the result as the next `gh-pages` commit ("Publish site
from <commit>") through a private Git index. It does not touch your working tree,
index or current branch. An unchanged site pushes nothing. The push only
fast-forwards, so if someone published in the meantime it fails and you run it
again.

`.github/workflows/pages.yml` runs the same script on every push to `main` when
GitHub Actions is available. Nothing depends on it: when Actions does not run,
publish by hand.

To preview locally:

```sh
make site                        # builds into the git-ignored _site/
python3 -m http.server -d _site  # then open http://localhost:8000/
```

`make site` needs the Python tools from `make python-tools` only to find a Python
3.12+ interpreter. The generator itself uses the standard library.
[`scripts/test_build_site.py`](../scripts/test_build_site.py) runs with
`make tooling-test`. It builds into a temporary directory and checks the key
pages, dataset counts, internal links, spoiler placement and the absence of
developer pointers on player pages.

## Structure

| Part | Where | Notes |
| --- | --- | --- |
| The original game | `/original/` | Three depths and Myths, tested. |
| Facts and figures | `/original/<page>/` | Tables and short answers. No formulas, no code paths. |
| How it works | `/original/how/<page>/` | Step by step, with exact odds and rule status. |
| Under the hood | `/original/hood/` | Format guides rendered from Game Facts, plus the file catalog. These pages show Implementation and Tests links. |
| Ultima V Redux | `/redux/` | Get started, differences, options, known gaps and credits. |
| Data | `/data/` | JSON datasets and `index.json`. |

Rules for every page:

- Never publish original game text, maps, art, music or code. Names and numbers
  from Game Facts tables are fine. There is no tile art.
- Facts and How-it-works pages show no Implementation or Tests lines.
- Puzzle answers (mantras, quest item positions, Words of Power) appear only
  inside reveal elements, hidden by default. The reader's "Hide puzzle answers"
  choice is kept in `localStorage`, guarded by `try/catch`.
- Pages work without JavaScript. Scripts only add sorting, filtering and reveal.
- Never claim ownership of the Ultima trademark. The fan-project notice is the
  `fan_notice` and Trademarks entries in `site/content/redux.json`.

## Source layout

| Path | Purpose |
| --- | --- |
| [`scripts/build_site.py`](../scripts/build_site.py) | Entry point: `python scripts/build_site.py OUTPUT_DIR`. |
| [`scripts/publish_site.py`](../scripts/publish_site.py) | Builds and pushes to `gh-pages` (`make site-publish`); tested by `scripts/test_publish_site.py`. |
| `scripts/site_data.py`, `scripts/site_data_play.py` | Build datasets from Game Facts tables. |
| `scripts/site_markdown.py` | Small Markdown renderer for the Game Facts subset. |
| `scripts/site_context.py` | Shared state, layout and HTML helpers. |
| `scripts/site_pages_*.py` | Page builders: data pages and Atlas, curated pages, Under the hood, landing pages. |
| `site/content/` | Curated JSON content (below). |
| `site/static/` | `style.css` and `site.js`, copied to `/assets/`. |

## Adding or changing content

**A facts or how-it-works page.** Add `site/content/pages/<name>.json` with `slug`,
`tier` (`facts` or `how`), `title`, `summary`, `lede`, `rules` (every cited rule
ID) and `blocks`. Block types are `p`, `h2`, `ul`, `steps`, `stats`, `callout`,
`table` (`head` and `rows`; cells may hold inline Markdown), `myths` and
`computed`; an `h2` may set an `id` for links. A `computed` block renders
a table calculated from the Game Facts (`level_table`, `ambush_table`); register new
ones in `scripts/site_pages_content.py`. The combat page's `hit_table`,
`fight_example` (its creature, weapon, `dex` and `armour` are block fields),
`chest_contents` and `chest_arms` live in
[`scripts/site_combat.py`](../scripts/site_combat.py). Verify every number against
the cited rule.
Links inside content use site-absolute paths such as `/original/levels/`.

**A walkthrough for an Under the hood guide.** Add `site/content/hood/<slug>.json`,
named after the guide's slug in `scripts/site_pages_hood.py`. It holds `intro`,
`code` (Redux source paths, which must exist), a `pipeline` of steps,
`pipeline_notes`, `sections` and `not_yet`. Each section has an `id`, an optional
`step` number, a `title`, `rules` and `blocks`. A section may cite only rules from
its own guide, and each cited rule's Original status is shown beside it. On top of
the shared block types, walkthroughs add `chips`, `code`, `bits` (an
example byte, decoded at build time) and `bitfield`. Computed blocks are
`archive_offsets`, recomputed and checked against the rule text, and
`ega_palette`, read from `pkg/graphics/legacy/model.go`. They live in
[`scripts/site_hood_walkthrough.py`](../scripts/site_hood_walkthrough.py). The
walkthrough renders above the guide's full rule reference;
`site/content/hood/graphics.json` is the example.

**A myth.** Add an object to `site/content/myths.json`. Its verdict is `busted`,
`confirmed`, `depends` or `unknown`, and every rule it cites must be marked
Original: Established, or the build fails.

**A Redux difference.** Add an object to `site/content/differences.json` citing one
rule that records a deviation (`REDUX DEVIATION:` marker or Redux: Intentional
deviation). Name the Configure option in `setting` when one restores the original.

**A known gap.** `site/content/known_gaps.json` is keyed by rule ID. The build fails
if a listed rule is no longer Redux: Known discrepancy, and also if a Known
discrepancy rule is missing from the file. Set `listed` to `false` to keep a gap
on the developer notes only.

**An option.** Edit `site/content/redux.json`. Each label is checked against
`gameplaySettings` in `cmd/ultimav/audio_settings_dialog_rendering.go`.

**A file in the Under the hood catalog.** Edit `site/content/file_catalog.json`.
Status is derived from the listed rules: all Established is Documented, anything
Incomplete or Conflicting is Partly, no rules is Not yet.

**A format guide.** Add a `Topic` to `TOPICS` in `scripts/site_pages_hood.py`. The
topic must be in the Game Facts catalog.

**A dataset.** Write a builder in `scripts/site_data.py` and register it in
`BUILDERS`. Every dataset has `dataset`, `schema_version`, `source` (rule IDs with
their Original status) and `records`. Values computed from other rules go in each
record's `derived` block, and `derived_from` names the rule. List puzzle-answer
fields in `spoiler_fields`. To show it as a sortable table, render the rows in a
page builder with `data-key` (and optionally `data-group`) on each row and
`data-json` on the table, as `site_pages_data.py` does for the Bestiary.

## Datasets

| File | Rules | Records |
| --- | --- | --- |
| `creatures.json` | ENEMY-0001, ENEMY-0002, PROGRESS-0002, COMBAT-0009 | 48 |
| `equipment.json` | EQUIP-0001 | 48 |
| `locations.json` | MAP-0003, MAP-0004, WORD-0001 | 40 |
| `moon_phases.json` | MOON-0002 | 28 |
| `moonstones.json` | MOON-0001 | 8 |
| `shrines.json` | SHRINE-0001 | 8 |
| `quest_items.json` | QUEST-0002 | 4 |
| `spells.json` | SPELL-0002, MIX-0001, SPELL-0001 | 48 |
| `reagents.json` | MIX-0001, MERCH-0005 | 8 |
| `shops.json` | MERCH-0001 to MERCH-0012, EQUIP-0001 | 46 |
| `commands.json` | CMD-0001 | 31 |

The datasets contain the puzzle answers in plain JSON, flagged by
`spoiler_fields`. Only the rendered pages hide them.
