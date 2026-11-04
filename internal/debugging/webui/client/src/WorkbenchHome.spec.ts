import { createApp, type App as VueApp } from "vue"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import WorkbenchHome from "./WorkbenchHome.vue"
import type { NavigationSurface } from "./command_navigation"
import { taskDefinition, type HomeCard } from "./workbench_tasks"

let app: VueApp<Element> | undefined
let root: HTMLDivElement

const recent: NavigationSurface = {
  id: "more:shrine.status",
  workspace: "more",
  workspaceLabel: "More",
  label: "Shrine Progress",
  description: "Inspect and alter shrine progression.",
  commandId: "shrine.status",
  commands: [],
}

const cards: HomeCard[] = [
  {
    task: taskDefinition("time"),
    quick: [{ id: "time:20:0", label: "Set 20:00", description: "Set the time of day to 20:00", effect: { kind: "time", hour: 20, minute: 0 } }],
  },
  { task: taskDefinition("party"), quick: [] },
]

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
  it("offers a card per task with quick actions, an open button and recent tools", () => {
    const openTask = vi.fn()
    const openSurface = vi.fn()
    const quick = vi.fn()
    mount({ cards, recents: [recent], onOpenTask: openTask, onOpenSurface: openSurface, onQuick: quick })

    expect(root.querySelector("h1")?.textContent).toBe("What do you want to set up?")
    click('[data-home-quick="time:20:0"]')
    click('[data-home-open="party"]')
    click('[data-home-recent="more:shrine.status"]')

    expect(quick).toHaveBeenCalledWith(cards[0].quick[0])
    expect(openTask).toHaveBeenCalledWith("party")
    expect(openSurface).toHaveBeenCalledWith(recent)
    expect(root.querySelector('[data-home-task="party"] .home-chips')).toBeNull()
  })

  it("explains an empty recent list and reports the last quick action", () => {
    mount({ cards, recents: [], feedback: { success: false, text: "Set Calendar Time failed: rejected" } })

    expect(root.querySelector(".home-recents")?.textContent).toContain("Nothing yet")
    expect(root.querySelector("[data-home-feedback]")?.textContent).toContain("rejected")
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
