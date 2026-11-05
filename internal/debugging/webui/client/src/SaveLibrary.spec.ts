import { createApp, h, nextTick, ref, type App as VueApp } from "vue"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import SaveLibrary from "./SaveLibrary.vue"
import type { SaveSnapshotDetail, SaveSnapshotListData } from "./saves"

let app: VueApp<Element> | undefined
let root: HTMLElement

const revision = "a".repeat(64)
const summary = {
  reference: "personal:before-doom", source: "personal" as const, id: "before-doom", name: "Before Doom",
  description: "Reviewed checkpoint", createdAt: "2026-08-06T12:00:00Z", gameVersion: "test", revision,
  compatibility: { compatible: true }, eligibility: { canCapture: true, canLoad: true, canDelete: true },
}
const detail: SaveSnapshotDetail = {
  summary, snapshotVersion: 1, location: 2, locationName: "Britain", x: 12, y: 17, floor: 2, year: 1, month: 4, day: 8,
  hour: 21, minute: 30, partyMembers: 6, moonPhase: "Full Moon", moonPhaseIndex: 4,
}
const list: SaveSnapshotListData = { snapshots: [summary], captureEligibility: { canCapture: true, canLoad: true, canDelete: true } }

beforeEach(() => {
  root = document.createElement("div")
  document.body.append(root)
})

afterEach(() => {
  app?.unmount()
  app = undefined
  root.remove()
})

describe("Checkpoints page", () => {
  it("compares now with after loading, pinned to the checkpoint as it was fetched", async () => {
    const selected = ref<SaveSnapshotDetail | undefined>(undefined)
    const inspect = vi.fn()
    const load = vi.fn()
    app = createApp({ setup: () => () => h(SaveLibrary, {
      data: list, detail: selected.value, pendingList: false, pendingDetail: false, pendingMutation: false,
      currentScene: { locationName: "Yew", floor: 0, date: "1-1-1", time: "12:00", partyMembers: 5 },
      onInspect: inspect, onLoad: load,
    }) })
    app.mount(root)
    click('[data-save-action="review-load"]')
    expect(inspect).toHaveBeenCalledWith(summary.reference)
    selected.value = structuredClone(detail)
    await nextTick()
    const review = root.querySelector('[data-save-review="load"]')!
    expect(review.textContent).toContain("Yew")
    expect(review.textContent).toContain("Britain")
    expect(review.textContent).toContain("Full Moon")
    expect(review.textContent).not.toMatch(/undo/i)

    selected.value = { ...selected.value, locationName: "Changed live detail" }
    await nextTick()
    expect(root.querySelector('[data-save-review="load"]')?.textContent).not.toContain("Changed live detail")

    click('[data-save-action="confirm-load"]')
    expect(load).toHaveBeenCalledWith({ reference: summary.reference, expectedRevision: revision })
  })

  it("disarms the review when the checkpoint's revision changes underneath it", async () => {
    const selected = ref<SaveSnapshotDetail>(structuredClone(detail))
    app = createApp({ setup: () => () => h(SaveLibrary, { data: list, detail: selected.value, pendingList: false, pendingDetail: false, pendingMutation: false }) })
    app.mount(root)
    click('[data-save-action="review-load"]')
    selected.value = structuredClone(detail)
    await nextTick()
    expect(root.querySelector('[data-save-action="confirm-load"]')).not.toBeNull()
    selected.value = { ...detail, summary: { ...summary, revision: "b".repeat(64) } }
    await nextTick()
    expect(root.querySelector('[data-save-action="confirm-load"]')).toBeNull()
  })

  it("derives the slug from the name, shows why saving is blocked, and lists no automatic checkpoints", async () => {
    const capture = vi.fn()
    app = createApp(SaveLibrary, {
      data: { snapshots: [], captureEligibility: { canCapture: false, canLoad: true, canDelete: false, reason: "Not available during combat." } },
      pendingList: false, pendingDetail: false, pendingMutation: false, onCapture: capture,
    })
    app.mount(root)
    expect(root.querySelector("[data-save-blocked]")?.textContent).toContain("combat")
    expect(root.querySelector<HTMLButtonElement>('[data-save-action="capture"]')!.disabled).toBe(true)
    expect(root.textContent).not.toContain("Automatic")
    expect(root.querySelector('[data-save-list="rollback"]')).toBeNull()
  })

  it("saves a named checkpoint for the chosen audience", async () => {
    const capture = vi.fn()
    app = createApp(SaveLibrary, { data: list, pendingList: false, pendingDetail: false, pendingMutation: false, onCapture: capture })
    app.mount(root)
    input("[data-save-name]", "Covetous L6 before room 12")
    click('[data-save-owner="project"]')
    await nextTick()
    click('[data-save-action="capture"]')
    expect(capture).toHaveBeenCalledWith({ source: "project", id: "covetous-l6-before-room-12", name: "Covetous L6 before room 12", description: "" })
  })

  it("requires the exact checkpoint name before emitting a revision-safe delete", async () => {
    const remove = vi.fn()
    const selected = ref<SaveSnapshotDetail | undefined>(undefined)
    app = createApp({ setup: () => () => h(SaveLibrary, { data: list, detail: selected.value, pendingList: false, pendingDetail: false, pendingMutation: false, onDelete: remove }) })
    app.mount(root)
    click('[data-save-action="review-delete"]')
    selected.value = structuredClone(detail)
    await nextTick()
    const confirm = root.querySelector<HTMLButtonElement>('[data-save-action="confirm-delete"]')!
    expect(confirm.disabled).toBe(true)
    input('[data-save-review="delete"] input', summary.name)
    await nextTick()
    expect(confirm.disabled).toBe(false)
    confirm.click()
    expect(remove).toHaveBeenCalledWith({ reference: summary.reference, expectedRevision: revision })
  })
})

function click(selector: string): void {
  const element = root.querySelector<HTMLElement>(selector)
  if (!element) throw new Error(`missing ${selector}`)
  element.click()
}

function input(selector: string, value: string): void {
  const element = root.querySelector<HTMLInputElement>(selector)
  if (!element) throw new Error(`missing ${selector}`)
  element.value = value
  element.dispatchEvent(new Event("input", { bubbles: true }))
}
