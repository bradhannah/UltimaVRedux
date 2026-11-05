import type { NavigationWorkspace } from "./command_navigation"

export type TaskID = NavigationWorkspace
export type ToolGroup = "World extras" | "For developers"

export interface TaskDefinition {
  id: TaskID
  label: string
  blurb: string
  /** SVG path data (24x24 viewBox, stroked) for the Home card icon. */
  icon: string[]
}

export interface ToolDefinition {
  task: TaskID
  /** More page only. */
  group?: ToolGroup
  /** Defaults to the catalog command name. */
  name?: string
  /** Defaults to the catalog command description. */
  description?: string
}

/** Task tabs in display order; Home is not a task. */
export const taskDefinitions: TaskDefinition[] = [
  { id: "goto", label: "Go to", blurb: "Landmarks, shrines and any map position.", icon: ["M5 12h14M13 6l6 6-6 6"] },
  { id: "party", label: "Party", blurb: "Members, health, levels and equipment.", icon: ["M9 11a3 3 0 1 0 0-6 3 3 0 0 0 0 6z", "M3 20c0-3.3 2.7-6 6-6s6 2.7 6 6M16 6.5a2.5 2.5 0 1 1 0 5M17 14c2.5.4 4 2.6 4 6"] },
  { id: "items", label: "Items & spells", blurb: "Food, gold, reagents, potions, quest items, spells.", icon: ["M6 8h12l-1 12H7L6 8z", "M9 8V6a3 3 0 0 1 6 0v2"] },
  { id: "time", label: "Time & sky", blurb: "Hour and day, moon phases and wind.", icon: ["M20 14.5A8 8 0 1 1 9.5 4a6.5 6.5 0 0 0 10.5 10.5z"] },
  { id: "fight", label: "Fight", blurb: "Start an encounter or control the current battle.", icon: ["M4 4l10 10M20 4L10 14M5 19l3-3M19 19l-3-3"] },
  { id: "checkpoints", label: "Checkpoints", blurb: "Save this moment and come back to it.", icon: ["M6 21V4h11l-2 4 2 4H6"] },
  { id: "more", label: "More", blurb: "Rarely needed tools and developer diagnostics.", icon: ["M5 12h.01M12 12h.01M19 12h.01"] },
]

/** Home shows one card for every task except More. */
export const homeTaskIDs: TaskID[] = ["goto", "fight", "party", "items", "time", "checkpoints"]

export function taskDefinition(id: TaskID): TaskDefinition {
  const found = taskDefinitions.find((task) => task.id === id)
  if (!found) throw new Error(`unknown Workbench task: ${id}`)
  return found
}

/** Surface owners keyed by catalog command; key order is the order shown in the task page. */
export const toolDefinitions: Record<string, ToolDefinition> = {
  "world.position.status": { task: "goto", name: "Go to", description: "Move the party to a map, floor and position, to a room of a dungeon, or travel to a landmark." },
  "party.character.inspect": { task: "party", name: "Party", description: "Party members, health, levels, equipment and whole-party actions." },
  "inventory.locker.status": { task: "items", name: "Items & spells", description: "Supplies, reagents, potions, scrolls, equipment stock, quest items and prepared spells." },
  "calendar.status": { task: "time", name: "Time & sky", description: "Time of day, the date, the moon and moongates, and the wind." },
  "combat.status": { task: "fight", name: "Fight", description: "Start an encounter, or control the battle that is under way." },
  "save.snapshot.list": { task: "checkpoints", name: "Checkpoints", description: "Save this moment and come back to it later." },
  "quest.item.status": { task: "more", group: "World extras", name: "Quest Item Shards", description: "Use shards of the gem on a Shadowlord's flame, and add or remove quest items one at a time." },
  "search.status": { task: "more", group: "World extras", name: "Item Stacks and Search" },
  "shrine.status": { task: "more", group: "World extras", name: "Shrine Progress", description: "Inspect and alter authoritative shrine quest and ruined-state progression." },
  "monster.ecosystem.status": { task: "more", group: "World extras", name: "Monster Ecosystem" },
  "vehicle.status": { task: "more", group: "World extras", name: "Ships" },
  "merchant.tester.catalog": { task: "more", group: "World extras", name: "Merchant Tester" },
  "command-test.telescope": { task: "more", group: "World extras", name: "Observation", description: "Queue the production Telescope, Spyglass, or Crystal Ball workflow and inspect its expected branch and context." },
  "test-setup.status": { task: "more", group: "For developers", name: "Test Setups", description: "Queue the production camping workflow, or wipe the party to test death and resurrection." },
  "guards.status": { task: "more", group: "For developers", name: "World Diagnostics", description: "Inspect hidden guard hostility, Shadowlord placement, and death/resurrection consequences." },
  "logging.status": { task: "more", group: "For developers", name: "Logging" },
  "debug.options.status": { task: "more", group: "For developers", name: "Debug Options" },
}

const toolOrder = Object.keys(toolDefinitions)
const groupOrder: ToolGroup[] = ["World extras", "For developers"]

/** Sorts owner commands in task-page order; commands without a definition follow, alphabetically by id. */
export function compareToolOrder(left: string, right: string): number {
  const leftIndex = toolOrder.indexOf(left)
  const rightIndex = toolOrder.indexOf(right)
  if (leftIndex >= 0 && rightIndex >= 0) return leftIndex - rightIndex
  if (leftIndex >= 0) return -1
  if (rightIndex >= 0) return 1
  return left.localeCompare(right)
}

export function toolGroupOrder(group: ToolGroup): number {
  return groupOrder.indexOf(group)
}

export interface PartyMemberHealth {
  status: string
  currentHp: number
  maximumHp: number
}

export interface PartyHealthSummary {
  count: number
  text: string
  needsAttention: boolean
}

/** Plain-language party health for the top bar, or undefined while no party is known. */
export function partyHealthSummary(party: PartyMemberHealth[] | undefined): PartyHealthSummary | undefined {
  if (!party || party.length === 0) return undefined
  const afflictions = new Map<string, number>()
  let wounded = 0
  for (const member of party) {
    const status = member.status.trim().toLocaleLowerCase()
    if (status !== "" && status !== "good") afflictions.set(status, (afflictions.get(status) ?? 0) + 1)
    else if (member.currentHp < member.maximumHp) wounded += 1
  }
  const parts = [...afflictions].map(([status, count]) => `${count} ${status}`)
  if (wounded > 0) parts.push(`${wounded} wounded`)
  return { count: party.length, text: parts.length === 0 ? "all healthy" : parts.join(", "), needsAttention: parts.length > 0 }
}

export interface PlaceData {
  locationName: string
  mapType: string
  floor: number
}

/** Plain-language location for the top bar. */
export function placeLabel(place: PlaceData): string {
  if (place.mapType === "dungeon") return `${place.locationName} · Level ${place.floor + 1}`
  if (place.floor === -1) return `${place.locationName} · ${place.mapType === "large" ? "Underworld" : "Basement"}`
  if (place.floor > 0) return `${place.locationName} · Floor ${place.floor}`
  return place.locationName
}

/**
 * One page per task: its anchor command stands for the page everywhere (palette, polling, results),
 * `queries` are read on open and while the page is visible, `commands` belong to the page, and
 * `uses` are other tools' commands the page also calls.
 */
export interface PageDefinition {
  anchor: string
  queries: string[]
  loadOnce?: string[]
  commands: string[]
  uses?: string[]
}

export const pageDefinitions: Partial<Record<TaskID, PageDefinition>> = {
  goto: {
    anchor: "world.position.status",
    queries: ["world.position.status"],
    commands: ["world.position.set", "movement.landmark.travel", "movement.free-move.set"],
  },
  party: {
    anchor: "party.character.inspect",
    queries: ["party.character.inspect", "inventory.locker.status"],
    commands: ["party.character.update", "party.equipment.loadout.update", "party.maximize-levels", "party.fill", "party.member.add", "party.karma.set", "party.restore"],
  },
  items: {
    anchor: "inventory.locker.status",
    queries: ["inventory.locker.status", "inventory.spell.status", "quest.item.status"],
    commands: [
      "inventory.provision.set", "inventory.provision.preset", "inventory.reagent.set", "inventory.reagent.preset", "inventory.potion.add",
      "inventory.potion.preset", "inventory.scroll.add", "inventory.scroll.preset", "inventory.equipment.preset", "inventory.equipment.grant-preset",
      "inventory.equipment.set", "inventory.equipment.clear", "quest.inventory.preset-usable",
      "inventory.spell.status", "inventory.spell.set", "inventory.spell.preset-max", "spell.effect.set-remaining", "spell.effect.activate",
    ],
    uses: ["quest.item.ensure", "quest.item.remove"],
  },
  time: {
    anchor: "calendar.status",
    queries: ["calendar.status", "environment.wind.status", "moongate.status"],
    commands: ["calendar.set-time", "calendar.set-day", "environment.wind.status", "environment.wind.set", "moongate.status", "overlay.moongate.set", "moongate.travel"],
  },
  fight: {
    anchor: "combat.status",
    queries: ["combat.status", "party.character.inspect"],
    loadOnce: ["combat.encounter.options"],
    commands: [
      "combat.encounter.options", "combat.encounter.start", "combat.monster.sleep.set", "combat.exit-debug", "combat.monster.spawn", "rng.seed.set",
      "combat.monster.damage-raw", "combat.monster.kill", "combat.monster.kill-all", "combat.monster.sleep-all", "combat.dungeon-room.clear",
      "combat.monster.clear", "combat.presentation.speed.set",
    ],
  },
  checkpoints: {
    anchor: "save.snapshot.list",
    queries: ["save.snapshot.list", "test-setup.status"],
    commands: ["save.snapshot.get", "save.snapshot.capture", "save.snapshot.load", "save.snapshot.delete"],
  },
}

export type HomeQuickEffect =
  | { kind: "landmark"; landmark: string }
  | { kind: "party-restore" }
  | { kind: "time"; hour: number; minute: number }
  | { kind: "open"; commandId: string }
  | { kind: "exit-combat" }

export interface HomeQuickAction {
  id: string
  label: string
  description: string
  effect: HomeQuickEffect
  disabled?: boolean
}

export interface HomeCard {
  task: TaskDefinition
  quick: HomeQuickAction[]
}

export const preferredLandmarks = ["codex", "lord_british_secret_room", "honesty"]

export const timePresets: { hour: number; minute: number; label: string }[] = [
  { hour: 5, minute: 0, label: "Set 05:00" },
  { hour: 12, minute: 0, label: "Set 12:00" },
  { hour: 20, minute: 0, label: "Set 20:00" },
]
