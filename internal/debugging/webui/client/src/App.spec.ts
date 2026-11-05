import { createApp, nextTick, type App as VueApp } from "vue"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import App from "./App.vue"
import { pageDefinitions } from "./workbench_tasks"
import initialCatalog from "./initial_catalog.fixture.json"
import { sessionReady, successfulQueryCompletion, calendarStatusCompletion, testSetupStatusCompletion, inventoryLockerCompletion } from "./app_test_fixtures"

type Listener = (event: { data?: string; code?: number; reason?: string }) => void

class MockWebSocket {
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSED = 3
  static instances: MockWebSocket[] = []

  readonly sent: string[] = []
  readonly listeners = new Map<string, Listener[]>()
  readyState = MockWebSocket.CONNECTING

  constructor(readonly url: string) {
    MockWebSocket.instances.push(this)
  }

  addEventListener(type: string, listener: Listener): void {
    const listeners = this.listeners.get(type) ?? []
    listeners.push(listener)
    this.listeners.set(type, listeners)
  }

  send(message: string): void {
    if (this.readyState !== MockWebSocket.OPEN) throw new Error("socket is not open")
    this.sent.push(message)
  }

  close(code = 1000, reason = ""): void {
    if (this.readyState === MockWebSocket.CLOSED) return
    this.readyState = MockWebSocket.CLOSED
    this.emit("close", { code, reason })
  }

  open(): void {
    this.readyState = MockWebSocket.OPEN
    this.emit("open", {})
  }

  message(message: object): void {
    this.emit("message", { data: JSON.stringify(message) })
  }

  private emit(type: string, event: { data?: string; code?: number; reason?: string }): void {
    for (const listener of this.listeners.get(type) ?? []) listener(event)
  }
}

let app: VueApp<Element> | undefined
let root: HTMLElement

beforeEach(() => {
  MockWebSocket.instances = []
  localStorage.clear()
  vi.useFakeTimers()
  vi.stubGlobal("WebSocket", MockWebSocket)
  vi.stubGlobal("matchMedia", () => ({
    matches: false,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }))
  root = document.createElement("div")
  document.body.append(root)
})

afterEach(() => {
  app?.unmount()
  app = undefined
  root.remove()
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe("debug workbench connection", () => {
  it("polls Combat exactly once per interval whether selected or hidden, including catalog refresh", async () => {
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(formCatalogSnapshot())
    await flushUI()
    connection.message(activeCombatCompletion(executionForCommand(connection, "combat.status").id, "combat-initial", false))
    await openWorkspace("game")
    for (const selected of ["combat.status", "inventory.locker.status", "party.character.update"]) {
      await selectTool(selected)
      await flushUI()
      const last = executionForCommand(connection, "combat.status")
      connection.message(activeCombatCompletion(last.id, `combat-${selected}`, false))
      await flushUI()
      const before = executionsForCommand(connection, "combat.status").length
      vi.advanceTimersByTime(1000)
      await flushUI()
      expect(executionsForCommand(connection, "combat.status")).toHaveLength(before + 1)
      connection.message(activeCombatCompletion(executionForCommand(connection, "combat.status").id, `polled-${selected}`, true))
      connection.message(formCatalogSnapshot())
      await flushUI()
      expect(executionsForCommand(connection, "combat.status")).toHaveLength(before + 1)
    }
  })

	it("opens Home on every launch while preserving the saved instrument and history state", async () => {
		localStorage.setItem("u5r-debug-workbench-workspace", "game")
		localStorage.setItem("u5r-debug-workbench-instrument", "shrine.status")
		localStorage.setItem("u5r-debug-workbench-history-collapsed", "true")
		mountApp()
		const connection = MockWebSocket.instances[0]
		connection.open()
		connection.message(sessionReady("session-1"))
		connection.message(formCatalogSnapshot())
		await flushUI()

		expect(root.querySelector('[data-workspace="home"]')?.getAttribute("aria-current")).toBe("page")
		expect(root.querySelector("[data-home-workspace]")).not.toBeNull()
		expect(root.querySelector(".instrument-title-row")).toBeNull()
		await openWorkspace("more")
		expect(root.querySelector('[data-command-select="shrine.status"]')?.getAttribute("aria-current")).toBe("true")
		expect(root.querySelector(".instrument-title-row")?.textContent).toContain("Shrine Progress")
		expect(root.querySelector(".instrument-title-row .execution-mode")?.textContent).toContain("LIVE STATE + EXPLICIT ALTERATION")
		expect(root.querySelector(".history-panel")?.classList.contains("collapsed")).toBe(true)
		expect(root.querySelector(".history-panel .stream-actions button")?.textContent).toBe("EXPAND HISTORY")
	})

	it("persists instrument and history changes without persisting the active workspace", async () => {
		mountApp()
		const connection = MockWebSocket.instances[0]
		connection.open()
		connection.message(sessionReady("session-1"))
		connection.message(formCatalogSnapshot())
		await flushUI()
		await openWorkspace("more")
		await selectTool("shrine.status")
		click(".history-panel .stream-actions button")
		await flushUI()

		expect(localStorage.getItem("u5r-debug-workbench-workspace")).toBeNull()
		expect(localStorage.getItem("u5r-debug-workbench-instrument")).toBe("shrine.status")
		expect(localStorage.getItem("u5r-debug-workbench-history-collapsed")).toBe("true")
	})

	it("opens a recent Home surface and automatically loads its read-only query", async () => {
		localStorage.setItem("u5r-debug-workbench-navigation-recents", JSON.stringify(["more:shrine.status"]))
		mountApp()
		const connection = MockWebSocket.instances[0]
		connection.open()
		connection.message(sessionReady("session-1"))
		connection.message(formCatalogSnapshot())
		await flushUI()

		expect(root.querySelector('[data-home-recent="more:shrine.status"]')?.textContent).toContain("Shrine Progress")
		const sentBeforeNavigation = connection.sent.length
		click('[data-home-recent="more:shrine.status"]')
		await flushUI()

		expect(root.querySelector('[data-workspace="more"]')?.getAttribute("aria-current")).toBe("page")
		expect(root.querySelector('[data-command-select="shrine.status"]')?.getAttribute("aria-current")).toBe("true")
		expect(connection.sent).toHaveLength(sentBeforeNavigation + 1)
		expect(executionForCommand(connection, "shrine.status").payload.arguments).toEqual({})
	})

	it("shows an explicit failure when an automatically loaded reference query fails", async () => {
		mountApp()
		const connection = MockWebSocket.instances[0]
		connection.open()
		connection.message(sessionReady("session-1"))
		connection.message(formCatalogSnapshot())
		await flushUI()
		await openWorkspace("more")
		await selectTool("shrine.status")
		await flushUI()
		const query = executionForCommand(connection, "shrine.status")
		connection.message({
			version: 1, id: query.id, type: "command.failed", payload: {
				sessionId: "session-1", requestId: query.id, commandId: "shrine.status", sceneGeneration: 1,
				success: false, errorCode: "reference_load_failed", errorMessage: "missing reference data",
			},
		})
		await flushUI()

		expect(root.querySelector("[data-query-load-failure]")?.textContent).toContain("reference_load_failed :: missing reference data")
	})

	it("owns the three observation workflows in one typed pane", async () => {
		mountApp()
		const connection = MockWebSocket.instances[0]
		connection.open()
		connection.message(sessionReady("session-1"))
		connection.message(formCatalogSnapshot())
		await flushUI()
		await openWorkspace("more")

		const visibleCommandIDs = [...root.querySelectorAll<HTMLElement>("[data-command-select]")].map((element) => element.dataset.commandSelect)
		expect(visibleCommandIDs.filter((commandID) => commandID === "command-test.telescope")).toHaveLength(1)
		expect(visibleCommandIDs).not.toContain("command-test.spyglass")
		expect(visibleCommandIDs).not.toContain("command-test.crystal-ball")

		await selectTool("command-test.telescope")
		await flushUI()
		expect(root.querySelector('[data-command-select="command-test.telescope"]')?.textContent).toContain("Observation")
		expect(root.querySelectorAll("[data-observation-command]")).toHaveLength(3)

		click('[data-observation-command="command-test.telescope"]')
		await flushUI()
		const telescope = executionForCommand(connection, "command-test.telescope")
		expect(telescope.payload.arguments).toEqual({})
		expect([...root.querySelectorAll<HTMLButtonElement>("[data-observation-command]")].every((button) => button.disabled)).toBe(true)

		connection.message(observationCompletion(telescope.id, "command-test.telescope", "sun_damage"))
		await flushUI()
		expect(root.querySelector(".observation-telemetry")?.textContent).toContain("sun_damage")
		expect(root.querySelector(".observation-telemetry")?.textContent).toContain("Day 12, Month 3, 139 · 22:15")
		expect(root.querySelector(".observation-telemetry")?.textContent).toContain("Lord British's Castle")
		expect(root.querySelector(".observation-telemetry")?.textContent).toContain("Floor0")
		expect(root.querySelector(".observation-telemetry")?.textContent).toContain("final interaction continues in the game window")

		click('[data-observation-command="command-test.spyglass"]')
		const spyglass = executionForCommand(connection, "command-test.spyglass")
		expect(spyglass.payload.arguments).toEqual({})
		connection.message(observationCompletion(spyglass.id, "command-test.spyglass", "night_sky"))
		await flushUI()
		click('[data-observation-command="command-test.crystal-ball"]')
		expect(executionForCommand(connection, "command-test.crystal-ball").payload.arguments).toEqual({})
	})

  it("ignores stale socket events after a manual retry", async () => {
    mountApp()
    const first = MockWebSocket.instances[0]
    first.open()
    first.close(1006, "connection lost")
    await flushUI()

    click("[data-retry]")
    await flushUI()
    const second = MockWebSocket.instances[1]
    first.message(sessionReady("stale-session"))
    expect(root.textContent).not.toContain("Scene 7")

    second.open()
    second.message(sessionReady("active-session"))
    await flushUI()
    expect(root.textContent).toContain("Scene 7")
  })

  it("deduplicates accepted and completion replays while acknowledging every replayed delivery", async () => {
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(catalogSnapshot())
    await flushUI()
    await openWorkspace("system")

    click(".instrument button")
    await flushUI()
    const request = JSON.parse(connection.sent.at(-1) ?? "{}") as { id: string }
		const accepted = { version: 1, id: request.id, type: "command.accepted", payload: { requestId: request.id, commandId: "logging.status" } }
		connection.message(accepted)
		connection.message(accepted)
    const completion = commandCompletion(request.id)
    connection.message(completion)
    connection.message(completion)
    await flushUI()

    expect(root.querySelectorAll(".result-entry")).toHaveLength(0)
    const acknowledgements = connection.sent.filter((message) => JSON.parse(message).type === "command.ack")
    expect(acknowledgements).toHaveLength(2)
  })

  it("reports unresolved commands when the previous session cannot resume", async () => {
    mountApp()
    const first = MockWebSocket.instances[0]
    first.open()
    first.message(sessionReady("session-1"))
    first.message(catalogSnapshot())
    await flushUI()
    await openWorkspace("system")
    click(".instrument button")
    await flushUI()

    first.close(1006, "connection lost")
    await flushUI()
    click("[data-retry]")
    await flushUI()
    const second = MockWebSocket.instances[1]
    second.open()
    second.message(sessionReady("session-2", false))
    await flushUI()

    expect(root.textContent).toContain("session_lost")
    expect(root.textContent).toContain("outcome is unknown")
  })

  it("retains pending commands while a session resumes", async () => {
		mountApp()
		const first = MockWebSocket.instances[0]
		first.open()
		first.message(sessionReady("session-1"))
		first.message(catalogSnapshot())
		await flushUI()
		await openWorkspace("system")
		click(".instrument button")
		await flushUI()

		first.close(1006, "connection lost")
		await flushUI()
		click("[data-retry]")
		const second = MockWebSocket.instances[1]
		second.open()
		second.message(sessionReady("session-1", true))
		await flushUI()

		expect(root.querySelector<HTMLButtonElement>(".instrument button")?.disabled).toBe(true)
		expect(second.sent.some((message) => JSON.parse(message).type === "operation.status")).toBe(true)
	})

  it("prevents F5 refresh when the focused command is pending", async () => {
		mountApp()
		const connection = MockWebSocket.instances[0]
		connection.open()
		connection.message(sessionReady("session-1"))
		connection.message(catalogSnapshot())
		await flushUI()
		await openWorkspace("system")
		const button = root.querySelector<HTMLButtonElement>(".instrument button")
		if (!button) throw new Error("missing command button")
		button.focus()
		button.click()
		await flushUI()
		const sentCount = connection.sent.length
		const event = new KeyboardEvent("keydown", { key: "F5", bubbles: true, cancelable: true })

		window.dispatchEvent(event)

		expect(event.defaultPrevented).toBe(true)
		expect(connection.sent).toHaveLength(sentCount)
	})

  it("runs the active instrument with F5 outside the form", async () => {
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(catalogSnapshot())
    await flushUI()
    await openWorkspace("more")
    root.querySelector<HTMLButtonElement>("[data-command-palette-launcher]")?.focus()
    const event = new KeyboardEvent("keydown", { key: "F5", bubbles: true, cancelable: true })

    window.dispatchEvent(event)
    await flushUI()

    expect(event.defaultPrevented).toBe(true)
    expect(latestCommandExecution(connection).payload.arguments).toEqual({})
  })

  it("prevents F5 without defaulting the Observation pane to Telescope", async () => {
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(formCatalogSnapshot())
    await flushUI()
    await openWorkspace("more")
    await selectTool("command-test.telescope")
    await flushUI()
    root.querySelector<HTMLButtonElement>('[data-observation-command="command-test.spyglass"]')?.focus()
    const sentCount = connection.sent.length
    const event = new KeyboardEvent("keydown", { key: "F5", bubbles: true, cancelable: true })

    window.dispatchEvent(event)

    expect(event.defaultPrevented).toBe(true)
    expect(connection.sent).toHaveLength(sentCount)
    expect(executionsForCommand(connection, "command-test.telescope")).toHaveLength(0)
  })

  it("leaves modified F5 browser shortcuts untouched", async () => {
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(catalogSnapshot())
    await flushUI()
    const sentCount = connection.sent.length
    const event = new KeyboardEvent("keydown", { key: "F5", ctrlKey: true, bubbles: true, cancelable: true })

    window.dispatchEvent(event)

    expect(event.defaultPrevented).toBe(false)
    expect(connection.sent).toHaveLength(sentCount)
  })

  it("closes a connection that never completes its opening handshake", async () => {
		mountApp()
		const connection = MockWebSocket.instances[0]

		vi.advanceTimersByTime(5000)
		await flushUI()

		expect(connection.readyState).toBe(MockWebSocket.CLOSED)
    expect(root.textContent).toContain("Retry now")
  })

  it("refreshes the recovery deadline when operation status is confirmed", async () => {
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(catalogSnapshot())
    await flushUI()
    await openWorkspace("system")
    click(".instrument button")
    await flushUI()
    const execution = [...connection.sent]
      .map((message) => JSON.parse(message) as { id: string; type: string })
      .find((message) => message.type === "command.execute")
    if (!execution) throw new Error("missing command execution")

    vi.advanceTimersByTime(10000)
    connection.message({
      version: 1,
      id: execution.id,
      type: "command.accepted",
      payload: { requestId: execution.id, commandId: "logging.status" },
    })
    vi.advanceTimersByTime(25000)
    await flushUI()

    expect(connection.readyState).toBe(MockWebSocket.OPEN)
  })

  it("lists only owner tools on More and hides companion operations", async () => {
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(formCatalogSnapshot())
    await flushUI()
    await openWorkspace("more")

		const commandIDs = [...root.querySelectorAll<HTMLElement>("[data-command-select]")].map((button) => button.dataset.commandSelect)
		expect(commandIDs).toEqual([
			"quest.item.status",
			"search.status",
			"shrine.status",
			"monster.ecosystem.status",
			"vehicle.status",
			"merchant.tester.catalog",
			"command-test.telescope",
			"test-setup.status",
			"guards.status",
			"logging.status",
			"debug.options.status",
		])
		expect([...root.querySelectorAll(".tool-group h2")].map((heading) => heading.textContent)).toEqual(["World extras", "For developers"])
		expect(commandIDs).not.toContain("command-test.spyglass")
		expect(commandIDs).not.toContain("command-test.crystal-ball")
    expect(commandIDs).not.toContain("environment.wind.set")
    expect(commandIDs).not.toContain("party.character.inspect")
		expect(commandIDs).not.toContain("party.equipment.loadout.update")
		expect(commandIDs).not.toContain("party.maximize-levels")
		expect(commandIDs).not.toContain("party.fill")
		expect(commandIDs).not.toContain("party.member.add")
    expect(commandIDs).not.toContain("calendar.set-time")
    expect(commandIDs).not.toContain("search.progress.reset")
    expect(commandIDs).not.toContain("quest.item.remove")
		expect(commandIDs).not.toContain("world.position.set")
    expect(commandIDs).not.toContain("moongate.status")
    expect(commandIDs).not.toContain("moongate.travel")
    expect(commandIDs).not.toContain("combat.encounter.start")
    expect(commandIDs).not.toContain("overlay.lighting.set")
    expect(commandIDs).not.toContain("monster.clear-map-enemies")
    expect(commandIDs).not.toContain("ship.damage-raw")
		expect(commandIDs).not.toContain("ship.skiff.set")
		expect(commandIDs).not.toContain("movement.free-move.set")
		expect(commandIDs).not.toContain("shadowlord.status")
    expect(commandIDs).not.toContain("death.status")
		expect(commandIDs).not.toContain("movement.landmark.travel")
		expect(commandIDs).not.toContain("party.karma.set")
		expect(commandIDs).not.toContain("command-test.hole-up")
		expect(commandIDs).not.toContain("command-test.search")
		expect(commandIDs).not.toContain("logging.categories.set-all")
		expect(commandIDs).not.toContain("merchant.tester.preview.open")
		expect(commandIDs).not.toContain("merchant.tester.live.open")
  })

  it("opens a navigation-only palette with Ctrl+K or Meta+K from every workspace", async () => {
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(formCatalogSnapshot())
    await flushUI()

    for (const workspace of ["system", "saves"] as const) {
      await openWorkspace(workspace)
      expect(root.querySelector("[data-command-palette-launcher]")).not.toBeNull()
    }

    const sentBeforePalette = connection.sent.length
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "k", metaKey: true, bubbles: true, cancelable: true }))
    await flushUI()
    const input = root.querySelector<HTMLInputElement>("#command-palette-input")!
    expect(document.activeElement).toBe(input)
    expect(root.querySelector<HTMLElement>(".topbar")?.inert).toBe(true)
    setInputValue(input, "Logging")
    await flushUI()
    input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true, cancelable: true }))
    await flushUI()

    expect(root.querySelector('[data-workspace="more"]')?.getAttribute("aria-current")).toBe("page")
    expect(root.querySelector(".instrument-title-row")?.textContent).toContain("Logging")
    expect(connection.sent).toHaveLength(sentBeforePalette + 1)
    expect(executionForCommand(connection, "logging.status").payload.arguments).toEqual({})
    expect(root.querySelector("[data-command-palette]")).toBeNull()
    expect(root.querySelector<HTMLElement>(".topbar")?.inert).toBe(false)

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "k", ctrlKey: true, bubbles: true, cancelable: true }))
    await flushUI()
    expect(root.querySelector("[data-command-palette]")).not.toBeNull()
  })

  it("moves from keyboard command activation into the specialized logging pane", async () => {
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(formCatalogSnapshot())
    await flushUI()
    await openWorkspace("more")
    const loggingCommand = root.querySelector<HTMLButtonElement>('button[data-command-select="logging.status"]')
    if (!loggingCommand) throw new Error("missing logging command")
    loggingCommand.focus()

    loggingCommand.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true, cancelable: true }))
    await flushUI()

    expect(root.querySelector(".instrument-title-row")?.textContent).toContain("Logging")
    expect(root.querySelector('[aria-label="Logging console"]')).not.toBeNull()
  })

  it("associates request protocol failures with the selected command", async () => {
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(catalogSnapshot())
    await flushUI()
    await openWorkspace("system")
    click(".instrument button")
    const execution = latestCommandExecution(connection)

    connection.message({
      version: 1,
      id: execution.id,
      type: "protocol.error",
      payload: { code: "invalid_request", message: "Rejected by the command catalog." },
    })
    await flushUI()

    expect(root.querySelector(".operation-banner")?.textContent).toContain("Logging Status")
    expect(root.querySelector(".operation-banner")?.textContent).toContain("Rejected by the command catalog.")
  })

  it("shows the latest operation in a compact instrument banner", async () => {
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(formCatalogSnapshot())
    connection.message(shrineCompletion("shrine-result"))
    connection.message(commandCompletion("logging-result"))
    await flushUI()
    await selectTool("logging.status")
    expect(root.querySelector(".operation-banner")?.textContent).toContain("Logging status loaded.")
    expect(root.querySelector(".operation-banner")?.textContent).not.toContain("Humility")
    expect(root.querySelectorAll(".result-entry")).toHaveLength(2)
    expect(root.querySelector('[data-history-request="logging-result"]')?.getAttribute("data-state")).toBe("success")
  })

  it("caps global history and reports trimmed results", async () => {
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(catalogSnapshot())
    await flushUI()
    await openWorkspace("system")
    for (let index = 0; index <= 100; index += 1) connection.message(commandCompletion(`result-${index}`))
    await flushUI()

    expect(root.querySelectorAll(".result-entry")).toHaveLength(100)
    expect(root.textContent).toContain("1 OLDER RESULTS TRIMMED")
    expect(root.querySelector(".operation-banner")?.textContent).toContain("Logging status loaded.")
    click(".stream-actions button:last-child")
    await flushUI()
    expect(root.querySelectorAll(".result-entry")).toHaveLength(0)
    expect(root.querySelector(".operation-banner")).toBeNull()
  })

  it("collapses and vertically resizes global history", async () => {
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(catalogSnapshot())
    await flushUI()

    click(".stream-actions button:first-child")
    await flushUI()
    expect(root.querySelector(".history-panel")?.classList.contains("collapsed")).toBe(true)
    expect((root.querySelector(".result-stream") as HTMLElement).style.display).toBe("none")
    click(".stream-actions button:first-child")
    await flushUI()

    root.querySelector(".history-resize-handle")?.dispatchEvent(new MouseEvent("pointerdown", { clientY: 300, bubbles: true }))
    window.dispatchEvent(new MouseEvent("pointermove", { clientY: 200, bubbles: true }))
    window.dispatchEvent(new MouseEvent("pointerup", { bubbles: true }))
    await flushUI()
    expect((root.querySelector(".history-panel") as HTMLElement).style.height).toBe("320px")
  })

  it("persists right docking and resizes global history horizontally", async () => {
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(catalogSnapshot())
    await flushUI()

    click('[data-history-action="dock"]')
    await flushUI()
    expect(root.querySelector(".workbench-shell")?.classList.contains("history-dock-right")).toBe(true)
    expect(localStorage.getItem("u5r-debug-workbench-history-dock")).toBe("right")
    expect(root.querySelector(".history-resize-handle")?.getAttribute("aria-orientation")).toBe("vertical")
    expect(root.querySelector('[data-history-action="dock"]')?.textContent).toBe("DOCK BOTTOM")

    root.querySelector(".history-resize-handle")?.dispatchEvent(new MouseEvent("pointerdown", { clientX: 500, bubbles: true }))
    window.dispatchEvent(new MouseEvent("pointermove", { clientX: 400, bubbles: true }))
    window.dispatchEvent(new MouseEvent("pointerup", { bubbles: true }))
    await flushUI()
    expect((root.querySelector(".history-panel") as HTMLElement).style.width).toBe("520px")

    click('[data-history-action="dock"]')
    await flushUI()
    expect(root.querySelector(".workbench-shell")?.classList.contains("history-dock-bottom")).toBe(true)
    expect(localStorage.getItem("u5r-debug-workbench-history-dock")).toBe("bottom")
    expect(root.querySelector(".history-resize-handle")?.getAttribute("aria-orientation")).toBe("horizontal")
  })

  it("restores the saved global history dock", async () => {
    localStorage.setItem("u5r-debug-workbench-history-dock", "right")
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(catalogSnapshot())
    await flushUI()

    expect(root.querySelector(".workbench-shell")?.classList.contains("history-dock-right")).toBe(true)
    expect((root.querySelector(".history-panel") as HTMLElement).style.width).toBe("420px")
  })

	it("owns camping and explicitly confirms reviewed death test setups", async () => {
		mountApp()
		const connection = MockWebSocket.instances[0]
		connection.open()
		connection.message(sessionReady("session-1"))
		connection.message(formCatalogSnapshot())
		await flushUI()
		await openWorkspace("more")
		await selectTool("test-setup.status")
		await flushUI()
		const status = executionForCommand(connection, "test-setup.status")
		connection.message(testSetupStatusCompletion(status.id))
		await flushUI()

		expect(root.querySelector(".test-setups")?.textContent).toContain("Test Setups")
		expect(root.querySelector(".test-setups-note")?.textContent).toContain("BUILT-IN UTILITIES")
		root.querySelector<HTMLButtonElement>(".hole-up-card button")?.click()
		const holeUpExecution = executionForCommand(connection, "command-test.hole-up")
		expect(holeUpExecution.payload.arguments).toEqual({
			hours: 8, forceWanderer: false, forceAmbush: false, expectedPartyFingerprint: "party-1",
		})
		const statusRefreshes = executionsForCommand(connection, "test-setup.status").length
		connection.message(successfulMutationCompletion(holeUpExecution.id, "command-test.hole-up", "workflow"))
		await flushUI()
		expect(executionsForCommand(connection, "test-setup.status")).toHaveLength(statusRefreshes + 1)
		const refreshedStatus = executionForCommand(connection, "test-setup.status")
		connection.message(testSetupStatusCompletion(refreshedStatus.id))
		await flushUI()
		const rawCommandIDs = Array.from(root.querySelectorAll<HTMLElement>('[data-command-select]')).map((element) => element.dataset.commandSelect)
		expect(rawCommandIDs).not.toContain("death.kill-party")
		expect(rawCommandIDs).not.toContain("death.test-sequence")
		const killButton = root.querySelector<HTMLButtonElement>(".death-wipe-card button")
		killButton?.click()
		await flushUI()
		expect(killButton?.textContent).toContain("CONFIRM DESTRUCTIVE WIPE")
		killButton?.click()
		const kill = executionForCommand(connection, "death.kill-party")
		expect(kill.payload.arguments).toEqual({ expectedFingerprint: "wipe-1" })
		const statusRefreshesBeforeWipe = executionsForCommand(connection, "test-setup.status").length
		connection.message(successfulMutationCompletion(kill.id, "death.kill-party", "synchronized_live"))
		await flushUI()
		expect(executionsForCommand(connection, "test-setup.status")).toHaveLength(statusRefreshesBeforeWipe + 1)
		expect(executionsForCommand(connection, "death.status")).toHaveLength(1)

		const sequenceButton = root.querySelector<HTMLButtonElement>(".death-sequence-card button")
		sequenceButton?.click()
		await flushUI()
		expect(sequenceButton?.textContent).toContain("CONFIRM WIPE + WORKFLOW")
		sequenceButton?.click()
		expect(executionForCommand(connection, "death.test-sequence").payload.arguments).toEqual({ expectedFingerprint: "wipe-1" })
	})

	it("owns all snapshot commands in the Saves workspace and refreshes scene-bound status after load", async () => {
		mountApp()
		const connection = MockWebSocket.instances[0]
		connection.open()
		connection.message(sessionReady("session-1"))
		connection.message(formCatalogSnapshot())
		await flushUI()
		const initialCalendar = executionForCommand(connection, "calendar.status")
		connection.message(successfulQueryCompletion(initialCalendar.id, "calendar.status", "Calendar.", {}))
		await flushUI()
		await openWorkspace("checkpoints")
		const list = executionForCommand(connection, "save.snapshot.list")
		connection.message(successfulQueryCompletion(list.id, "save.snapshot.list", "Listed.", {
			captureEligibility: { canCapture: true, canLoad: true, canDelete: false },
			snapshots: [{
				reference: "project:before-doom", source: "project", id: "before-doom", name: "Before Doom", description: "Checkpoint",
				createdAt: "2026-08-06T12:00:00Z", gameVersion: "test", revision: "a".repeat(64), compatibility: { compatible: true },
				eligibility: { canCapture: true, canLoad: true, canDelete: true },
			}],
		}))
		await flushUI()
		expect(root.querySelector("[data-saves-workspace]")).not.toBeNull()
		expect(root.querySelectorAll('[data-command-select="save.snapshot.get"]')).toHaveLength(0)
		click('[data-save-list="project"]')
		await flushUI()
		click('[data-save-reference="project:before-doom"] [data-save-action="review-load"]')
		const get = executionForCommand(connection, "save.snapshot.get")
		connection.message(successfulQueryCompletion(get.id, "save.snapshot.get", "Loaded.", { snapshot: {
			summary: {
				reference: "project:before-doom", source: "project", id: "before-doom", name: "Before Doom", description: "Checkpoint",
				createdAt: "2026-08-06T12:00:00Z", gameVersion: "test", revision: "a".repeat(64), compatibility: { compatible: true },
				eligibility: { canCapture: true, canLoad: true, canDelete: true },
			}, snapshotVersion: 1, location: 9, locationName: "New Magincia", x: 1, y: 2, floor: 0, year: 1, month: 1, day: 1, hour: 12, minute: 0,
			partyMembers: 6, moonPhase: "New Moon", moonPhaseIndex: 0,
		} }))
		await flushUI()
		click('[data-save-action="confirm-load"]')
		const load = executionForCommand(connection, "save.snapshot.load")
		const calendarBefore = executionsForCommand(connection, "calendar.status").length
		const savesBefore = executionsForCommand(connection, "save.snapshot.list").length
		connection.message(successfulMutationCompletion(load.id, "save.snapshot.load", "scene_restart"))
		await flushUI()
		expect(executionsForCommand(connection, "calendar.status").length).toBeGreaterThan(calendarBefore)
		expect(executionsForCommand(connection, "save.snapshot.list").length).toBeGreaterThan(savesBefore)
	})

  it("owns comprehensive logging controls in Live and refreshes typed process state", async () => {
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(formCatalogSnapshot())
    await flushUI()

    await openWorkspace("more")
    await selectTool("logging.status")
    await flushUI()
    const status = executionForCommand(connection, "logging.status")
    connection.message(commandCompletion(status.id))
    await flushUI()

    expect(root.querySelector('[aria-label="Logging console"]')?.textContent).toContain("Minimum level")
    click(".logging-summary article:first-child button")
    expect(executionForCommand(connection, "logging.enabled.set").payload.arguments).toEqual({ enabled: false })

    const level = root.querySelector<HTMLSelectElement>("#logging-level")
    if (!level) throw new Error("missing logging level selector")
    level.value = "WARN"
    level.dispatchEvent(new Event("change", { bubbles: true }))
    expect(executionForCommand(connection, "logging.level.set").payload.arguments).toEqual({ level: "WARN" })
  })

  it("enables a logging category after Logging Control is already open", async () => {
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(formCatalogSnapshot())
    await flushUI()

    await openWorkspace("more")
    await selectTool("logging.status")
    await flushUI()
    const status = executionForCommand(connection, "logging.status")
    connection.message(commandCompletion(status.id))
    await flushUI()

    const movement = [...root.querySelectorAll<HTMLElement>(".logging-categories article")]
      .find((article) => article.querySelector("span")?.textContent === "movement")
    movement?.querySelector<HTMLButtonElement>("button")?.click()

    expect(executionForCommand(connection, "logging.category.set").payload.arguments).toEqual({ category: "movement", enabled: true })
  })

  it("shows distinct category states and submits enable-all and disable-all operations", async () => {
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(formCatalogSnapshot())
    await flushUI()

    await openWorkspace("more")
    await selectTool("logging.status")
    await flushUI()
    let status = executionForCommand(connection, "logging.status")
    connection.message(commandCompletion(status.id, { movement: false, combat: true }))
    await flushUI()

    const categories = [...root.querySelectorAll<HTMLElement>(".logging-categories article")]
    expect(categories.find((article) => article.textContent?.includes("movement"))?.classList.contains("is-disabled")).toBe(true)
    expect(categories.find((article) => article.textContent?.includes("combat"))?.classList.contains("is-enabled")).toBe(true)

    click(".logging-category-actions button:first-child")
    let bulk = executionForCommand(connection, "logging.categories.set-all")
    expect(bulk.payload.arguments).toEqual({ enabled: true })
    connection.message(successfulMutationCompletion(bulk.id, "logging.categories.set-all", "live_safe"))
    await flushUI()
    status = executionForCommand(connection, "logging.status")
    connection.message(commandCompletion(status.id, { movement: true, combat: true }))
    await flushUI()

    click(".logging-category-actions button:last-child")
    bulk = executionForCommand(connection, "logging.categories.set-all")
    expect(bulk.payload.arguments).toEqual({ enabled: false })
  })

	 it("keeps explicit Free Move control on Go to and in the Debug Options instrument", async () => {
		mountApp()
		const connection = MockWebSocket.instances[0]
		connection.open()
		connection.message(sessionReady("session-1"))
		connection.message(formCatalogSnapshot())
		await flushUI()

		const status = executionForCommand(connection, "debug.options.status")
		connection.message(successfulQueryCompletion(status.id, "debug.options.status", "Debug options loaded.", {
			freeMove: false,
			textPlaceholdersEnabled: false,
			dialogBoundsEnabled: false,
			unlimitedCastingResources: false,
			bypassSpellLevelRestrictions: false,
			lightingOverlayEnabled: false,
		}))
		await flushUI()
		await openWorkspace("goto")
		connection.message(worldPositionCompletion(executionForCommand(connection, "world.position.status").id))
		await flushUI()
		expect(root.querySelector<HTMLInputElement>("[data-goto-free-move]")?.checked).toBe(false)
		click("[data-goto-free-move]")
		const mutation = executionForCommand(connection, "movement.free-move.set")
		expect(mutation.payload.arguments).toEqual({ enabled: true })
		connection.message(successfulMutationCompletion(mutation.id, "movement.free-move.set", "live_safe"))
		await flushUI()
		const refresh = executionForCommand(connection, "debug.options.status")
		connection.message(successfulQueryCompletion(refresh.id, "debug.options.status", "Debug options loaded.", {
			freeMove: true,
			textPlaceholdersEnabled: false,
			dialogBoundsEnabled: false,
			unlimitedCastingResources: false,
			bypassSpellLevelRestrictions: false,
			lightingOverlayEnabled: false,
		}))
		await flushUI()
		expect(root.querySelector<HTMLInputElement>("[data-goto-free-move]")?.checked).toBe(true)

		await openWorkspace("more")
		await selectTool("debug.options.status")
		await flushUI()
		expect(root.querySelector('[aria-label="Debug options"]')?.textContent).toContain("Text Placeholders")
		expect(root.querySelector('[aria-label="Debug options"]')?.textContent).toContain("Unlimited Casting Resources")
		const switches = root.querySelectorAll<HTMLButtonElement>('[aria-label="Debug options"] [role="switch"]')
		switches[3].dispatchEvent(new MouseEvent("click", { bubbles: true }))
		expect(executionForCommand(connection, "overlay.lighting.set").payload.arguments).toEqual({ enabled: true })
		switches[4].dispatchEvent(new MouseEvent("click", { bubbles: true }))
		const unlimited = executionForCommand(connection, "magic.unlimited-casting-resources.set")
		expect(unlimited.payload.arguments).toEqual({ enabled: true })
	})

	it("owns hidden Guard, Shadowlord, and Death state in World Diagnostics", async () => {
		mountApp()
		const connection = MockWebSocket.instances[0]
		connection.open()
		connection.message(sessionReady("session-1"))
		connection.message(formCatalogSnapshot())
		await flushUI()
		await openWorkspace("more")
		const selector = root.querySelector<HTMLButtonElement>('button[data-command-select="guards.status"]')
		expect(selector?.textContent).toContain("World Diagnostics")
		selector?.click()
		await flushUI()

		const guards = executionForCommand(connection, "guards.status")
		const shadowlords = executionForCommand(connection, "shadowlord.status")
		const death = executionForCommand(connection, "death.status")
		connection.message(successfulQueryCompletion(guards.id, "guards.status", "Guards loaded.", {
			supported: true, locationId: 5, locationName: "Britain", floor: 0, criminal: true,
			friendlyNpcCount: 12, potentialGuardCount: 4, hostilePotentialGuardCount: 3, otherHostileNpcCount: 1,
		}))
		connection.message(successfulQueryCompletion(shadowlords.id, "shadowlord.status", "Shadowlords loaded.", {
			day: 17, aliveCount: 2, allDefeated: false, summoned: true,
			summonedShadowlord: { id: 1, name: "Astaroth", antiVirtue: "Hatred", alive: true },
			shadowlords: [
				{ id: 0, name: "Faulinei", antiVirtue: "Falsehood", alive: false },
				{ id: 1, name: "Astaroth", antiVirtue: "Hatred", alive: true },
				{ id: 2, name: "Nosfentor", antiVirtue: "Cowardice", alive: true },
			],
			townAssignments: [{ locationId: 5, locationName: "Britain", shadowlordId: 1, shadowlordName: "Astaroth" }],
		}))
		connection.message(successfulQueryCompletion(death.id, "death.status", "Death loaded.", {
			allPartyDead: false, activePartyCount: 6, deadPartyCount: 1, karma: 60,
			experienceRetainedPercent: 60, experienceLostPercent: 40,
			minimumPostDeathKarma: 75, minimumPostDeathFood: 63,
			resurrection: { locationId: 1, locationName: "Castle Britannia", x: 10, y: 10, floor: 1 },
		}))
		await flushUI()

		const diagnostics = root.querySelector('[aria-label="Exceptional world-state diagnostics"]')?.textContent
		expect(diagnostics).toContain("Criminal")
		expect(diagnostics).toContain("Astaroth")
		expect(diagnostics).toContain("40%")
		expect(diagnostics).toContain("Castle Britannia")
	})

  it("polls calendar globally while visible without duplicating selected calendar polling", async () => {
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(formCatalogSnapshot())
    await flushUI()
    await openWorkspace("time")

    const initial = executionForCommand(connection, "calendar.status")
    expect(executionsForCommand(connection, "calendar.status")).toHaveLength(1)
    connection.message(calendarStatusCompletion(initial.id))
    await flushUI()

    await selectTool("calendar.status")
    await flushUI()
    const selected = executionForCommand(connection, "calendar.status")
    connection.message(calendarStatusCompletion(selected.id))
    await flushUI()
    const beforeTick = executionsForCommand(connection, "calendar.status").length
    vi.advanceTimersByTime(1000)
    await flushUI()
    expect(executionsForCommand(connection, "calendar.status")).toHaveLength(beforeTick + 1)

    const poll = executionForCommand(connection, "calendar.status")
    connection.message(calendarStatusCompletion(poll.id))
    await flushUI()
    const hidden = vi.spyOn(document, "hidden", "get").mockReturnValue(true)
    vi.advanceTimersByTime(1000)
    await flushUI()
    expect(executionsForCommand(connection, "calendar.status")).toHaveLength(beforeTick + 1)
    hidden.mockRestore()
  })

  it("renders nearby search stacks, adds an item, and submits the armed destructive reset", async () => {
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(formCatalogSnapshot())
    await flushUI()
    await openWorkspace("more")
    await selectTool("search.status")
    await flushUI()
    const statusExecution = executionForCommand(connection, "search.status")
    connection.message(searchStatusCompletion(statusExecution.id))
    await flushUI()

    expect(root.querySelector(".search-inspector")?.textContent).toContain("Britain")
    const addItem = root.querySelector<HTMLSelectElement>(".inline-add-form select")
    const addQuantity = root.querySelector<HTMLInputElement>(".inline-add-form input")
    if (!addItem || !addQuantity) throw new Error("missing search stack controls")
    addItem.value = "gold"
    addItem.dispatchEvent(new Event("change", { bubbles: true }))
    setInputValue(addQuantity, "3")
    await flushUI()
    click(".inline-add-form button[type=submit]")
    expect(executionForCommand(connection, "search.item-stack.add").payload.arguments).toEqual({ x: 42, y: 84, item: "gold", quantity: 3 })

    click(".reset-zone > button")
    await flushUI()
    expect(executionsForCommand(connection, "search.progress.reset")).toHaveLength(0)
    click(".reset-zone .danger-button")
    const resetExecution = executionForCommand(connection, "search.progress.reset")
    expect(resetExecution.payload.arguments).toEqual({ scope: "one_time_locations" })
    const statusCountBeforeReset = executionsForCommand(connection, "search.status").length
    connection.message(successfulMutationCompletion(resetExecution.id, "search.progress.reset", "coordinated_alteration"))
    await flushUI()
    expect(executionsForCommand(connection, "search.status")).toHaveLength(statusCountBeforeReset + 1)
  })

  it("runs production search from the selected directional cell", async () => {
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(formCatalogSnapshot())
    await flushUI()
    await openWorkspace("more")
    await selectTool("search.status")
    await flushUI()
    const statusExecution = executionForCommand(connection, "search.status")
    connection.message(searchStatusCompletion(statusExecution.id, true))
    await flushUI()

    click('.search-cross button[style*="north"]')
    await flushUI()
    click(".search-map-pane > button")
    const searchExecution = executionForCommand(connection, "command-test.search")
    expect(searchExecution.payload.arguments).toEqual({ direction: "up" })
    const searchRefreshes = executionsForCommand(connection, "search.status").length
    connection.message(successfulMutationCompletion(searchExecution.id, "command-test.search", "workflow"))
    await flushUI()
    expect(executionsForCommand(connection, "search.status")).toHaveLength(searchRefreshes + 1)
  })

  it("labels missing quest items GRANT and disables owned items as OWNED", async () => {
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(formCatalogSnapshot())
    await flushUI()
    await openWorkspace("more")
    await selectTool("quest.item.status")
    await flushUI()
    const statusExecution = executionForCommand(connection, "quest.item.status")
    connection.message(questItemStatusCompletion(statusExecution.id))
    await flushUI()

    const questRows = [...root.querySelectorAll<HTMLElement>(".quest-ledger-row:not(.quest-ledger-heading)")]
    expect(questRows[0]?.querySelector<HTMLButtonElement>(".row-actions button:first-child")?.textContent).toBe("GRANT")
    expect(questRows[0]?.querySelector<HTMLButtonElement>(".row-actions button:first-child")?.disabled).toBe(false)
    expect(questRows[1]?.querySelector<HTMLButtonElement>(".row-actions button:first-child")?.textContent).toBe("OWNED")
    expect(questRows[1]?.querySelector<HTMLButtonElement>(".row-actions button:first-child")?.disabled).toBe(true)
    expect(root.querySelector(".quest-inventory")?.textContent).not.toContain("ENSURE")
    questRows[0]?.querySelector<HTMLButtonElement>(".row-actions button:first-child")?.click()
    const ensureExecution = executionForCommand(connection, "quest.item.ensure")
    expect(ensureExecution.payload.arguments).toEqual({ item: "carpet" })
    connection.message(successfulMutationCompletion(ensureExecution.id, "quest.item.ensure", "coordinated_alteration"))
    await flushUI()
    questRows[1]?.querySelector<HTMLButtonElement>(".danger-button")?.click()
    await flushUI()
    expect(executionsForCommand(connection, "quest.item.remove")).toHaveLength(0)
    questRows[1]?.querySelector<HTMLButtonElement>(".danger-button")?.click()
    expect(executionForCommand(connection, "quest.item.remove").payload.arguments).toEqual({ item: "crown" })
  })

  it("owns reviewed shard use, requires confirmation, and sends every stale-state precondition", async () => {
	mountApp()
	const connection = MockWebSocket.instances[0]
	connection.open()
	connection.message(sessionReady("session-1"))
	connection.message(formCatalogSnapshot())
	await flushUI()
	await openWorkspace("commands")
	expect([...root.querySelectorAll<HTMLElement>("[data-command-select]")].map((element) => element.dataset.commandSelect)).not.toContain("quest.shard.use")
	await openWorkspace("more")
	await selectTool("quest.item.status")
	await flushUI()
	connection.message(questItemStatusCompletion(executionForCommand(connection, "quest.item.status").id))
	await flushUI()
	const shardRow = [...root.querySelectorAll<HTMLElement>(".quest-ledger-row:not(.quest-ledger-heading)")]
		.find((row) => row.textContent?.includes("Shard of Hatred"))
	expect([...root.querySelectorAll<HTMLElement>(".quest-ledger-row:not(.quest-ledger-heading)")]
		.filter((row) => row.textContent?.includes("Shard of ")).flatMap((row) => [...row.querySelectorAll("button")])
		.filter((button) => button.textContent === "USE")).toHaveLength(3)
	const use = [...(shardRow?.querySelectorAll<HTMLButtonElement>("button") ?? [])].find((button) => button.textContent === "USE")
	use?.click()
	await flushUI()
	expect(executionsForCommand(connection, "quest.shard.use")).toHaveLength(0)
	shardRow?.querySelector<HTMLButtonElement>("button.armed")?.click()
	expect(executionForCommand(connection, "quest.shard.use").payload.arguments).toEqual({
		shard: "shard_hatred", expectedQuantity: 1, expectedLocationId: 31, expectedX: 15, expectedY: 3,
		expectedFloor: 1, expectedFlame: "love", expectedShadowlordSummoned: true,
		expectedShadowlord: "astaroth", expectedShadowlordAlive: true,
	})
  })

  it("reviews shrine quest-state presets and submits edited shrine progress", async () => {
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(formCatalogSnapshot())
    await flushUI()
    await openWorkspace("commands")
    const rawCommandIDs = [...root.querySelectorAll<HTMLElement>("[data-command-select]")]
      .map((element) => element.dataset.commandSelect)
    expect(rawCommandIDs).not.toContain("codex.quest-state.set-all")
    await openWorkspace("more")
    await selectTool("shrine.status")
    await flushUI()

    for (const statusExecution of executionsForCommand(connection, "shrine.status")) {
      connection.message(shrineStatusCompletion(statusExecution.id))
    }
    await flushUI()
    setControlValue("shrine-progress-selector", "7")
    await flushUI()
    expect(root.querySelector(".shrine-progress-inspector")?.textContent).toContain("AUTHORITATIVE CURRENT GAME STATE")

    const presetExecutionsBefore = executionsForCommand(connection, "codex.quest-state.set-all").length
    click('[data-shrine-preset="completed"]')
    await flushUI()
    expect(executionsForCommand(connection, "codex.quest-state.set-all")).toHaveLength(presetExecutionsBefore)
    expect(root.querySelector(".shrine-preset-review")?.textContent).toContain("Prepare Endgame")
    expect(root.querySelector(".shrine-preset-review")?.textContent).toContain("shrine-review-1")
    click("[data-shrine-preset-confirm]")
    const presetMutation = executionForCommand(connection, "codex.quest-state.set-all")
    expect(presetMutation.payload.arguments).toEqual({ targetState: "completed", expectedFingerprint: "shrine-review-1" })
    for (const statusExecution of executionsForCommand(connection, "shrine.status")) {
      connection.message(shrineStatusCompletion(statusExecution.id))
    }
    await flushUI()
    const statusRefreshes = executionsForCommand(connection, "shrine.status").length
    connection.message(successfulMutationCompletion(presetMutation.id, "codex.quest-state.set-all", "coordinated_alteration"))
    await flushUI()
    expect(executionsForCommand(connection, "shrine.status")).toHaveLength(statusRefreshes + 1)
    const presetRefresh = executionForCommand(connection, "shrine.status")
    connection.message(shrineStatusCompletion(presetRefresh.id))
    await flushUI()
    click('[data-shrine-preset="not_assigned"]')
    await flushUI()
    expect(root.querySelector(".shrine-preset-review")?.textContent).toContain("Clear All")
    click(".shrine-preset-review button:last-of-type")
    await flushUI()

    setControlValue("shrine-quest-state", "offering")
    click('[data-shrine-condition="false"]')
    await flushUI()
    vi.advanceTimersByTime(1000)
    await flushUI()
    const pollExecution = latestCommandExecution(connection)
    connection.message(shrineStatusCompletion(pollExecution.id))
    await flushUI()

    expect(root.querySelector<HTMLSelectElement>("#shrine-quest-state")?.value).toBe("offering")
    click('[data-command-id="shrine.progress.update"]')
    const mutation = latestCommandExecution(connection)
    expect(mutation.payload.arguments).toEqual({ index: "7", questState: "offering", ruined: false })
  })

  it("submits explicit monster generation state and requires confirmation to clear enemies", async () => {
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(formCatalogSnapshot())
    await flushUI()
    await openWorkspace("more")
    await selectTool("monster.ecosystem.status")
    await flushUI()
    const status = executionForCommand(connection, "monster.ecosystem.status")
    connection.message(monsterEcosystemCompletion(status.id))
    await flushUI()
    click(".generation-console .segment-control button:first-child")
    setInputValue(root.querySelector<HTMLInputElement>('.generation-console input[type="number"]'), "25")
    await flushUI()
    click('.generation-console button[type="submit"]')
    expect(executionForCommand(connection, "monster.generation.set").payload.arguments).toEqual({ enabled: true, odds: 25 })

    click(".monster-console .reset-zone > button")
    await flushUI()
    expect(executionsForCommand(connection, "monster.clear-map-enemies")).toHaveLength(0)
    click(".monster-console .reset-zone .danger-button")
    expect(executionForCommand(connection, "monster.clear-map-enemies").payload.arguments).toEqual({
      expectedLocationId: 0,
      expectedFloor: 0,
      expectedEnemyCount: 1,
    })
  })

  it("owns Merchant Tester in Tools, refreshes only on selection or F5, and serializes both operation contracts", async () => {
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(formCatalogSnapshot())
    await flushUI()
    await openWorkspace("more")

    const toolIDs = [...root.querySelectorAll<HTMLElement>("[data-command-select]")].map((element) => element.dataset.commandSelect)
    expect(toolIDs.filter((commandID) => commandID === "merchant.tester.catalog")).toHaveLength(1)
    expect(toolIDs).not.toContain("merchant.tester.preview.open")
    expect(toolIDs).not.toContain("merchant.tester.live.open")

    await selectTool("merchant.tester.catalog")
    await flushUI()
    const initialCatalogQuery = executionForCommand(connection, "merchant.tester.catalog")
    expect(initialCatalogQuery.payload.arguments).toEqual({})
    connection.message(merchantTesterCatalogCompletion(initialCatalogQuery.id))
    await flushUI()
    expect(root.querySelector(".instrument")?.classList.contains("instrument--reference")).toBe(true)
    expect(root.querySelector(".instrument-index")?.textContent).toContain("catalog: query · preview: workflow · live: workflow / coordinated_alteration")
    expect(root.querySelector(".execution-mode")?.textContent).toContain("QUERY + PREVIEW + DESTRUCTIVE LIVE")

    const queryCount = executionsForCommand(connection, "merchant.tester.catalog").length
    vi.advanceTimersByTime(1000)
    await flushUI()
    expect(executionsForCommand(connection, "merchant.tester.catalog")).toHaveLength(queryCount)

    const f5 = new KeyboardEvent("keydown", { key: "F5", bubbles: true, cancelable: true })
    window.dispatchEvent(f5)
    await flushUI()
    expect(f5.defaultPrevented).toBe(true)
    expect(executionsForCommand(connection, "merchant.tester.catalog")).toHaveLength(queryCount + 1)
    expect(executionsForCommand(connection, "merchant.tester.preview.open")).toHaveLength(0)
    expect(executionsForCommand(connection, "merchant.tester.live.open")).toHaveLength(0)
    connection.message(merchantTesterCatalogCompletion(executionForCommand(connection, "merchant.tester.catalog").id))
    await flushUI()

    click('[data-merchant-id="arms.britain"]')
    expect(root.querySelector('[data-merchant-id="arms.britain"]')?.textContent).toContain("READY")
    click("[data-merchant-preview]")
    await flushUI()
    const preview = executionForCommand(connection, "merchant.tester.preview.open")
    expect(preview.payload.arguments).toEqual({ merchantId: "arms.britain" })
    expect(root.querySelector<HTMLButtonElement>("[data-merchant-preview]")?.disabled).toBe(true)
    expect(root.querySelector<HTMLButtonElement>("[data-merchant-live-review]")?.disabled).toBe(false)

    const queriesBeforePreviewCompletion = executionsForCommand(connection, "merchant.tester.catalog").length
    connection.message(successfulMutationCompletion(preview.id, "merchant.tester.preview.open", "workflow"))
    await flushUI()
    connection.message(formCatalogSnapshot())
    await flushUI()
    expect(executionsForCommand(connection, "merchant.tester.catalog")).toHaveLength(queriesBeforePreviewCompletion)
    expect(root.querySelector(".operation-banner")?.textContent).toContain("Open Merchant Preview")

    click("[data-merchant-live-review]")
    await flushUI()
    expect(root.querySelector("[data-merchant-live-review-panel]")?.textContent).toContain("Britain · LOCATION 3")
    click("[data-merchant-live-open]")
    await flushUI()
    const live = executionForCommand(connection, "merchant.tester.live.open")
    expect(live.payload.arguments).toEqual({
      merchantId: "arms.britain",
      expectedLocationId: 3,
      expectedFloor: 2,
      expectedX: 17,
      expectedY: 29,
      expectedSourceFingerprint: "merchant-source-fingerprint-1234567890",
    })
    expect(root.querySelector<HTMLButtonElement>("[data-merchant-live-open]")?.disabled).toBe(true)
    expect(root.querySelector<HTMLButtonElement>("[data-merchant-preview]")?.disabled).toBe(false)

    const queriesBeforeLiveCompletion = executionsForCommand(connection, "merchant.tester.catalog").length
    connection.message(successfulMutationCompletion(live.id, "merchant.tester.live.open", "workflow"))
    await flushUI()
    connection.message(formCatalogSnapshot())
    await flushUI()
    expect(executionsForCommand(connection, "merchant.tester.catalog")).toHaveLength(queriesBeforeLiveCompletion)
  })

  it("keeps a pending Merchant Tester operation owned and suppressed through session resume", async () => {
    mountApp()
    const first = MockWebSocket.instances[0]
    first.open()
    first.message(sessionReady("session-1"))
    first.message(formCatalogSnapshot())
    await flushUI()
    await openWorkspace("more")
    await selectTool("merchant.tester.catalog")
    await flushUI()
    first.message(merchantTesterCatalogCompletion(executionForCommand(first, "merchant.tester.catalog").id))
    await flushUI()
    click('[data-merchant-id="guild.britain"]')
    click("[data-merchant-preview]")
    await flushUI()
    const preview = executionForCommand(first, "merchant.tester.preview.open")

    first.close(1006, "connection lost")
    await flushUI()
    click("[data-retry]")
    const second = MockWebSocket.instances[1]
    second.open()
    second.message(sessionReady("session-1", true))
    second.message(formCatalogSnapshot())
    await flushUI()

    expect(second.sent.some((message) => JSON.parse(message).type === "operation.status")).toBe(true)
    expect(executionsForCommand(second, "merchant.tester.catalog")).toHaveLength(0)
    expect(root.querySelector<HTMLButtonElement>("[data-merchant-preview]")?.disabled).toBe(true)

    second.message(successfulMutationCompletion(preview.id, "merchant.tester.preview.open", "workflow"))
    await flushUI()
    second.message(formCatalogSnapshot())
    await flushUI()
    expect(executionsForCommand(second, "merchant.tester.catalog")).toHaveLength(0)
    expect(root.querySelector(".operation-banner")?.textContent).toContain("Open Merchant Preview")
  })

  it("operates the dedicated Ships instrument", async () => {
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(formCatalogSnapshot())
    await flushUI()
		await openWorkspace("commands")
		const rawCommandIDs = [...root.querySelectorAll<HTMLElement>("[data-command-select]")].map((element) => element.dataset.commandSelect)
		expect(rawCommandIDs).not.toContain("vehicle.spawn")
		expect(rawCommandIDs).not.toContain("vehicle.force-board")
    await openWorkspace("more")
    await selectTool("vehicle.status")
    await flushUI()
    const status = executionForCommand(connection, "vehicle.status")
    connection.message(vehicleStatusCompletion(status.id, 50))
    await flushUI()
		setSelectValue(root.querySelector<HTMLSelectElement>("[data-vehicle-spawn-type]"), "skiff")
		setSelectValue(root.querySelector<HTMLSelectElement>("[data-vehicle-spawn-placement]"), "dock")
		await flushUI()
		setSelectValue(root.querySelector<HTMLSelectElement>("[data-vehicle-spawn-dock]"), "buccaneers_den")
		click('.vehicle-creation-console button[type="submit"]')
		const spawn = executionForCommand(connection, "vehicle.spawn")
		expect(spawn.payload.arguments).toEqual({ type: "skiff", placement: "dock", dock: "buccaneers_den" })
		connection.message(successfulMutationCompletion(spawn.id, "vehicle.spawn", "synchronized_live"))
		await flushUI()
		const spawnRefresh = executionForCommand(connection, "vehicle.status")
		connection.message(vehicleStatusCompletion(spawnRefresh.id, 50))
		await flushUI()

		setSelectValue(root.querySelector<HTMLSelectElement>("[data-vehicle-force-board-type]"), "carpet")
		click(".force-board-console button")
		const forceBoard = executionForCommand(connection, "vehicle.force-board")
		expect(forceBoard.payload.arguments).toEqual({ type: "carpet" })
		connection.message(successfulMutationCompletion(forceBoard.id, "vehicle.force-board", "synchronized_live"))
		await flushUI()
		expect(executionsForCommand(connection, "vehicle.status")).toHaveLength(3)
		const boardRefresh = executionForCommand(connection, "vehicle.status")
		connection.message(vehicleStatusCompletion(boardRefresh.id, 50))
		await flushUI()
    setInputValue(root.querySelector<HTMLInputElement>('.hull-operations input[type="number"]'), "49")
    await flushUI()
    click('.hull-operations form button[type="submit"]')
    expect(executionsForCommand(connection, "ship.damage-raw")).toHaveLength(0)
    click('.hull-operations form button[type="submit"]')
    const damage = executionForCommand(connection, "ship.damage-raw")
    expect(damage.payload.arguments).toEqual({ amount: 49 })
    connection.message(successfulMutationCompletion(damage.id, "ship.damage-raw", "synchronized_live"))
    await flushUI()
    const refresh = executionForCommand(connection, "vehicle.status")
    connection.message(vehicleStatusCompletion(refresh.id, 1))
    await flushUI()
    click(".hull-operations > section button")
    expect(executionForCommand(connection, "ship.repair-full").payload.arguments).toEqual({})
		setInputValue(root.querySelector<HTMLInputElement>('.skiff-operation input[type="number"]'), "12")
		click(".skiff-operation button")
		expect(executionForCommand(connection, "ship.skiff.set").payload.arguments).toEqual({ quantity: 12 })
  })

  it("owns the spell inventory in Inspect, hides companions, and polls silently only while selected", async () => {
    mountApp()
    const connection = MockWebSocket.instances[0]
    connection.open()
    connection.message(sessionReady("session-1"))
    connection.message(spellInventoryCatalogSnapshot())
    await flushUI()

    for (const tab of ["items", "more"] as const) {
      await openWorkspace(tab)
      for (const commandID of ["inventory.spell.set", "inventory.spell.preset-max"]) {
        expect(root.querySelector(`[data-command-select="${commandID}"]`)).toBeNull()
      }
    }

    await openWorkspace("items")
    await selectTool("inventory.spell.status")
    await flushUI()
    const status = executionForCommand(connection, "inventory.spell.status")
    connection.message(spellInventoryCompletion(status.id, "spell-1", 2))
    await flushUI()
    const historyBeforePoll = root.querySelectorAll(".result-entry").length
    vi.advanceTimersByTime(1000)
    await flushUI()
    const poll = executionForCommand(connection, "inventory.spell.status")
    connection.message(spellInventoryCompletion(poll.id, "spell-2", 3))
    await flushUI()
    expect(root.querySelectorAll(".result-entry")).toHaveLength(historyBeforePoll)

    await openWorkspace("items")
    await selectTool("inventory.locker.status")
    await flushUI()
    const spellQueries = executionsForCommand(connection, "inventory.spell.status").length
    vi.advanceTimersByTime(1000)
    await flushUI()
    expect(executionsForCommand(connection, "inventory.spell.status")).toHaveLength(spellQueries)
  })

	it("shows a one-click top bar combat exit backed by the latest reviewed status", async () => {
		mountApp()
		const connection = MockWebSocket.instances[0]
		connection.open()
		connection.message(sessionReady("session-1"))
		connection.message(formCatalogSnapshot())
		await flushUI()

		const status = executionForCommand(connection, "combat.status")
		connection.message(activeCombatCompletion(status.id, "quick-exit-snapshot", false))
		await flushUI()

		const quickExit = root.querySelector<HTMLButtonElement>("[data-quick-combat-exit]")
		expect(quickExit).not.toBeNull()
		expect(quickExit?.disabled).toBe(false)
		quickExit?.click()

		const exit = executionForCommand(connection, "combat.exit-debug")
		expect(exit.payload.arguments).toEqual({ encounterId: "9", expectedSnapshot: "quick-exit-snapshot" })
	})

	it("shows where, when, party and mode in the top bar on every view and marks Fight during combat", async () => {
		mountApp()
		const connection = MockWebSocket.instances[0]
		connection.open()
		connection.message(sessionReady("session-1"))
		connection.message(formCatalogSnapshot())
		await flushUI()
		connection.message(calendarStatusCompletion(executionForCommand(connection, "calendar.status").id))
		connection.message(testSetupStatusCompletion(executionForCommand(connection, "test-setup.status").id))
		connection.message(successfulQueryCompletion(executionForCommand(connection, "combat.status").id, "combat.status", "Combat.", { active: false }))
		await flushUI()

		for (const view of ["home", "party", "more"] as const) {
			await openWorkspace(view)
			expect(root.querySelector("[data-topbar-where]")?.textContent).toContain("Britain")
			expect(root.querySelector("[data-topbar-when]")?.textContent).toContain("Wednesday, July 12, 138 · 4:15 PM")
			expect(root.querySelector("[data-topbar-party]")?.textContent).toContain("1 · all healthy")
			expect(root.querySelector("[data-topbar-mode]")?.textContent).toBe("Exploring")
		}
		expect(root.querySelector('[data-workspace="fight"] .combat-marker')).toBeNull()
		expect([...root.querySelectorAll(".task-tabs button")].map((tab) => tab.textContent?.trim())).toEqual([
			"Home", "Go to", "Party", "Items & spells", "Time & sky", "Fight", "Checkpoints", "More",
		])

		vi.advanceTimersByTime(1000)
		connection.message(activeCombatCompletion(executionForCommand(connection, "combat.status").id, "combat-bar", false))
		await flushUI()
		expect(root.querySelector("[data-topbar-mode]")?.textContent).toBe("In combat")
		expect(root.querySelector('[data-workspace="fight"] .combat-marker')).not.toBeNull()
	})

	it("runs Home quick actions only through existing commands and reports the outcome", async () => {
		mountApp()
		const connection = MockWebSocket.instances[0]
		connection.open()
		connection.message(sessionReady("session-1"))
		connection.message(formCatalogSnapshot())
		await flushUI()

		expect([...root.querySelectorAll("[data-home-task]")].map((card) => (card as HTMLElement).dataset.homeTask)).toEqual([
			"goto", "fight", "party", "items", "time", "checkpoints",
		])
		click('[data-home-quick="landmark:codex"]')
		const travel = executionForCommand(connection, "movement.landmark.travel")
		expect(travel.payload.arguments).toEqual({ landmark: "codex" })
		connection.message(successfulMutationCompletion(travel.id, "movement.landmark.travel", "synchronized_live"))
		await flushUI()
		expect(root.querySelector("[data-home-feedback]")?.classList.contains("success")).toBe(true)

		click('[data-home-quick="time:20:0"]')
		expect(executionForCommand(connection, "calendar.set-time").payload.arguments).toEqual({ hour: 20, minute: 0 })

		// Heal and cure everyone reads the roster first so the reviewed fingerprint is fresh.
		click('[data-home-quick="party-restore"]')
		await flushUI()
		connection.message(partyCharacterInspectCompletion(executionForCommand(connection, "party.character.inspect").id, "roster-fresh"))
		await flushUI()
		expect(executionForCommand(connection, "party.restore").payload.arguments).toEqual({ expectedFingerprint: "roster-fresh" })
	})

	it("keeps every supported tool reachable from a task tab and the palette", async () => {
		mountApp()
		const connection = MockWebSocket.instances[0]
		connection.open()
		connection.message(sessionReady("session-1"))
		connection.message(formCatalogSnapshot())
		await flushUI()

		// Every task tab shows one page; More lists the remaining tools.
		for (const tab of ["goto", "party", "items", "time", "fight", "checkpoints"] as const) {
			await openWorkspace(tab)
			expect(root.querySelector(`[data-task="${tab}"] .instrument-bank`), tab).not.toBeNull()
			expect(root.querySelector("[data-command-select]"), `${tab} has no tool picker`).toBeNull()
		}
		await openWorkspace("more")
		const moreTools = [...root.querySelectorAll<HTMLElement>("[data-command-select]")].map((button) => button.dataset.commandSelect ?? "")
		expect(moreTools).toContain("quest.item.status")

		// Search finds every page, its companion commands and every More tool.
		click("[data-command-palette-launcher]")
		await flushUI()
		const input = root.querySelector<HTMLInputElement>("#command-palette-input")!
		const expectedSurface: Record<string, string> = {}
		for (const [task, page] of Object.entries(pageDefinitions)) {
			for (const commandID of [page!.anchor, ...page!.commands]) expectedSurface[commandID] = `${task}:${page!.anchor}`
		}
		for (const commandID of moreTools) expectedSurface[commandID] = `more:${commandID}`
		for (const [commandID, surface] of Object.entries(expectedSurface)) {
			if (!(initialCatalog as { id: string }[]).some((command) => command.id === commandID)) continue
			setInputValue(input, commandID)
			await flushUI()
			expect(root.querySelector(`[data-palette-surface="${surface}"]`), commandID).not.toBeNull()
		}
	})

	it("applies staged Time & sky changes one command at a time and reports the outcome", async () => {
		mountApp()
		const connection = MockWebSocket.instances[0]
		connection.open()
		connection.message(sessionReady("session-1"))
		connection.message(formCatalogSnapshot())
		await flushUI()
		await openWorkspace("time")
		connection.message(calendarStatusCompletion(executionForCommand(connection, "calendar.status").id))
		connection.message(windCompletion(executionForCommand(connection, "environment.wind.status").id, "Calm"))
		await flushUI()
		click('[data-time-preset="20:00"]')
		click('[data-wind="west"]')
		await flushUI()
		click("[data-change-apply]")
		await flushUI()
		const time = executionForCommand(connection, "calendar.set-time")
		expect(time.payload.arguments).toEqual({ hour: 20, minute: 0 })
		expect(executionsForCommand(connection, "environment.wind.set")).toHaveLength(0)
		connection.message(successfulMutationCompletion(time.id, "calendar.set-time", "live_safe"))
		await flushUI()
		const wind = executionForCommand(connection, "environment.wind.set")
		expect(wind.payload.arguments).toEqual({ direction: "west" })
		connection.message(successfulMutationCompletion(wind.id, "environment.wind.set", "live_safe"))
		await flushUI()
		expect(root.querySelector("[data-page-notice]")?.textContent).toBe("Applied 2 changes.")
	})

	it("stops a staged batch at the first failure and says which change it stopped at", async () => {
		mountApp()
		const connection = MockWebSocket.instances[0]
		connection.open()
		connection.message(sessionReady("session-1"))
		connection.message(formCatalogSnapshot())
		await flushUI()
		await openWorkspace("time")
		connection.message(calendarStatusCompletion(executionForCommand(connection, "calendar.status").id))
		connection.message(windCompletion(executionForCommand(connection, "environment.wind.status").id, "Calm"))
		await flushUI()
		click('[data-time-preset="20:00"]')
		click('[data-wind="west"]')
		await flushUI()
		click("[data-change-apply]")
		await flushUI()
		const time = executionForCommand(connection, "calendar.set-time")
		connection.message(failedCommandCompletion(time.id, "calendar.set-time", "stale_calendar"))
		await flushUI()
		expect(executionsForCommand(connection, "environment.wind.set")).toHaveLength(0)
		expect(root.querySelector("[data-page-notice]")?.textContent).toContain("changed in the game since this page loaded")
	})

	it("arrives outside a dungeon room through world.position.set with the room number", async () => {
		mountApp()
		const connection = MockWebSocket.instances[0]
		connection.open()
		connection.message(sessionReady("session-1"))
		connection.message(formCatalogSnapshot())
		await flushUI()
		await openWorkspace("goto")
		connection.message(worldPositionCompletion(executionForCommand(connection, "world.position.status").id, {
			destinations: [{
				locationId: 33, locationName: "Deceit", mapType: "dungeon", floors: [0, 1],
				bounds: { minimumX: 0, maximumX: 7, minimumY: 0, maximumY: 7 }, defaultSetup: { floor: 0, x: 1, y: 1 },
				rooms: [{ room: 4, label: "Room 4", floor: 1, roomX: 3, roomY: 3, available: true, setup: { floor: 1, x: 2, y: 3 }, facingLabel: "East" }],
			}],
		}))
		await flushUI()
		click('[data-goto-place="dest:33"]')
		await flushUI()
		click('[data-goto-mode="dungeon_outside_room"]')
		await flushUI()
		click('[data-goto-level="1"]')
		await flushUI()
		click("[data-goto-go]")
		await flushUI()
		const mutation = executionForCommand(connection, "world.position.set")
		expect(mutation.payload.arguments).toMatchObject({ placement: "dungeon_outside_room", location: "33", room: 4, expectedSourceFingerprint: "world-source-1" })
	})

})

function mountApp(): void {
  app = createApp(App)
  app.mount(root)
}

function click(selector: string): void {
  const element = root.querySelector<HTMLElement>(selector)
  if (!element) throw new Error(`missing test element: ${selector}\n${root.innerHTML}`)
  element.dispatchEvent(new MouseEvent("click", { bubbles: true }))
}

type OpenableView = "home" | "goto" | "party" | "items" | "time" | "fight" | "checkpoints" | "more"
const toolTabs: Record<string, OpenableView> = {
  "world.position.status": "goto",
  "party.character.inspect": "party",
  "party.character.update": "party",
  "party.equipment.loadout.update": "party",
  "inventory.locker.status": "items",
  "inventory.spell.status": "items",
  "quest.item.status": "more",
  "search.status": "more",
  "calendar.status": "time",
  "environment.wind.status": "time",
  "moongate.status": "time",
  "combat.status": "fight",
  "combat.encounter.options": "fight",
  "save.snapshot.list": "checkpoints",
  "shrine.status": "more",
  "monster.ecosystem.status": "more",
  "vehicle.status": "more",
  "merchant.tester.catalog": "more",
  "command-test.telescope": "more",
  "test-setup.status": "more",
  "guards.status": "more",
  "logging.status": "more",
  "debug.options.status": "more"
}

// Older tests name the workspace a tool used to live in; the tool helpers below open the tab that now holds it.
async function openWorkspace(workspace: OpenableView | "game" | "system" | "saves" | "tools" | "live" | "inspect" | "commands"): Promise<void> {
  const legacy: Record<string, OpenableView> = { game: "items", live: "items", inspect: "items", system: "more", tools: "more", commands: "more", saves: "checkpoints" }
  click(`[data-workspace="${legacy[workspace] ?? workspace}"]`)
  await flushUI()
}

async function selectTool(commandID: string): Promise<void> {
  const selector = `[data-command-select="${commandID}"]`
  if (!root.querySelector(selector)) await openWorkspace(toolTabs[commandID] ?? "more")
  // Tabs with a single tool select it on arrival and show no tool picker.
  if (root.querySelector(selector)) click(selector)
}

function setControlValue(id: string, value: string): void {
  const control = root.querySelector<HTMLInputElement | HTMLSelectElement>(`[id="${id}"]`)
  if (!control) throw new Error(`missing test control: ${id}`)
  control.value = value
  control.dispatchEvent(new Event(control instanceof HTMLSelectElement ? "change" : "input", { bubbles: true }))
}

function setInputValue(control: HTMLInputElement | HTMLTextAreaElement | undefined | null, value: string): void {
  if (!control) throw new Error("missing test input")
  control.value = value
  control.dispatchEvent(new Event("input", { bubbles: true }))
}

function setSelectValue(control: HTMLSelectElement | undefined | null, value: string): void {
  if (!control) throw new Error("missing test select")
  control.value = value
  control.dispatchEvent(new Event("change", { bubbles: true }))
}

async function flushUI(): Promise<void> {
  await Promise.resolve()
  await nextTick()
}

function catalogSnapshot(): object {
  return {
    version: 1,
    id: "catalog",
    type: "catalog.snapshot",
    payload: {
      sceneGeneration: 7,
      optionsRevision: 1,
      commands: [{
        id: "logging.status",
        category: "Diagnostics",
        name: "Logging Status",
        description: "Show logging status.",
        executionModes: ["query"],
        destructive: false,
      }],
    },
  }
}

function formCatalogSnapshot(): object {
  return {
    version: 1,
    id: "catalog",
    type: "catalog.snapshot",
    payload: {
      sceneGeneration: 7,
      optionsRevision: 9,
		commands: initialCatalog,
      optionSets: [{
        provider: "party.active-characters",
        choices: [
          {
            value: "0.QXZhdGFy",
            label: "Position 1 · Avatar · Avatar",
			metadata: { partyPosition: 1, name: "Avatar", class: "Avatar", status: "Good", strength: 20, dexterity: 20, intelligence: 20, currentHp: 100, maximumHp: 100, currentMp: 20, experience: 500, level: 5, karma: 50 },
          },
          {
            value: "1.U2hhbWlubw",
            label: "Position 2 · Shamino · Fighter",
			metadata: { partyPosition: 2, name: "Shamino", class: "Fighter", status: "Good", strength: 18, dexterity: 21, intelligence: 17, currentHp: 82, maximumHp: 90, currentMp: 17, experience: 400, level: 4, karma: 50 },
          },
          {
            value: "2.SW9sbw",
            label: "Position 3 · Iolo · Bard",
			metadata: { partyPosition: 3, name: "Iolo", class: "Bard", status: "Good", strength: 17, dexterity: 22, intelligence: 18, currentHp: 78, maximumHp: 85, currentMp: 9, experience: 350, level: 4, karma: 50 },
          },
        ],
      }, {
        provider: "party.active-positions",
        choices: [
          { value: "1", label: "Position 1 · Avatar", metadata: { partyPosition: 1, name: "Avatar" } },
          { value: "2", label: "Position 2 · Shamino", metadata: { partyPosition: 2, name: "Shamino" } },
          { value: "3", label: "Position 3 · Iolo", metadata: { partyPosition: 3, name: "Iolo" } },
        ],
      }, {
        provider: "merchant.tester.merchants",
        choices: [
          { value: "arms.britain", label: "Britain | Arms | Britanny (Arms Dealer)" },
          { value: "guild.britain", label: "Britain | Guild | Guido (Guild)" },
          { value: "guild.paws", label: "Paws | Guild | Sindar (Guild)" },
        ],
      }],
    },
  }
}

function spellInventoryCatalogSnapshot(): object {
	return formCatalogSnapshot()
}

function latestCommandExecution(connection: MockWebSocket): { id: string; payload: { arguments: Record<string, unknown>; expectedOptionsRevision?: number } } {
  const execution = [...connection.sent]
    .map((message) => JSON.parse(message) as { id: string; type: string; payload: { arguments: Record<string, unknown>; expectedOptionsRevision?: number } })
    .reverse()
    .find((message) => message.type === "command.execute")
  if (!execution) throw new Error("missing command execution")

	return execution
}

function executionsForCommand(connection: MockWebSocket, commandID: string): { id: string; payload: { arguments: Record<string, unknown> } }[] {
  return connection.sent
    .map((message) => JSON.parse(message) as { id: string; type: string; payload: { commandId?: string; arguments: Record<string, unknown> } })
    .filter((message) => message.type === "command.execute" && message.payload.commandId === commandID)
}

function executionForCommand(connection: MockWebSocket, commandID: string): { id: string; payload: { arguments: Record<string, unknown> } } {
  const execution = executionsForCommand(connection, commandID).at(-1)
  if (!execution) throw new Error(`missing command execution: ${commandID}`)
  return execution
}

function characterAlterationCompletion(requestId: string): object {
  return {
    version: 1,
    id: requestId,
    type: "command.completed",
    payload: {
      sessionId: "session-1",
      requestId,
      commandId: "party.character.update",
      sceneGeneration: 7,
      effectiveExecutionMode: "live_safe",
      success: true,
      summary: "Avatar updated.",
      changes: [{ path: "party.character.Avatar.strength", before: 20, after: 24 }],
      data: {
        character: {
          partyPosition: 1,
          name: "Avatar",
          class: "Avatar",
          status: "Poisoned",
          strength: 24,
          dexterity: 20,
          intelligence: 20,
          currentMp: 20,
          currentHp: 100,
          maximumHp: 100,
          experience: 500,
          level: 5,
        },
        order: [{ partyPosition: 1, name: "Avatar" }],
      },
    },
  }
}

function partyCharacterInspectCompletion(requestId: string, fingerprint: string): object {
	return successfulQueryCompletion(requestId, "party.character.inspect", "Full roster loaded.", {
		fingerprint,
		karma: 50,
		characters: [],
		roster: [
			{ rosterIndex: 0, name: "Avatar", class: "Avatar", status: "good", active: true, strength: 20, dexterity: 20, intelligence: 20, currentMp: 20, currentHp: 100, maximumHp: 100, experience: 500, level: 5 },
			{ rosterIndex: 1, name: "Shamino", class: "Fighter", status: "good", active: true, strength: 18, dexterity: 21, intelligence: 17, currentMp: 17, currentHp: 82, maximumHp: 90, experience: 400, level: 4 },
			{ rosterIndex: 8, name: "Iolo", class: "Bard", status: "good", active: false, strength: 17, dexterity: 22, intelligence: 18, currentMp: 9, currentHp: 78, maximumHp: 85, experience: 350, level: 4 },
		],
	})
}

function windCompletion(requestId: string, direction: string): object {
  return {
    version: 1,
    id: requestId,
    type: "command.completed",
    payload: {
      sessionId: "session-1",
      requestId,
      commandId: "environment.wind.status",
      sceneGeneration: 7,
      effectiveExecutionMode: "query",
      success: true,
      summary: `Current wind: ${direction}`,
      data: { direction },
    },
  }
}

function searchStatusCompletion(requestId: string, includeNorth = false): object {
  return successfulQueryCompletion(requestId, "search.status", "Search status loaded for 1 nearby cells.", {
    locationId: 5,
    locationName: "Britain",
    mapType: "small_map",
    floor: 0,
    cells: [{
      direction: "current", x: 42, y: 84, locationId: 5, locationName: "Britain", mapType: "small_map", floor: 0,
      inBounds: true, tileIndex: 12, tileName: "Grass", authenticLocations: [{
		index: 3, found: true, repeatable: false, rawObjectType: 2, rawPayload: 25,
		classification: "inventory_item", category: "Provision", itemType: 9, itemId: 5,
		itemName: "Gold", quantity: 25, discoveryText: "a sack of gold!",
      }], itemStack: [{ itemType: 1, itemId: 0, itemName: "Gold", quantity: 10 }],
    }, ...(includeNorth ? [{ direction: "north", x: 42, y: 83, locationId: 5, locationName: "Britain", mapType: "small_map", floor: 0, inBounds: true, tileIndex: 12, tileName: "Grass", authenticLocations: [], itemStack: [] }] : [])],
    progress: { totalLocations: 20, foundLocations: 5, oneTimeLocations: 16, foundOneTimeLocations: 4 },
  })
}

function questItemStatusCompletion(requestId: string): object {
  return successfulQueryCompletion(requestId, "quest.item.status", "Quest item status loaded: 2 items.", {
    items: [{
      key: "carpet", name: "Magic Carpet", category: "artifact", quantity: 0,
      canonicalPlacement: { locationId: 0, locationName: "Britannia", x: 55, y: 23, floor: 0 },
    }, { key: "crown", name: "Crown", category: "artifact", quantity: 1 }, {
		key: "shard_falsehood", name: "Shard of Falsehood", category: "shard", quantity: 1,
		shardUseReview: {
			shard: "shard_falsehood", quantity: 1, locationId: 31, locationName: "Empath Abbey", x: 15, y: 3, floor: 1,
			flame: "love", flameMatches: false, shadowlordSummoned: true, shadowlord: "astaroth", shadowlordAlive: true,
			expectedOutcome: "no_effect",
		},
	}, {
		key: "shard_hatred", name: "Shard of Hatred", category: "shard", quantity: 1,
		shardUseReview: {
			shard: "shard_hatred", quantity: 1, locationId: 31, locationName: "Empath Abbey", x: 15, y: 3, floor: 1,
			flame: "love", flameMatches: true, shadowlordSummoned: true, shadowlord: "astaroth", shadowlordAlive: true,
			expectedOutcome: "success",
		},
	}, {
		key: "shard_cowardice", name: "Shard of Cowardice", category: "shard", quantity: 1,
		shardUseReview: {
			shard: "shard_cowardice", quantity: 1, locationId: 31, locationName: "Empath Abbey", x: 15, y: 3, floor: 1,
			flame: "love", flameMatches: false, shadowlordSummoned: true, shadowlord: "astaroth", shadowlordAlive: true,
			expectedOutcome: "no_effect",
		},
	}],
  })
}

function worldPositionCompletion(requestId: string, overrides: Record<string, unknown> = {}): object {
  return successfulQueryCompletion(requestId, "world.position.status", "World position loaded.", {
    sourceFingerprint: "world-source-1",
    locationId: 0, locationName: "Britannia / Underworld", mapType: "large", x: 100, y: 101, floor: 0,
    bounds: { minimumX: 0, maximumX: 255, minimumY: 0, maximumY: 255 },
    destinations: [{
		locationId: 0, locationName: "Britannia / Underworld", mapType: "large", floors: [-1, 0],
		bounds: { minimumX: 0, maximumX: 255, minimumY: 0, maximumY: 255 }, defaultSetup: { floor: 0, x: 80, y: 100 },
	}, {
		locationId: 2, locationName: "Britain", mapType: "small", floors: [-1, 0, 1],
		bounds: { minimumX: 0, maximumX: 31, minimumY: 0, maximumY: 31 }, defaultSetup: { floor: 0, x: 15, y: 30 }, worldEntrance: { floor: 0, x: 85, y: 107 },
	}, {
		locationId: 33, locationName: "Deceit", mapType: "dungeon", floors: [0, 1, 2, 3, 4, 5, 6, 7],
		bounds: { minimumX: 0, maximumX: 7, minimumY: 0, maximumY: 7 }, defaultSetup: { floor: 0, x: 1, y: 1 },
		placementTargets: [
			{ placement: "dungeon_outside_entrance", placementLabel: "Outside dungeon entrance", route: "britannia", routeLabel: "Britannia", available: true, locationId: 0, setup: { floor: 0, x: 240, y: 73 } },
			{ placement: "dungeon_inside_start", placementLabel: "Inside dungeon start", route: "britannia", routeLabel: "Britannia", available: true, locationId: 33, setup: { floor: 0, x: 1, y: 1 }, facingLabel: "East", lastDirectionLabel: "Arrived from above" },
			{ placement: "dungeon_outside_entrance", placementLabel: "Outside dungeon entrance", route: "underworld", routeLabel: "Underworld", available: true, locationId: 0, setup: { floor: -1, x: 240, y: 73 } },
			{ placement: "dungeon_inside_start", placementLabel: "Inside dungeon start", route: "underworld", routeLabel: "Underworld", available: true, locationId: 33, setup: { floor: 7, x: 7, y: 7 }, facingLabel: "West", lastDirectionLabel: "Arrived from below" },
			{ placement: "dungeon_underworld_exit", placementLabel: "Underworld exit ladder", route: "underworld", routeLabel: "Underworld", available: true, locationId: 33, setup: { floor: 7, x: 7, y: 7 }, facingLabel: "West", lastDirectionLabel: "West" },
		],
	}],
	...overrides,
  })
}

function moongateStatusCompletion(requestId: string): object {
  return successfulQueryCompletion(requestId, "moongate.status", "Moongate status loaded.", {
    sourceFingerprint: "source-1", phase: "New Moon", phaseIndex: 0, visible: true, playerOnActiveGate: true, teleportationActive: false,
    moonstones: [{ index: 0, phase: "New Moon", x: 10, y: 20, map: 0, level: 0, state: "buried", found: true }],
    route: { fingerprint: "route-1", kind: "moonstone", destination: { x: 44, y: 55, map: 0, level: 0, locationId: 8, locationName: "Moonglow" } },
  })
}

function spiritualityMoongateStatusCompletion(requestId: string): object {
  return successfulQueryCompletion(requestId, "moongate.status", "Moongate status loaded.", {
    sourceFingerprint: "source-1", phase: "New Moon", phaseIndex: 0, visible: true, playerOnActiveGate: true, teleportationActive: false,
    moonstones: [{ index: 0, phase: "New Moon", x: 10, y: 20, map: 0, level: 0, state: "buried", found: true }],
    route: { fingerprint: "spirituality:6", kind: "spirituality", shrineIndex: 6, shrineName: "Spirituality" },
  })
}

function combatEncounterPreview(entities: object[] = [], triggers: object[] = []): object {
  const slots = (x: number) => Array.from({ length: 6 }, (_, y) => ({ x, y: y + 2 }))
  return {
    tiles: Array.from({ length: 121 }, (_, index) => (index === 53 ? 255 : 68)),
    party: { west: slots(1), east: slots(9), north: slots(5), south: slots(5) },
    monsterSlots: [{ x: 7, y: 3 }, { x: 7, y: 5 }, { x: 7, y: 7 }, { x: 8, y: 4 }],
    entities,
    triggers,
  }
}

function combatEncounterOptionsCompletion(requestId: string): object {
  return successfulQueryCompletion(requestId, "combat.encounter.options", "Encounter options loaded.", {
    britanniaMaps: [{ index: 0, name: "Camping", preview: combatEncounterPreview() }],
    dungeons: [{ id: 0, name: "Deceit", rooms: [
      { room: 0, floor: 5, cell: { x: 3, y: 4 }, preview: combatEncounterPreview(
        [{ kind: "monster", name: "Ghost", sprite: 400, x: 9, y: 4, concealed: true }], [{ x: 9, y: 4 }]) },
      { room: 1, preview: combatEncounterPreview() },
    ] }],
    entryDirections: [{ id: 0, name: "west" }, { id: 1, name: "east" }, { id: 2, name: "north" }, { id: 3, name: "south" }],
    countModes: [{ id: 0, name: "automatic" }, { id: 1, name: "exact" }, { id: 2, name: "maximum" }],
    monsters: [{ id: 32, name: "Orc", strength: 12, dexterity: 10, intelligence: 6, armour: 2, damage: 8, hitPoints: 24, maxGroup: 8, friend: { id: 20, name: "Rat" } }],
  })
}

function merchantTesterCatalogCompletion(requestId: string): object {
  return successfulQueryCompletion(requestId, "merchant.tester.catalog", "Merchant Tester catalog loaded: 3 canonical merchants.", {
    sourceFingerprint: "merchant-source-fingerprint-1234567890",
    currentContext: { locationId: 3, locationName: "Britain", floor: 2, x: 17, y: 29 },
    merchants: [{
      merchantId: "arms.britain", townId: 3, townName: "Britain", shopName: "North Armoury", ownerName: "Britanny",
      familyId: "arms", familyLabel: "Arms Dealer", dialogCode: 0, implemented: true,
      previewAvailable: true, liveAvailable: true, liveExecutionMode: "workflow",
    }, {
      merchantId: "guild.britain", townId: 3, townName: "Britain", shopName: "The Guild", ownerName: "Guido",
      familyId: "guild", familyLabel: "Guild", dialogCode: 5, implemented: true,
      previewAvailable: true, liveAvailable: true, liveExecutionMode: "workflow",
    }, {
      merchantId: "guild.paws", townId: 4, townName: "Paws", shopName: "Paws Guild", ownerName: "Sindar",
      familyId: "guild", familyLabel: "Guild", dialogCode: 5, implemented: true,
      previewAvailable: true, liveAvailable: true, liveExecutionMode: "coordinated_alteration",
    }],
  })
}

function monsterEcosystemCompletion(requestId: string): object {
  return successfulQueryCompletion(requestId, "monster.ecosystem.status", "Monster ecosystem loaded.", {
    locationId: 0,
    locationName: "Britannia",
    mapType: "large_map",
    floor: 0,
    generationEnabled: false,
    generationBaseOdds: 12,
    capacity: 32,
    currentUnitCount: 3,
    enemyCount: 1,
    friendlyCount: 1,
    vehicleCount: 1,
    enemies: [{
      runtimeToken: 17, monsterType: 32, name: "Orc", position: { x: 40, y: 41 }, floor: 0, visible: true,
      activelyAttacks: true, canFlyOverWater: false, canPassThroughWalls: false, doNotMove: false,
      hasWhirlpoolMovement: false, skipNextTurn: false, isSandEnemy: false, isWaterEnemy: false,
    }],
  })
}

function vehicleStatusCompletion(requestId: string, hullHp: number): object {
  return successfulQueryCompletion(requestId, "vehicle.status", "Vehicle status loaded.", {
    vehicleType: 3,
    vehicleName: "frigate",
    currentDirection: "east",
    previousDirection: "north",
    ownedByNpc: false,
    hullHp,
		maximumHullHp: 99,
		sailState: "Anchored",
    skiffQuantity: 1,
    position: { x: 90, y: 91 },
    floor: 0,
    hmsCapeActive: false,
    rawHullDamageAvailable: hullHp > 1,
    fullHullRepairAvailable: true,
  })
}

function spellInventoryCompletion(requestId: string, fingerprint: string, firstQuantity: number, activeSpell = {
  active: false,
  effectType: "",
  friendlyName: "",
  incantations: [] as string[],
  sourceAmbiguous: false,
  description: "",
  turnsRemaining: 0,
}, magicLight = {
  active: false,
  minutesRemaining: 0,
}): object {
  return successfulQueryCompletion(requestId, "inventory.spell.status", "Prepared spell inventory loaded.", {
    snapshotFingerprint: fingerprint,
    spells: Array.from({ length: 48 }, (_, id) => ({
      id,
      name: id === 0 ? "In Lor / Light" : `Spell ${id + 1}`,
      description: `Description ${id + 1}`,
      circle: Math.floor(id / 6) + 1,
      targeting: "party",
      context: "anywhere",
      quantity: id === 0 ? firstQuantity : id % 6,
      maximum: 99,
    })),
    magicLight,
    activeSpell,
  })
}

function activeCombatCompletion(requestId: string, fingerprint: string, asleep: boolean, fixedDungeonRoom = false): object {
  const cells = Array.from({ length: 121 }, (_, index) => ({
    x: index % 11,
    y: Math.floor(index / 11),
    tileId: 1,
    ...(index === 48 ? { occupantKind: "monster", occupantId: "1", occupantName: "Orc" } : {}),
  }))
  return successfulQueryCompletion(requestId, "combat.status", "Active combat status loaded.", {
    active: true,
    snapshotFingerprint: fingerprint,
    monsterTypes: Array.from({ length: 48 }, (_, id) => ({ id, name: id === 32 ? "Orc" : `Monster ${id}` })),
    encounterId: "9",
    map: {
      name: "Glade Combat", index: 2, territory: "britannia", entryDirection: "north", victory: false,
      turnStarted: true, pendingAdvanceTurn: false, soloMoveMode: false, currentPartyIndex: 0,
      activeMonsterIndex: -1, targetPartyIndex: -1, objectCount: 0, cells,
    },
    party: [{
      index: 0, name: "Avatar", x: 5, y: 8, active: true, currentActor: true, currentTarget: false,
      currentHp: 100, maximumHp: 100, currentMp: 20, maximumMp: 20, dexterity: 15, status: "Good",
      queueCountdown: 21, queueStatus: "active", canAct: true,
    }],
    monsters: [{
      id: "1", managerIndex: 0, typeId: 32, name: "Orc", x: 4, y: 4, currentHp: 20, maximumHp: 20,
      currentDexterity: 11, baseDexterity: 12, active: true, dead: false, invisible: false, asleep,
      poisoned: false, currentActor: false, queueCountdown: 25, queueStatus: "active", canAct: !asleep,
		hostile: true,
      abilities: [],
    }],
    forecast: [{ order: 1, kind: "party", id: "0", name: "Avatar", dexterity: 15, resetCountdown: 21, canAct: true, status: "active" }],
		activeMonsterCount: 1, activeHostileMonsterCount: 1, totalMonsterCount: 1,
		fixedDungeonRoom, ...(fixedDungeonRoom ? { dungeonIndex: 0, dungeonRoom: 10 } : {}),
		defeatedExperience: 0, rngSeed: 41, workflowActive: false,
		presentationSpeedPercent: 100, configurationEditing: false,
  })
}

function successfulMutationCompletion(requestId: string, commandId: string, effectiveExecutionMode: string): object {
  return {
    version: 1,
    id: requestId,
    type: "command.completed",
    payload: {
      sessionId: "session-1",
      requestId,
      commandId,
      sceneGeneration: 7,
      effectiveExecutionMode,
      success: true,
      summary: `${commandId} completed.`,
    },
  }
}

function failedCommandCompletion(requestId: string, commandId: string, errorCode: string): object {
  return {
    version: 1,
    id: requestId,
    type: "command.failed",
    payload: {
      sessionId: "session-1",
      requestId,
      commandId,
      sceneGeneration: 7,
      success: false,
      errorCode,
      errorMessage: `${commandId} review is stale.`,
    },
  }
}

function observationCompletion(requestId: string, commandId: string, expectedBranch: string): object {
  return {
    version: 1,
    id: requestId,
    type: "command.completed",
    payload: {
      sessionId: "session-1",
      requestId,
      commandId,
      sceneGeneration: 7,
      effectiveExecutionMode: "workflow",
      success: true,
      summary: `${commandId} workflow queued.`,
      data: {
        workflow: `${commandId}.workflow`,
        queued: true,
        expectedBranch,
        context: {
          date: "Day 12, Month 3, 139",
          time: "22:15",
          hour: 22,
          minute: 15,
          isNight: true,
          mapType: "small_map",
          locationId: 0,
          locationName: "Lord British's Castle",
          floor: 0,
        },
      },
    },
  }
}

function shrineStatusCompletion(requestId: string): object {
  const names = ["Honesty", "Compassion", "Valour", "Justice", "Sacrifice", "Honor", "Spirituality", "Humility"]
  return {
    version: 1,
    id: requestId,
    type: "command.completed",
    payload: {
      sessionId: "session-1",
      requestId,
      commandId: "shrine.status",
      sceneGeneration: 7,
      effectiveExecutionMode: "query",
      success: true,
      summary: "Shrine atlas loaded.",
      data: {
        fingerprint: "shrine-review-1",
        shrines: names.map((virtueName, index) => ({
          index,
          virtueName,
          virtueKey: virtueName.slice(0, 4).toLocaleLowerCase(),
          mantra: index === 7 ? "lum" : "ahm",
          wordOfPower: index === 7 ? "VERAMOCOR" : "FALLAX",
          worldX: 231,
          worldY: 216,
          strBonus: 0,
          dexBonus: 0,
          intBonus: 0,
          karmaBonus: index === 7 ? 6 : 3,
          questState: index === 7 ? "ready_to_complete" : "not_assigned",
          completed: index === 7,
          active: index === 7,
          ruined: index === 7,
        })),
        firstActiveIndex: 7,
        allCompleted: false,
      },
    },
  }
}

function shrineCompletion(requestId: string): object {
	return {
		version: 1,
		id: requestId,
		type: "command.completed",
		payload: {
			sessionId: "session-1",
			requestId,
			commandId: "shrine.reference.inspect",
			sceneGeneration: 7,
			effectiveExecutionMode: "query",
			success: true,
			summary: "Shrine reference loaded: Humility.",
			data: {
				index: 7,
				virtueName: "Humility",
				virtueKey: "humi",
				mantra: "lum",
				wordOfPower: "VERAMOCOR",
				worldX: 231,
				worldY: 216,
				strBonus: 0,
				dexBonus: 0,
				intBonus: 0,
				karmaBonus: 6,
			},
		},
	}
}

function commandCompletion(requestId: string, categoryStates: Record<string, boolean> = { movement: false }): object {
  return {
    version: 1,
    id: requestId,
    type: "command.completed",
    payload: {
      sessionId: "session-1",
      requestId,
      commandId: "logging.status",
      sceneGeneration: 7,
      effectiveExecutionMode: "query",
      success: true,
      summary: "Logging status loaded.",
      data: {
        enabled: true,
        stdoutEnabled: false,
        minimumLevel: "INFO",
        categoryStates,
      },
    },
  }
}
