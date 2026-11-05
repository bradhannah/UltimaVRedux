import { createApp, h, nextTick, type App as VueApp } from "vue"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { ActiveCombatStatusData, CalendarStatusData, InventoryLockerStatusData, PartyCharacterInspectData, WorldPositionStatusData } from "./approved_tool_views"
import FightPage from "./FightPage.vue"
import GoToPage from "./GoToPage.vue"
import initialCatalog from "./initial_catalog.fixture.json"
import ItemsPage from "./ItemsPage.vue"
import PartyPage from "./PartyPage.vue"
import TimePage from "./TimePage.vue"
import type { CommandDefinition } from "./protocol"

const commands = initialCatalog as CommandDefinition[]
let app: VueApp<Element> | undefined
let root: HTMLElement

beforeEach(() => {
  root = document.createElement("div")
  document.body.append(root)
})
afterEach(() => {
  app?.unmount()
  app = undefined
  root.remove()
})

function mount(component: object, props: Record<string, unknown>): void {
  app = createApp({ setup: () => () => h(component, props) })
  app.mount(root)
}
function click(selector: string): void {
  const element = root.querySelector<HTMLElement>(selector)
  if (!element) throw new Error(`missing ${selector}`)
  element.click()
}
async function type(selector: string, value: string): Promise<void> {
  const element = root.querySelector<HTMLInputElement | HTMLSelectElement>(selector)
  if (!element) throw new Error(`missing ${selector}`)
  element.value = value
  element.dispatchEvent(new Event(element instanceof HTMLSelectElement ? "change" : "input", { bubbles: true }))
  await nextTick()
}

describe("Time & sky page", () => {
  const calendar: CalendarStatusData = { year: 138, month: 7, day: 12, hour: 16, minute: 15, turn: 1, date: "July 12", time: "4:15 PM", moonPhase: "Full Moon", moonPhaseIndex: 4 }

  it("stages time, day and wind together and applies them as one reviewed list", async () => {
    const apply = vi.fn()
    mount(TimePage, { calendar, windDirection: "Calm", busy: false, travelPending: false, overlayPending: false, onApply: apply })
    expect(root.querySelector("[data-change-bar]")).toBeNull()
    click('[data-time-preset="20:00"]')
    await type("[data-time-day]", "14")
    click('[data-wind="west"]')
    await nextTick()
    expect(root.querySelector("[data-change-bar]")?.textContent).toContain("3 changes")
    click("[data-change-apply]")
    expect(apply).toHaveBeenCalledWith([
      { commandID: "calendar.set-time", argumentsByID: { hour: "20", minute: "0" } },
      { commandID: "calendar.set-day", argumentsByID: { day: "14" } },
      { commandID: "environment.wind.set", argumentsByID: { direction: "west" } },
    ])
    await nextTick()
    expect(root.querySelector("[data-change-bar]")).toBeNull()
  })

  it("blocks an impossible clock value with a plain reason and reads the moon from the calendar", async () => {
    mount(TimePage, { calendar, busy: false, travelPending: false, overlayPending: false })
    await type("[data-time-hour]", "25")
    expect(root.querySelector("[data-change-bar]")?.textContent).toContain("Hour must be 0-23")
    expect(root.querySelector<HTMLButtonElement>("[data-change-apply]")!.disabled).toBe(true)
    expect(root.querySelector("[data-moon-rail]")?.getAttribute("aria-label")).toContain("Full Moon")
  })
})

describe("Items & spells page", () => {
  const locker: InventoryLockerStatusData = {
    snapshotFingerprint: "locker-1",
    provisions: [{ name: "gold", quantity: 10, maximum: 9999 }],
    reagents: [{ id: 2, name: "Garlic", quantity: 4, maximum: 99 }, { id: 5, name: "Black Pearl", quantity: 0, maximum: 99 }],
    potions: [{ id: 0, name: "Blue", quantity: 1, maximum: 99 }], scrolls: [], equipment: [], equipped: [],
    usablePresetReview: { reviewFingerprint: "usable-1", changes: [{ category: "potions", path: "potions.0", before: 1, after: 5 }] },
  }

  it("stages reagent edits and sends one command per row with the quantity it was reviewed at", async () => {
    const apply = vi.fn()
    mount(ItemsPage, { locker, commands, busy: false, onApply: apply })
    click('[data-items-tab="reagents"]')
    await nextTick()
    await type('[aria-label="Garlic new amount"]', "50")
    click('[data-item-row="reagent:5"] .pg-btn.small')
    await nextTick()
    expect(root.querySelector("[data-change-bar]")?.textContent).toContain("2 changes")
    click("[data-change-apply]")
    expect(apply).toHaveBeenCalledWith([
      { commandID: "inventory.reagent.set", argumentsByID: { reagent: "2", targetQuantity: "50", expectedQuantity: "4" } },
      { commandID: "inventory.reagent.set", argumentsByID: { reagent: "5", targetQuantity: "99", expectedQuantity: "0" } },
    ])
  })

  it("searches every kind, adds potions rather than setting them, and refuses to lower them", async () => {
    const apply = vi.fn()
    mount(ItemsPage, { locker, commands, busy: false, onApply: apply })
    await type("[data-items-search]", "blue")
    expect(root.querySelectorAll("[data-item-row]")).toHaveLength(1)
    await type('[aria-label="Blue potion new amount"]', "0")
    expect(root.querySelector<HTMLButtonElement>("[data-change-apply]")!.disabled).toBe(true)
    expect(root.querySelector("[data-change-bar]")?.textContent).toContain("can only be added")
    await type('[aria-label="Blue potion new amount"]', "6")
    click("[data-change-apply]")
    expect(apply).toHaveBeenCalledWith([{ commandID: "inventory.potion.add", argumentsByID: { potion: "0", amount: "5" } }])
  })

  it("stages a usable-inventory preset against its review fingerprint and drops it when the inventory moves", async () => {
    const apply = vi.fn()
    mount(ItemsPage, { locker, commands, busy: false, onApply: apply })
    click("[data-items-preset]:last-of-type")
    await nextTick()
    expect(root.querySelector("[data-change-bar]")?.textContent).toContain("Everything a playtest needs")
    click("[data-change-apply]")
    expect(apply).toHaveBeenCalledWith([{ commandID: "quest.inventory.preset-usable", argumentsByID: { expectedFingerprint: "usable-1" } }])
  })
})

describe("Party page", () => {
  const member = (rosterIndex: number, name: string, currentHp: number) => ({
    rosterIndex, name, gender: 0, class: "Bard", status: "good" as const, active: true, partyMap: 0, strength: 18, dexterity: 22,
    intelligence: 20, currentMp: 5, currentHp, maximumHp: 120, experience: 420, level: 4, monthsAtInn: 0, armorClass: 0,
    helmet: 255, armor: 255, weapon: 255, shield: 255, ring: 255, amulet: 255,
  })
  const roster: PartyCharacterInspectData = {
    fingerprint: "roster-1", karma: 50, characters: [],
    roster: [member(0, "Avatar", 96), member(1, "Iolo", 60), { ...member(7, "Gwenno", 40), active: false }],
  }
  const optionSets = { "party.active-characters": [
    { value: "t-avatar", label: "Avatar", metadata: { partyPosition: 1, name: "Avatar" } },
    { value: "t-iolo", label: "Iolo", metadata: { partyPosition: 2, name: "Iolo" } },
  ] }

  it("applies whole-party actions with the roster fingerprint and offers no best-gear action", () => {
    const apply = vi.fn()
    mount(PartyPage, { roster, optionSets, commands, busy: false, onApply: apply })
    expect(root.querySelector('[data-party-wide="party.equipment.best"]')).toBeNull()
    click('[data-party-wide="party.restore"]')
    expect(apply).toHaveBeenCalledWith([{ commandID: "party.restore", argumentsByID: { expectedFingerprint: "roster-1" } }])
  })

  it("keeps each member's edits until applied and sends them under that member's token", async () => {
    const apply = vi.fn()
    mount(PartyPage, { roster, optionSets, commands, busy: false, onApply: apply })
    click('[data-party-member="2"]')
    await nextTick()
    click('[data-party-full="currentHp"]')
    click('[data-party-status="poisoned"]')
    await nextTick()
    expect(root.querySelector("[data-change-bar]")?.textContent).toContain("Iolo")
    click('[data-party-member="1"]')
    await type('[data-party-field="strength"]', "30")
    expect(root.querySelector("[data-change-bar]")?.textContent).toContain("3 changes")
    click("[data-change-apply]")
    expect(apply).toHaveBeenCalledWith([
      { commandID: "party.character.update", argumentsByID: { character: "t-avatar", strength: "30" } },
      { commandID: "party.character.update", argumentsByID: { character: "t-iolo", currentHp: "120", status: "poisoned" } },
    ])
  })

  it("stages a party position change for a member other than the Avatar", async () => {
    const apply = vi.fn()
    const sets = { ...optionSets, "party.active-positions": [1, 2, 3].map((n) => ({ value: String(n), label: `Position ${n}` })) }
    mount(PartyPage, { roster, optionSets: sets, commands, busy: false, onApply: apply })
    expect(root.querySelector('select[data-party-field="partyPosition"]')).toBeNull()
    click('[data-party-member="2"]')
    await nextTick()
    expect([...root.querySelectorAll('select[data-party-field="partyPosition"] option')].map((o) => o.textContent)).toEqual(["Position 2", "Position 3"])
    await type('select[data-party-field="partyPosition"]', "3")
    click("[data-change-apply]")
    expect(apply).toHaveBeenCalledWith([{ commandID: "party.character.update", argumentsByID: { character: "t-iolo", partyPosition: "3" } }])
  })

  it("refuses hit points above the maximum", async () => {
    mount(PartyPage, { roster, optionSets, commands, busy: false })
    await type('[data-party-field="currentHp"]', "500")
    expect(root.querySelector("[data-change-bar]")?.textContent).toContain("Must be between 1 and 120")
  })
})

describe("Go to page", () => {
  const status: WorldPositionStatusData = {
    sourceFingerprint: "src-1", locationId: 2, locationName: "Britain", mapType: "small", floor: 0, x: 15, y: 30,
    bounds: { minimumX: 0, maximumX: 31, minimumY: 0, maximumY: 31 },
    destinations: [{
      locationId: 41, locationName: "Covetous", mapType: "dungeon", floors: [4, 5],
      bounds: { minimumX: 0, maximumX: 7, minimumY: 0, maximumY: 7 }, defaultSetup: { floor: 4, x: 1, y: 1 },
      placementTargets: [{ placement: "dungeon_inside_start", placementLabel: "Inside dungeon start", route: "britannia", routeLabel: "Britannia", available: true, locationId: 41, setup: { floor: 4, x: 1, y: 1 }, facingLabel: "East" }],
      rooms: [
        { room: 12, label: "Room 12", floor: 5, roomX: 3, roomY: 4, available: true, setup: { floor: 5, x: 2, y: 4 }, facingLabel: "East", summary: "Ghost ×3, Insects" },
        { room: 13, label: "Room 13", floor: 5, roomX: 6, roomY: 6, available: false, unavailableReason: "Walled in on every side." },
        { room: 3, label: "Room 3", floor: 4, roomX: 1, roomY: 1, available: true, setup: { floor: 4, x: 1, y: 2 }, facingLabel: "North" },
      ],
    }],
  }
  const base = { data: status, landmarks: [{ value: "honesty", label: "Shrine of Honesty" }], pending: false, pendingLandmark: false, refreshing: false, stale: false, freeMove: false, freeMovePending: false, canFreeMove: true }

  it("arrives outside a room on its level, facing it, and says what one step does", async () => {
    const apply = vi.fn()
    const fight = vi.fn()
    mount(GoToPage, { ...base, onApply: apply, onFightRoom: fight })
    click('[data-goto-place="dest:41"]')
    await nextTick()
    click('[data-goto-mode="dungeon_outside_room"]')
    await nextTick()
    click('[data-goto-level="5"]')
    await nextTick()
    expect(root.querySelectorAll("[data-goto-room]")).toHaveLength(2)
    expect(root.querySelector("[data-goto-sentence]")?.textContent).toBe("You will stand at cell 2,4 on level 6, facing east. Step forward to enter Room 12.")
    click("[data-goto-go]")
    expect(apply).toHaveBeenCalledWith({
      placement: "dungeon_outside_room", locationId: 41, room: 12,
      expectedLocationId: 2, expectedFloor: 0, expectedX: 15, expectedY: 30, expectedSourceFingerprint: "src-1",
    })
    click("[data-goto-fight]")
    expect(fight).toHaveBeenCalledWith({ locationName: "Covetous", room: 12 })
  })

  it("explains a room that cannot be approached instead of offering Go there", async () => {
    mount(GoToPage, base)
    click('[data-goto-place="dest:41"]')
    await nextTick()
    click('[data-goto-mode="dungeon_outside_room"]')
    await nextTick()
    click('[data-goto-level="5"]')
    await nextTick()
    click('[data-goto-room="13"]')
    await nextTick()
    expect(root.querySelector("[data-goto-reason]")?.textContent).toContain("Walled in on every side.")
    expect(root.querySelector<HTMLButtonElement>("[data-goto-go]")!.disabled).toBe(true)
  })

  it("filters places by kind and search and travels to landmarks", async () => {
    const travel = vi.fn()
    mount(GoToPage, { ...base, onTravelLandmark: travel })
    click('[data-goto-kind="landmark"]')
    await nextTick()
    expect(root.querySelectorAll("[data-goto-place]")).toHaveLength(1)
    click('[data-goto-place="land:honesty"]')
    await nextTick()
    click("[data-goto-go]")
    expect(travel).toHaveBeenCalledWith("honesty")
    click('[data-goto-kind="recent"]')
    await nextTick()
    expect(root.querySelectorAll("[data-goto-place]")).toHaveLength(1)
  })
})

describe("Fight page", () => {
  const combat = (overrides: Partial<ActiveCombatStatusData> = {}): ActiveCombatStatusData => ({
    active: true, snapshotFingerprint: "combat-1", encounterId: "9", monsterTypes: [{ id: 32, name: "Orc" }],
    map: { name: "Glade", index: 2, territory: "britannia", entryDirection: "north", victory: false, turnStarted: true, pendingAdvanceTurn: false, soloMoveMode: false, currentPartyIndex: 0, activeMonsterIndex: -1, targetPartyIndex: -1, objectCount: 0, cells: [] },
    party: [], monsters: [{
      id: "1", managerIndex: 0, typeId: 32, name: "Orc", x: 3, y: 3, currentHp: 25, maximumHp: 30, currentDexterity: 10, baseDexterity: 10,
      active: true, dead: false, invisible: false, asleep: false, poisoned: false, currentActor: false, queueCountdown: 0, queueStatus: "", canAct: true, abilities: [], hostile: true,
    }], forecast: [], activeMonsterCount: 1, activeHostileMonsterCount: 1, totalMonsterCount: 1, fixedDungeonRoom: false, defeatedExperience: 0,
    rngSeed: 42, workflowActive: false, presentationSpeedPercent: 100, configurationEditing: false, ...overrides,
  })

  it("sends each monster action and the group actions with the reviewed encounter snapshot", () => {
    const apply = vi.fn()
    mount(FightPage, { combat: combat(), commands, busy: false, startPending: false, onApply: apply })
    expect(root.querySelector("[data-fight-heal]")).toBeNull()
    click("[data-monster-hurt]")
    click("[data-monster-sleep]")
    click("[data-monster-defeat]")
    click("[data-fight-sleep-all]")
    click("[data-fight-win]")
    click("[data-fight-leave]")
    const base = { encounterId: "9", expectedSnapshot: "combat-1" }
    expect(apply.mock.calls.map(([steps]) => steps[0])).toEqual([
      { commandID: "combat.monster.damage-raw", argumentsByID: { ...base, monsterId: "1", expectedCurrentHp: "25", damage: "10" } },
      { commandID: "combat.monster.sleep.set", argumentsByID: { ...base, monsterId: "1", asleep: "true", expectedAsleep: "false" } },
      { commandID: "combat.monster.kill", argumentsByID: { ...base, monsterId: "1", expectedCurrentHp: "25" } },
      { commandID: "combat.monster.sleep-all", argumentsByID: base },
      { commandID: "combat.monster.kill-all", argumentsByID: base },
      { commandID: "combat.exit-debug", argumentsByID: base },
    ])
  })

  it("holds every action back while a monster is acting and says why", () => {
    mount(FightPage, { combat: combat({ workflowActive: true }), commands, busy: false, startPending: false })
    expect(root.querySelector("[data-fight-blocked]")?.textContent).toContain("Wait")
    expect(root.querySelector<HTMLButtonElement>("[data-fight-win]")!.disabled).toBe(true)
  })

  it("offers the encounter composer when no battle is under way", () => {
    mount(FightPage, { combat: combat({ active: false, unavailableReason: "No active combat." }), commands, busy: false, startPending: false })
    expect(root.querySelector(".encounter-composer")).not.toBeNull()
    expect(root.querySelector("[data-fight-win]")).toBeNull()
  })
})
