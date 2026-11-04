# Documentation Catalog

This catalog contains every retained Markdown document under `docs/` exactly
once. Each row has one semantic domain owner: the area responsible for maintaining
the contract, not an editor configuration or required tool. Cross-domain review
still applies. Start with [installation](INSTALL.md) for player setup or
[development](DEVELOPMENT_GUIDE.md) for contributor tools and workflows.

## Authority

Resolve factual conflicts within the relevant scope:

1. Executable Redux code and meaningful tests establish current implementation.
2. Game Facts are the maintained public explanation of original behavior and
   compatibility formats, with stable IDs, explicit scope, uncertainty, and Redux
   differences. A Redux test is not independent proof of original behavior.
3. ADRs record accepted design decisions, alternatives, and rationale.
4. System documentation defines current architecture and integration contracts.
5. General policy governs repository practice; procedures explain its application.
6. GitHub Issues own reports and incomplete work. An index only routes readers.

For original-game claims, relevant evidence in its exact edition and execution
context takes precedence over a summary. Rule assessment records the limits of
that interpretation; an isolated declaration or apparent defect is not proof of
shipped behavior.

Preserve detailed conditions, ordering, failure behavior, ownership, and cleanup
when consolidating documentation. Do not replace durable contracts with code-link
stubs or disguise unknown behavior as an intentional deviation. Use canonical
Game Facts rule IDs rather than fragile descriptive-heading references.

## General And Standards

| Document | One-line purpose | Primary owner | Authority classification |
|---|---|---|---|
| [`README.md`](README.md) | Catalog every retained documentation file and its ownership. | `documentation` | Index |
| [`INSTALL.md`](INSTALL.md) | Explain beginner Go setup, source installation, portable binaries, launch-path resolution, original-data setup, and platform limits. | `installation` | Procedure |
| [`ARCHITECTURE_GUIDELINES.md`](ARCHITECTURE_GUIDELINES.md) | Define package boundaries and cross-system architecture guardrails. | `architecture` | General policy |
| [`brand-guidelines.md`](brand-guidelines.md) | Define visual identity and brand usage. | `visual-design` | General policy |
| [`CODING_CONVENTIONS.md`](CODING_CONVENTIONS.md) | Define Go/Python style, typing, determinism, error, testing, and UI conventions. | `engineering` | General policy |
| [`DEVELOPMENT_GUIDE.md`](DEVELOPMENT_GUIDE.md) | Guide contributor setup, implementation workflows, and local cross-platform release packaging. | `development` | Procedure |
| [`ERROR_HANDLING.md`](ERROR_HANDLING.md) | Define panic, fatal, returned-error, and callback failure policy. | `engineering` | General policy |
| [`PROFILING.md`](PROFILING.md) | Define profile-first CPU, memory, and allocation investigation. | `performance` | Procedure |
| [`SITE.md`](SITE.md) | Explain how the public GitHub Pages site is generated, previewed, deployed, and extended with pages and datasets. | `public-site` | Procedure |
| [`TESTING.md`](TESTING.md) | Define test strategy, fixture recipes, suites, and regression review. | `testing` | General policy |

## Architecture Decisions

| Document | One-line purpose | Primary owner | Authority classification |
|---|---|---|---|
| [`ADR/ADR-001-central-gameclock.md`](ADR/ADR-001-central-gameclock.md) | Explain adapter-owned elapsed time and controlled gameplay randomness. | `time` | ADR rationale |
| [`ADR/ADR-002-displaymanager-pattern.md`](ADR/ADR-002-displaymanager-pattern.md) | Explain centralized display operations behind DisplayManager. | `display` | ADR rationale |
| [`ADR/ADR-003-systemcallbacks-decoupling.md`](ADR/ADR-003-systemcallbacks-decoupling.md) | Explain validated callbacks between gameplay and presentation. | `architecture` | ADR rationale |
| [`ADR/ADR-004-integration-testing-real-data.md`](ADR/ADR-004-integration-testing-real-data.md) | Record the decision to test integrations with real Ultima V data. | `testing` | ADR rationale |
| [`ADR/ADR-005-error-handling-strategy.md`](ADR/ADR-005-error-handling-strategy.md) | Record fatal-versus-recoverable error handling rationale. | `engineering` | ADR rationale |
| [`ADR/ADR-006-ui-widget-system-enhancements.md`](ADR/ADR-006-ui-widget-system-enhancements.md) | Record widget lifecycle, layout, and UI-boundary decisions. | `ui` | ADR rationale |
| [`ADR/ADR-007-audio-system-architecture.md`](ADR/ADR-007-audio-system-architecture.md) | Record audio lifecycle and integration architecture. | `audio` | ADR rationale |
| [`ADR/ADR-008-event-queue-execution-lanes.md`](ADR/ADR-008-event-queue-execution-lanes.md) | Record separate workflow and presentation-feedback queue ownership. | `workflows` | ADR rationale |
| [`ADR/ADR-009-canonical-save-snapshot.md`](ADR/ADR-009-canonical-save-snapshot.md) | Record canonical native snapshot and hydration boundaries. | `saves` | ADR rationale |
| [`ADR/ADR-010-reusable-audio-module.md`](ADR/ADR-010-reusable-audio-module.md) | Explain Berad Audio extraction, SDK migration, and pinned consumer integration. | `audio` | ADR rationale |
| [`ADR/ADR-011-berad-text.md`](ADR/ADR-011-berad-text.md) | Explain SDK-owned typography/rendering with game-owned text and input policy. | `text` | ADR rationale |
| [`ADR/ADR-012-berad-graphics.md`](ADR/ADR-012-berad-graphics.md) | Explain SDK image/resource mechanics with game-owned artwork, presentation and lifetime adapters. | `rendering` | ADR rationale |
| [`ADR/ADR-013-berad-rng-input.md`](ADR/ADR-013-berad-rng-input.md) | Explain exact RNG continuation and shared keyboard mechanisms with game-owned policies. | `architecture` | ADR rationale |

## Current Systems

| Document | One-line purpose | Primary owner | Authority classification |
|---|---|---|---|
| [`systems/AUDIO_SYSTEM.md`](systems/AUDIO_SYSTEM.md) | Define Redux integration, policy, assets and acceptance over Berad Audio. | `audio` | System documentation |
| [`systems/CAMPING_SYSTEM.md`](systems/CAMPING_SYSTEM.md) | Describe camping entry, orchestration, encounters, and cleanup. | `camping` | System documentation |
| [`systems/DEBUG_WORKBENCH.md`](systems/DEBUG_WORKBENCH.md) | Define control-plane commands, snapshots, and scenarios. | `debug-workbench` | System documentation |
| [`systems/EVENT_QUEUE_SYSTEM.md`](systems/EVENT_QUEUE_SYSTEM.md) | Define workflow, feedback, composition, priority, and input ownership. | `workflows` | System documentation |
| [`systems/INPUT_HANDLING.md`](systems/INPUT_HANDLING.md) | Define command input routing and primary/secondary state transitions. | `input` | System documentation |
| [`systems/LINEAR_CONVERSATION_SYSTEM.md`](systems/LINEAR_CONVERSATION_SYSTEM.md) | Describe semantic import, catalog, runtime, patches, and UI boundaries. | `conversations` | System documentation |
| [`systems/LOGGING_SYSTEM.md`](systems/LOGGING_SYSTEM.md) | Define logging categories, levels, filtering, and runtime controls. | `logging` | System documentation |
| [`systems/MESSAGE_OUTPUT_SYSTEM.md`](systems/MESSAGE_OUTPUT_SYSTEM.md) | Define message callbacks, text ownership, and UI adaptation. | `text` | System documentation |
| [`systems/MOONGATE_SYSTEM.md`](systems/MOONGATE_SYSTEM.md) | Describe gate state, rendering, travel workflow, and audio. | `moongates` | System documentation |
| [`systems/SPRITE_OUTLINE_SYSTEM.md`](systems/SPRITE_OUTLINE_SYSTEM.md) | Describe outline generation, caching, rendering, and use. | `rendering` | System documentation |
| [`systems/TIME_SYSTEM.md`](systems/TIME_SYSTEM.md) | Describe elapsed time, calendar, turns, daylight, and lunar state. | `time` | System documentation |
| [`systems/VISUAL_EFFECTS_SYSTEM.md`](systems/VISUAL_EFFECTS_SYSTEM.md) | Define shared flash, shake, inversion, dissolve, mask, and projectile effects. | `visual-effects` | System documentation |

## UI Systems

| Document | One-line purpose | Primary owner | Authority classification |
|---|---|---|---|
| [`ui/DEBUG_WORKBENCH_UI.md`](ui/DEBUG_WORKBENCH_UI.md) | Specify Workbench layout, instruments, interactions, and accessibility. | `debug-workbench` | System documentation |
| [`ui/GAME_UI_INTEGRATION.md`](ui/GAME_UI_INTEGRATION.md) | Describe GameScene widget wiring, callbacks, update, and rendering. | `ui` | System documentation |
| [`ui/MODAL_DIALOG_SYSTEM.md`](ui/MODAL_DIALOG_SYSTEM.md) | Define dialog selection, construction, input ownership, and cleanup. | `ui` | System documentation |
| [`ui/UI_WIDGET_SYSTEM.md`](ui/UI_WIDGET_SYSTEM.md) | Define widget boundaries, interfaces, lifecycle, and composition. | `ui` | System documentation |
| [`ui/WIDGET_COMPONENTS.md`](ui/WIDGET_COMPONENTS.md) | Select reusable widgets and preserve domain integration contracts. | `ui` | System documentation |

## Detailed References

| Document | One-line purpose | Primary owner | Authority classification |
|---|---|---|---|
| [`details/LEGACY_TO_NATIVE_SAVE_CROSSWALK.md`](details/LEGACY_TO_NATIVE_SAVE_CROSSWALK.md) | Record every decoded legacy field disposition and object-list conversion rule. | `saves` | System documentation |
| [`details/SAVE_SNAPSHOT_SCHEMA.md`](details/SAVE_SNAPSHOT_SCHEMA.md) | Specify native fields, validation, eligibility, repositories, and hydration. | `saves` | System documentation |
| [`details/SAVE_CONVERTER.md`](details/SAVE_CONVERTER.md) | Convert an exact original save pair to a standalone native snapshot or player slot and load the result. | `saves` | Procedure |
| [`details/search_locations.md`](details/search_locations.md) | Define fixed Search records, reward decoding, and native persistence. | `exploration` | System documentation |

## Original Gamefacts

The 42 topics below retain canonical rule IDs. The index and conventions are
separate catalog entries; they are not additional behavioral topics.

| Document | One-line purpose | Primary owner | Authority classification |
|---|---|---|---|
| [`gamefacts/README.md`](gamefacts/README.md) | Navigate the 42 behavioral and compatibility topics. | `game-facts` | Index |
| [`gamefacts/CONVENTIONS.md`](gamefacts/CONVENTIONS.md) | Define stable IDs, rule statuses, pseudocode, and relationships. | `game-facts` | General policy |
| [`gamefacts/ASTRONOMY.md`](gamefacts/ASTRONOMY.md) | Record instrument gates, star rendering, planets, and comets. | `astronomy` | Gamefact |
| [`gamefacts/BRIT_CBT_FORMAT.md`](gamefacts/BRIT_CBT_FORMAT.md) | Specify combat-map records, formations, and selection. | `combat` | Gamefact |
| [`gamefacts/CAMPING_MECHANICS.md`](gamefacts/CAMPING_MECHANICS.md) | Record camping timing, rest, healing, and encounters. | `camping` | Gamefact |
| [`gamefacts/CHARACTER_CREATION.md`](gamefacts/CHARACTER_CREATION.md) | Record virtue questions, Avatar generation, and initial state. | `characters` | Gamefact |
| [`gamefacts/CHARACTER_PROGRESSION.md`](gamefacts/CHARACTER_PROGRESSION.md) | Record attributes, experience, levels, and advancement. | `characters` | Gamefact |
| [`gamefacts/COMBAT.md`](gamefacts/COMBAT.md) | Record combat flow, formulas, AI, status, and Redux differences. | `combat` | Gamefact |
| [`gamefacts/COMMANDS_REFERENCE.md`](gamefacts/COMMANDS_REFERENCE.md) | Catalog command behavior, completion, doors, and chests. | `commands` | Gamefact |
| [`gamefacts/CRIME_PRISON.md`](gamefacts/CRIME_PRISON.md) | Record crime, guards, arrest, interrogation, and rescue. | `crime` | Gamefact |
| [`gamefacts/DATA_OVL_LAYOUT.md`](gamefacts/DATA_OVL_LAYOUT.md) | Specify DATA.OVL conventions and index its identified tables. | `data-formats` | Gamefact |
| [`gamefacts/DEATH_RESURRECTION.md`](gamefacts/DEATH_RESURRECTION.md) | Record death, revival, and party recovery. | `characters` | Gamefact |
| [`gamefacts/DUNGEON_SYSTEM.md`](gamefacts/DUNGEON_SYSTEM.md) | Record dungeon layout, movement, rooms, hazards, and transitions. | `dungeons` | Gamefact |
| [`gamefacts/ENEMIES.md`](gamefacts/ENEMIES.md) | Catalog enemy statistics, abilities, groups, and behavior. | `enemies` | Gamefact |
| [`gamefacts/ENVIRONMENTAL_HAZARDS.md`](gamefacts/ENVIRONMENTAL_HAZARDS.md) | Record terrain hazards, damage, poison, and protection. | `exploration` | Gamefact |
| [`gamefacts/EQUIPMENT_ORIGINAL_DATA.md`](gamefacts/EQUIPMENT_ORIGINAL_DATA.md) | Specify canonical equipment identities and reference ratings. | `equipment` | Gamefact |
| [`gamefacts/EQUIPMENT_STATS.md`](gamefacts/EQUIPMENT_STATS.md) | Record Ready, attacks, ammunition, armour, accessories, and prices. | `equipment` | Gamefact |
| [`gamefacts/GRAPHICS_FORMATS.md`](gamefacts/GRAPHICS_FORMATS.md) | Specify archives, compression, pixels, masks, fonts, and extraction. | `graphics-formats` | Gamefact |
| [`gamefacts/INVENTORY_SYSTEM.md`](gamefacts/INVENTORY_SYSTEM.md) | Record ownership, categories, limits, preparation, and consumption. | `inventory` | Gamefact |
| [`gamefacts/KARMA.md`](gamefacts/KARMA.md) | Record karma changes, thresholds, and consequences. | `karma` | Gamefact |
| [`gamefacts/LIGHTING.md`](gamefacts/LIGHTING.md) | Record sight, illumination, darkness, and Redux tint contracts. | `lighting` | Gamefact |
| [`gamefacts/MAGIC_SPELLS.md`](gamefacts/MAGIC_SPELLS.md) | Catalog casting order, all 48 effects, resolution, and Redux gaps. | `magic` | Gamefact |
| [`gamefacts/MAP_FILE_FORMATS.md`](gamefacts/MAP_FILE_FORMATS.md) | Specify world and settlement map files, location numbers, and entrances. | `maps` | Gamefact |
| [`gamefacts/MERCHANT_SERVICES.md`](gamefacts/MERCHANT_SERVICES.md) | Record merchant catalogs, pricing, services, and transaction timing. | `merchants` | Gamefact |
| [`gamefacts/MOONSTONE_LOCATIONS.md`](gamefacts/MOONSTONE_LOCATIONS.md) | Record placements, phases, gate routes, and destination knowledge. | `moongates` | Gamefact |
| [`gamefacts/NPC_SCHEDULES.md`](gamefacts/NPC_SCHEDULES.md) | Specify schedule records, selection, arrival, and availability. | `npcs` | Gamefact |
| [`gamefacts/POTIONS_SCROLLS.md`](gamefacts/POTIONS_SCROLLS.md) | Record potion/scroll identities, effects, and consumption. | `magic` | Gamefact |
| [`gamefacts/PROVISIONS_CONSUMPTION.md`](gamefacts/PROVISIONS_CONSUMPTION.md) | Record food, torches, keys, gems, and skull-key consumption. | `inventory` | Gamefact |
| [`gamefacts/QUEST_ITEMS_LOCATIONS.md`](gamefacts/QUEST_ITEMS_LOCATIONS.md) | Catalog quest-item locations, requirements, and uses. | `quests` | Gamefact |
| [`gamefacts/REAGENT_MIXING.md`](gamefacts/REAGENT_MIXING.md) | Record reagent encodings, recipes, mixing, and preparation. | `magic` | Gamefact |
| [`gamefacts/SAVED_GAM_STRUCTURE.md`](gamefacts/SAVED_GAM_STRUCTURE.md) | Specify legacy save allocation, offsets, and object-list authority. | `saves` | Gamefact |
| [`gamefacts/SHADOWLORDS.md`](gamefacts/SHADOWLORDS.md) | Record Shadowlord assignment, influence, summons, and destruction. | `shadowlords` | Gamefact |
| [`gamefacts/SHIP_MECHANICS.md`](gamefacts/SHIP_MECHANICS.md) | Record sails, wind, hull, sinking, repair, and cannon fire. | `vehicles` | Gamefact |
| [`gamefacts/SHRINE_SYSTEM.md`](gamefacts/SHRINE_SYSTEM.md) | Record meditation, quests, rewards, and destruction state. | `quests` | Gamefact |
| [`gamefacts/SIGNS_AND_FONTS.md`](gamefacts/SIGNS_AND_FONTS.md) | Specify sign records, font selection, glyph uses, and symbolic displays. | `text-formats` | Gamefact |
| [`gamefacts/STARTUP_MENU_AND_INTRODUCTIONS.md`](gamefacts/STARTUP_MENU_AND_INTRODUCTIONS.md) | Record startup, menu, credits, Introduction, and attract sequence. | `startup` | Gamefact |
| [`gamefacts/TALK_SYSTEM_STRUCTURE.md`](gamefacts/TALK_SYSTEM_STRUCTURE.md) | Specify conversation records, controls, matching, and runtime boundaries. | `conversations` | Gamefact |
| [`gamefacts/TEXT_FILE_FORMATS.md`](gamefacts/TEXT_FILE_FORMATS.md) | Specify Look, merchant-phrase, and ending text files. | `text-formats` | Gamefact |
| [`gamefacts/TILES.md`](gamefacts/TILES.md) | Catalog tile identity, properties, animation, and passability. | `maps` | Gamefact |
| [`gamefacts/TIME_COSTS.md`](gamefacts/TIME_COSTS.md) | Record calendar modifiers, context ownership, actor cadence, and special loops. | `time` | Gamefact |
| [`gamefacts/VEHICLES.md`](gamefacts/VEHICLES.md) | Record traversal, boarding, Exit, ownership, and state. | `vehicles` | Gamefact |
| [`gamefacts/VIEW_SYSTEM.md`](gamefacts/VIEW_SYSTEM.md) | Record scrying, map geometry, coordinates, and atlas authority. | `exploration` | Gamefact |
| [`gamefacts/WELLS_FOUNTAINS.md`](gamefacts/WELLS_FOUNTAINS.md) | Record wishing and drinking outcomes, odds, and conditions. | `exploration` | Gamefact |
| [`gamefacts/WORDS_OF_POWER.md`](gamefacts/WORDS_OF_POWER.md) | Record words, matching, entrances, earned clues, and Codex acknowledgement. | `quests` | Gamefact |

## Test Procedures

| Document | One-line purpose | Primary owner | Authority classification |
|---|---|---|---|
| [`testing/SHADOWLORD_MANUAL_TESTS.md`](testing/SHADOWLORD_MANUAL_TESTS.md) | Provide reproducible manual Workbench checks for Shadowlord behavior. | `testing` | Procedure |

The versioned JSON files under `schemas/` are native-save support assets owned
by the saves domain. Snapshot version 25 is current within envelope 2; version 24
is the only frozen promotion source. Frozen schemas V16–V24 remain because the
current schema reuses their definitions locally. See the snapshot contract for
executable schema and promotion boundaries.
