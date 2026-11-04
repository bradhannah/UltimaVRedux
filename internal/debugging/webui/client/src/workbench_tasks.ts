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
  "world.position.status": { task: "goto", name: "World Navigator", description: "Move the party to a map, floor and position, or travel to a landmark, with a review of the destination first." },
  "party.character.inspect": { task: "party", name: "Party Roster", description: "Review active positions 1-6 and inactive companions, then run fingerprint-checked roster and party-wide operations." },
  "party.character.update": { task: "party", name: "Character Editor", description: "Edit one active character's attributes, resources, individual condition and party position with a pending-change preview." },
  "party.equipment.loadout.update": { task: "party", name: "Equipment Loadouts", description: "Stage and review one partial six-slot plan for one or all active party members as an atomic operation." },
  "inventory.locker.status": { task: "items", name: "Inventory Locker" },
  "inventory.spell.status": { task: "items", name: "Spell Inventory" },
  "quest.item.status": { task: "items", name: "Quest Items" },
  "search.status": { task: "items", name: "Item Stacks and Search" },
  "calendar.status": { task: "time", name: "Calendar" },
  "environment.wind.status": { task: "time", name: "Wind" },
  "moongate.status": { task: "time", name: "Moongates and Moon Phases" },
  "combat.status": { task: "fight", name: "Active Combat" },
  "combat.encounter.options": { task: "fight", name: "Encounter Composer" },
  "save.snapshot.list": { task: "checkpoints", name: "Checkpoints", description: "Capture, inspect, load, and revision-safely remove immutable gameplay checkpoints." },
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

export type HomeQuickEffect =
  | { kind: "landmark"; landmark: string }
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
