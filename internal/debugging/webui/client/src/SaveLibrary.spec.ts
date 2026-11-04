import { createApp, h, nextTick, ref, type App as VueApp } from "vue"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import SaveLibrary from "./SaveLibrary.vue"
import type { SaveSnapshotDetail, SaveSnapshotListData } from "./saves"

let app: VueApp<Element> | undefined
let root: HTMLElement

const revision = "a".repeat(64)
const summary = {
  reference: "project:before-doom", source: "project" as const, id: "before-doom", name: "Before Doom",
  description: "Reviewed checkpoint", createdAt: "2026-08-06T12:00:00Z", gameVersion: "test", revision,
  compatibility: { compatible: true }, eligibility: { canCapture: true, canLoad: true, canDelete: true },
}
const detail: SaveSnapshotDetail = {
  summary, snapshotVersion: 1, location: 2, locationName: "Britain", x: 12, y: 17, floor: 2, year: 1, month: 4, day: 8,
  hour: 21, minute: 30, partyMembers: 6, moonPhase: "Full Moon", moonPhaseIndex: 4,
}

beforeEach(() => {
  root = document.createElement("div")
  document.body.append(root)
})

afterEach(() => {
  app?.unmount()
  app = undefined
  root.remove()
})

describe("SaveLibrary", () => {
  it("pins review values and disarms when the selected revision changes", async () => {
    const selected = ref(structuredClone(detail))
    const load = vi.fn()
    app = createApp({ setup: () => () => h(SaveLibrary, {
      detail: selected.value, pendingList: false, pendingDetail: false, pendingMutation: false, onLoad: load,
    }) })
    app.mount(root)
    click('[data-save-action="review-load"]')
    await nextTick()
    selected.value = { ...selected.value, locationName: "Changed live detail" }
    await nextTick()
    expect(root.querySelector('[data-save-review="load"]')?.textContent).toContain("Britain")
    expect(root.querySelector('[data-save-review="load"]')?.textContent).not.toContain("Changed live detail")
    selected.value = { ...selected.value, summary: { ...selected.value.summary, revision: "b".repeat(64) } }
    await nextTick()
    expect(root.querySelector('[data-save-action="confirm-load"]')).toBeNull()
    expect(load).not.toHaveBeenCalled()
  })
  it("keeps authoritative capture eligibility visible for an empty repository and derives an editable slug", async () => {
    mount({ snapshots: [], captureEligibility: { canCapture: true, canLoad: true, canDelete: false } })
    expect(root.querySelector('[data-save-source="project"]')?.textContent).toContain("NO PROJECT CAPTURES")

    input('input[placeholder="Before Dungeon Entry"]', "Before Dungeon Entry")
    await nextTick()
    expect((root.querySelector("[data-save-capture-slug]") as HTMLInputElement).value).toBe("before-dungeon-entry")
    input("[data-save-capture-slug]", "custom-checkpoint")
    input('input[placeholder="Before Dungeon Entry"]', "Renamed Capture")
    await nextTick()
    expect((root.querySelector("[data-save-capture-slug]") as HTMLInputElement).value).toBe("custom-checkpoint")
    expect((root.querySelector('[data-save-action="capture"]') as HTMLButtonElement).disabled).toBe(false)
  })

  it("renders detached review details with text moon label and stages scene-bound load", async () => {
    const load = vi.fn()
    mount({ snapshots: [summary], captureEligibility: { canCapture: true, canLoad: true, canDelete: false } }, detail, { load })
    expect(root.textContent).toContain("Full Moon")
    expect(root.textContent).toContain("6 active members")
    expect(root.textContent).toContain("Britain")
    click('[data-save-action="review-load"]')
    await nextTick()
    expect(root.querySelector('[data-save-review="load"]')?.textContent).toContain("AUTOMATIC ROLLBACK")
    expect(root.querySelector('[data-save-review="load"]')?.textContent).toContain("Britain")
    click('[data-save-action="confirm-load"]')
    expect(load).toHaveBeenCalledWith({ reference: summary.reference, expectedRevision: revision })
  })

  it("requires the exact snapshot name before emitting a revision-safe delete", async () => {
    const remove = vi.fn()
    mount({ snapshots: [summary], captureEligibility: { canCapture: true, canLoad: true, canDelete: false } }, detail, { delete: remove })
    click('[data-save-action="review-delete"]')
    await nextTick()
    const confirm = root.querySelector<HTMLButtonElement>('[data-save-action="confirm-delete"]')
    expect(confirm?.disabled).toBe(true)
    input('[data-save-review="delete"] input', summary.name)
    await nextTick()
    expect(confirm?.disabled).toBe(false)
    confirm?.click()
    expect(remove).toHaveBeenCalledWith({ reference: summary.reference, expectedRevision: revision })
  })
})

function mount(data: SaveSnapshotListData, selected?: SaveSnapshotDetail, handlers: { load?: (value: unknown) => void; delete?: (value: unknown) => void } = {}): void {
  app = createApp(SaveLibrary, {
    data, detail: selected, pendingList: false, pendingDetail: false, pendingMutation: false,
    onLoad: handlers.load,
    onDelete: handlers.delete,
  })
  app.mount(root)
}

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
