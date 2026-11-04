import { createApp, type App as VueApp } from "vue"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import WorkbenchHome from "./WorkbenchHome.vue"
import type { NavigationSurface } from "./command_navigation"

let app: VueApp<Element> | undefined
let root: HTMLDivElement

const recent: NavigationSurface = {
  id: "game:shrine.status",
  workspace: "game",
  workspaceLabel: "Game",
  label: "Shrine Progress",
  description: "Inspect and alter shrine progression.",
  commandId: "shrine.status",
  commands: [],
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

describe("Workbench Home", () => {
  it("opens recent surfaces and the navigation palette without executing commands", () => {
    const openPalette = vi.fn()
    const openSurface = vi.fn()
    mount({ recents: [recent], onOpenPalette: openPalette, onOpenSurface: openSurface })

    click("[data-home-go-to]")
    click('[data-home-recent="game:shrine.status"]')

    expect(openPalette).toHaveBeenCalledOnce()
    expect(openSurface).toHaveBeenCalledOnce()
    expect(openSurface).toHaveBeenCalledWith(recent)
  })

  it("offers a useful empty state", () => {
    mount({ recents: [] })

    expect(root.querySelector(".home-recents")?.textContent).toContain("NO RECENT SURFACES")
    expect(root.querySelector(".home-getting-started")?.textContent).toContain("searches every Workbench surface")
  })
})

function mount(props: Record<string, unknown>): void {
  app = createApp(WorkbenchHome, props)
  app.mount(root)
}

function click(selector: string): void {
  const element = root.querySelector<HTMLElement>(selector)
  if (!element) throw new Error(`missing test element: ${selector}`)
  element.dispatchEvent(new MouseEvent("click", { bubbles: true }))
}
