import { expect, test, type Page } from "@playwright/test"
import { mockWorkbench } from "./mock_workbench"

const pageErrors = new WeakMap<Page, string[]>()
test.beforeEach(({ page }) => {
  const errors: string[] = []
  pageErrors.set(page, errors)
  page.on("pageerror", (error) => errors.push(error.message))
})
test.afterEach(({ page }) => { expect(pageErrors.get(page)).toEqual([]) })

async function openWorkbench(page: Page) {
  const wire = await mockWorkbench(page)
  await page.goto("/")
  await expect(page.locator(".connection")).toHaveText("READY")
  return wire
}

async function openInventory(page: Page) {
  await page.locator('[data-workspace="items"]').click()
  await page.locator('[data-command-select="inventory.locker.status"]').click()
  await expect(page.getByRole("spinbutton", { name: "gold target quantity", exact: true })).toHaveValue("100")
}

test("desktop startup displays status and usable navigation", async ({ page }) => {
  const wire = await openWorkbench(page)
  await expect(page.getByRole("heading", { name: "What do you want to set up?" })).toBeVisible()
  await expect(page.locator("[data-topbar-where]")).toContainText("Britain")
  await expect(page.locator("[data-topbar-party]")).toContainText("1 · all healthy")
  await page.locator('[data-workspace="time"]').click()
  await page.locator('[data-command-select="calendar.status"]').click()
  await expect(page.locator(".calendar-readout")).toContainText("4:15 PM")
  await openInventory(page)
  await expect(page.locator('[data-workspace="more"]')).toBeInViewport()
  await page.locator('[data-workspace="more"]').click()
  await expect(page.locator('[data-workspace="more"]')).toHaveAttribute("aria-current", "page")
  expect(wire.executions("inventory.provision.set")).toHaveLength(0)
})

test("compact laptop layout keeps navigation and inventory controls reachable", async ({ page }) => {
  await page.setViewportSize({ width: 800, height: 900 })
  await openWorkbench(page)
  await openInventory(page)
  const gold = page.getByRole("spinbutton", { name: "gold target quantity", exact: true })
  await gold.fill("555")
  await gold.scrollIntoViewIfNeeded()
  await expect(gold).toBeInViewport({ ratio: 1 })
  await page.getByRole("button", { name: "STAGE CHANGE", exact: true }).click({ trial: true })
  await expect(page.locator('[data-workspace="more"]')).toBeInViewport()
  const overflow = await page.locator(".instrument-bank, .task-tabs, .history-panel").evaluateAll((elements) =>
    elements.filter((element) => element.scrollWidth > element.clientWidth + 1).map((element) => element.className))
  expect(overflow).toEqual([])
})

test("inventory mutation requires confirmation and reports a rejected command", async ({ page }) => {
  const wire = await openWorkbench(page)
  await openInventory(page)
  await page.getByRole("spinbutton", { name: "gold target quantity", exact: true }).fill("555")
  expect(wire.executions("inventory.provision.set")).toHaveLength(0)
  await page.getByRole("button", { name: "STAGE CHANGE", exact: true }).click()
  await expect(page.locator("[data-locker-review-apply]")).toBeVisible()
  expect(wire.executions("inventory.provision.set")).toHaveLength(0)
  await page.locator("[data-locker-review-cancel]").click()
  await expect(page.locator("[data-locker-review-apply]")).toHaveCount(0)
  expect(wire.executions("inventory.provision.set")).toHaveLength(0)
  await page.getByRole("button", { name: "STAGE CHANGE", exact: true }).click()
  await page.locator("[data-locker-review-apply]").click()
  await expect.poll(() => wire.executions("inventory.provision.set").length).toBe(1)
  const request = wire.executions("inventory.provision.set")[0]
  expect(request.payload.arguments).toEqual({ provision: "gold", targetQuantity: 555, expectedQuantity: 100 })
  wire.send({ version: 1, id: request.id, type: "command.failed", payload: {
    sessionId: "session-1", requestId: request.id, commandId: "inventory.provision.set", sceneGeneration: 7,
    success: false, errorCode: "test_rejection", errorMessage: "Inventory change rejected by test transport.",
  } })
  await expect(page.locator(".operation-banner.failure")).toContainText("Inventory change rejected by test transport.")
  expect(wire.executions("inventory.provision.set")).toHaveLength(1)
})

test("command palette supports keyboard navigation and restores focus on Escape", async ({ page }) => {
  const wire = await openWorkbench(page)
  const launcher = page.locator("[data-command-palette-launcher]")
  await launcher.focus()
  await page.keyboard.press("ControlOrMeta+k")
  const input = page.getByRole("combobox", { name: "Find a workspace surface" })
  await expect(input).toBeFocused()
  await page.keyboard.type("inventory")
  const options = page.getByRole("option")
  await expect(options.first()).toBeVisible()
  await page.keyboard.press("End")
  await expect(options.last()).toHaveAttribute("aria-selected", "true")
  await page.keyboard.press("Home")
  await expect(options.first()).toHaveAttribute("aria-selected", "true")
  await page.keyboard.press("Tab")
  await expect(page.getByRole("button", { name: "Close command palette" })).toBeFocused()
  await page.keyboard.press("Escape")
  await expect(page.locator("[data-command-palette]")).toHaveCount(0)
  await expect(launcher).toBeFocused()
  expect(wire.executions("inventory.provision.set")).toHaveLength(0)
})
