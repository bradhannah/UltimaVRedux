# Game Facts

Technical descriptions of original game behavior, compatibility formats, and
Redux handling. Rules use stable identifiers so reports and implementation can
refer to a specific condition rather than an entire subsystem.

Implementation and test paths resolve within this application source tree.
Original-behavior assessments remain scoped to their stated evidence and limits;
an available implementation is not proof of cross-version gameplay equivalence.
Release availability is described in the repository's installation guidance.

| Topic | Scope |
| --- | --- |
| [Karma](KARMA.md) | Party karma, moral events, dialogue, recovery, and reports. |
| [Legacy saves](SAVED_GAM_STRUCTURE.md) | Saved allocation, roster, inventory, effects, and object-list ownership. |
| [Combat records](BRIT_CBT_FORMAT.md) | Battlefield storage, formations, selection, and dungeon-only metadata. |
| [Map files](MAP_FILE_FORMATS.md) | World chunks, settlement floors, location numbers, entrances, and fixed scenes. |
| [Equipment data](EQUIPMENT_ORIGINAL_DATA.md) | Canonical equipment identities and semantic reference ratings. |
| [Equipment mechanics](EQUIPMENT_STATS.md) | Ready, attacks, commitment, special equipment, and pricing. |
| [Inventory](INVENTORY_SYSTEM.md) | Ownership categories, capacities, and resource commitment. |
| [Graphics formats](GRAPHICS_FORMATS.md) | Archive layouts, decoder assumptions, fonts, and signature paths. |
| [DATA.OVL layout](DATA_OVL_LAYOUT.md) | Initialized-data conventions, pointer bases, and an index of identified tables. |
| [Signs and fonts](SIGNS_AND_FONTS.md) | Text/control semantics and symbolic displays. |
| [Text files](TEXT_FILE_FORMATS.md) | Look descriptions, merchant phrases, and ending text storage. |
| [Tiles](TILES.md) | Contextual identity, movement, occupancy, animation, and presentation. |
| [Time costs](TIME_COSTS.md) | Calendar modifiers, context ownership, actor cadence, slow steps, and special loops. |
| [Provisions](PROVISIONS_CONSUMPTION.md) | Poison/meal ordering, observed hours, fuel, and key/gem commitment. |
| [Camping and rest](CAMPING_MECHANICS.md) | Guard/ambush ordering, duration, rewards, beds, and paid inn rest. |
| [Astronomy](ASTRONOMY.md) | Sky inheritance, instrument gates, planet tracks, stars/comets, and flexible navigation. |
| [Moonstones](MOONSTONE_LOCATIONS.md) | Placements, phases, gate stages/routes, and earned destination knowledge. |
| [Lighting](LIGHTING.md) | Original sight/illumination and Redux tint, daylight, and opening contracts. |
| [Dungeons](DUNGEON_SYSTEM.md) | Packed cells, exploration, encounters, room records, triggers, and finale boundaries. |
| [Hazards](ENVIRONMENTAL_HAZARDS.md) | Ordered party damage, transformed d30, terrain gates, falls, and tolls. |
| [Frigates](SHIP_MECHANICS.md) | Sails, waits, hull damage, repair, cannon fire, and wind predicates. |
| [Vehicles](VEHICLES.md) | Terrain/occupancy, boarding, Exit, and carpet ownership. |
| [View and atlas](VIEW_SYSTEM.md) | Paid observation, map geometry, crystal risk, and earned atlas authority. |
| [Wells and fountains](WELLS_FOUNTAINS.md) | Wish commitment/matching/placement and context-specific drinking effects. |
| [Creation](CHARACTER_CREATION.md) | Template/input/save construction, virtue tournament, generator state, and opening limits. |
| [Progression](CHARACTER_PROGRESSION.md) | XP-derived levels, kill attribution, Wanderer restoration, and stat awards. |
| [Recovery](DEATH_RESURRECTION.md) | Individual revival, caller commitment, and full-party destination/time restoration. |
| [Quest items](QUEST_ITEMS_LOCATIONS.md) | Storage, evidenced placements, acquisition gaps, and artifact competition. |
| [Shadowlords](SHADOWLORDS.md) | Daily assignment, town influence, theft, summons, and destruction rituals. |
| [Shrines](SHRINE_SYSTEM.md) | Meditation/restoration input, quest state, Codex selection, and guardian timing. |
| [Startup and introductions](STARTUP_MENU_AND_INTRODUCTIONS.md) | Menu, curated credits, illustrated frames, and attract interpreter. |
| [Words of Power](WORDS_OF_POWER.md) | Canonical words, matching/dispatch, earned clues, and Codex acknowledgement. |
| [NPC schedules](NPC_SCHEDULES.md) | Schedule bytes, settled actions, planning, contact, and authored-actor limits. |
| [Conversations](TALK_SYSTEM_STRUCTURE.md) | TLK structure, interpreter controls, matching, and semantic runtime boundaries. |
| [Merchants](MERCHANT_SERVICES.md) | Catalogs, pricing, transactions, service families, and batch cadence. |
| [Commands](COMMANDS_REFERENCE.md) | Context dispatch, completion, doors, chests, and input contracts. |
| [Crime and prison](CRIME_PRISON.md) | Tribute, guards, capture, murder, persistence, and prisoner rescue. |
| [Combat](COMBAT.md) | Scheduler, attack resolution, statuses, AI, loot, and encounter completion. |
| [Enemies](ENEMIES.md) | Complete creature catalog, abilities, admission, generation, and world continuation. |
| [Spells](MAGIC_SPELLS.md) | Prepared casting, all 48 effects/contexts, targeting, and commitment. |
| [Potions and scrolls](POTIONS_SCROLLS.md) | Item-specific consumption, substitution, effects, and duration ownership. |
| [Mixing](REAGENT_MIXING.md) | Reagent encodings, all recipes, commitment, failure traps, and learned formulas. |
| [Conventions](CONVENTIONS.md) | Rule IDs, statuses, pseudocode, and groups. |

Report suspected errors through the [issue-first contribution process](../../CONTRIBUTING.md).
Include the rule ID and the circumstances that distinguish the behaviors.
