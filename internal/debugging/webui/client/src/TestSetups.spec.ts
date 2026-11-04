import { createApp, nextTick, type App as VueApp } from "vue"
import { afterEach, beforeEach, describe, expect, it } from "vitest"

import TestSetups from "./TestSetups.vue"
import type { TestSetupStatusData } from "./approved_tool_views"

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

function status(overrides: Partial<TestSetupStatusData> = {}): TestSetupStatusData {
  return {
    partyFingerprint: "party-1",
    wipeReview: { fingerprint: "wipe-1234567890abcdef", members: [{ slot: 0, name: "Avatar", hp: 30, status: "good" }] },
    locationId: 1, locationName: "Britannia", mapType: "large", floor: 0,
    date: "1-1-142", time: "12:00", food: 100,
    combatActive: false, dialogActive: false, gameplayWorkflowActive: false,
    scriptedSceneActive: false, gameplayTerminal: false, finaleActive: false, allPartyDead: false,
    party: [],
    ...overrides,
  }
}

function mountTestSetups(data: TestSetupStatusData): Array<{ commandID: string; argumentsByID: Record<string, string> }> {
  const runs: Array<{ commandID: string; argumentsByID: Record<string, string> }> = []
  app = createApp(TestSetups, {
    data,
    pendingStatus: false,
    pendingHoleUp: false,
    pendingKillParty: false,
    pendingDeathSequence: false,
    onRun: (operation: { commandID: string; argumentsByID: Record<string, string> }) => runs.push(operation),
  })
  app.mount(root)
  return runs
}

describe("Test setups", () => {
  it("blocks every setup after a terminal finale", () => {
    mountTestSetups(status({ gameplayTerminal: true, finaleActive: true, locationName: "Finale" }))

    expect(root.querySelector(".test-setups-context")?.textContent).toContain("TERMINAL FINALE")
    expect(root.querySelector<HTMLButtonElement>(".hole-up-card button")?.disabled).toBe(true)
    expect(root.querySelector<HTMLButtonElement>(".death-wipe-card button")?.disabled).toBe(true)
  })

  it("requires a second click on the same reviewed fingerprint before wiping the party", async () => {
    const runs = mountTestSetups(status())
    const wipe = root.querySelector<HTMLButtonElement>(".death-wipe-card button")
    if (!wipe) throw new Error("missing wipe button")

    wipe.click()
    await nextTick()
    expect(runs).toEqual([])
    expect(root.querySelector("[data-quick-review]")?.textContent).toContain("wipe-1234567890abcdef")

    wipe.click()
    await nextTick()
    expect(runs).toEqual([{ commandID: "death.kill-party", argumentsByID: { expectedFingerprint: "wipe-1234567890abcdef" } }])
    expect(root.querySelector("[data-quick-review]")).toBeNull()
  })

  it("queues camping against the reviewed party fingerprint", () => {
    const runs = mountTestSetups(status())

    root.querySelector<HTMLButtonElement>(".hole-up-card button")?.click()

    expect(runs).toHaveLength(1)
    expect(runs[0]?.commandID).toBe("command-test.hole-up")
    expect(runs[0]?.argumentsByID.expectedPartyFingerprint).toBe("party-1")
  })
})
