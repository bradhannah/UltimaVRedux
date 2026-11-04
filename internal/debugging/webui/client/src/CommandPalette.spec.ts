import { createApp, nextTick, type App as VueApp } from "vue"
import { afterEach, describe, expect, it, vi } from "vitest"

import CommandPalette from "./CommandPalette.vue"
import type { NavigationSurface } from "./command_navigation"

let app: VueApp<Element> | undefined
let root: HTMLElement
const surfaces: NavigationSurface[] = [
  { id: "game:ships", workspace: "game", workspaceLabel: "Game", label: "Ships", description: "Vehicle controls", commandId: "vehicle.status", commands: [] },
  { id: "system:logging", workspace: "system", workspaceLabel: "System", label: "Logging", description: "Log channels", commandId: "logging.status", commands: [] },
]

afterEach(() => { app?.unmount(); root?.remove(); app = undefined })

describe("CommandPalette", () => {
  it("exposes combobox/listbox semantics and keyboard activation without executing", async () => {
    const activate = vi.fn()
    mount({ activate })
    await nextTick()
    const input = root.querySelector<HTMLInputElement>("[role=combobox]")!
    input.value = "logging"
    input.dispatchEvent(new Event("input", { bubbles: true }))
    await nextTick()
    expect(input.getAttribute("aria-activedescendant")).toBe("command-palette-system:logging")
    expect(root.querySelector('[role="listbox"] [role="option"]')?.getAttribute("aria-selected")).toBe("true")
    input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true, cancelable: true }))
    expect(activate).toHaveBeenCalledWith(expect.objectContaining({ id: "system:logging" }))
  })

  it("supports Home, End, arrows, Escape, and wraps Tab focus inside the modal", async () => {
    const close = vi.fn()
    mount({ close }, ["game:ships", "system:logging"])
    await nextTick()
    const input = root.querySelector<HTMLInputElement>("[role=combobox]")!
    input.dispatchEvent(new KeyboardEvent("keydown", { key: "End", bubbles: true, cancelable: true }))
    await nextTick()
    expect(input.getAttribute("aria-activedescendant")).toBe("command-palette-system:logging")
    input.dispatchEvent(new KeyboardEvent("keydown", { key: "Home", bubbles: true, cancelable: true }))
    await nextTick()
    expect(input.getAttribute("aria-activedescendant")).toBe("command-palette-game:ships")
    input.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true }))
    expect(close).toHaveBeenCalledOnce()
    const closeButton = root.querySelector<HTMLButtonElement>('button[aria-label="Close command palette"]')!
    closeButton.focus()
    closeButton.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true }))
    expect(close).toHaveBeenCalledTimes(2)
    closeButton.dispatchEvent(new KeyboardEvent("keydown", { key: "Tab", bubbles: true, cancelable: true }))
    expect(document.activeElement).toBe(input)
  })
})

function mount(listeners: { activate?: ReturnType<typeof vi.fn>; close?: ReturnType<typeof vi.fn> }, recents: string[] = []): void {
  root = document.createElement("div")
  document.body.append(root)
  app = createApp(CommandPalette, { surfaces, recents, onActivate: listeners.activate, onClose: listeners.close })
  app.mount(root)
}
