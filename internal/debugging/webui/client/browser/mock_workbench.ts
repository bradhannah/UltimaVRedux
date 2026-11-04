import type { Page, WebSocketRoute } from "@playwright/test"
import initialCatalog from "../src/initial_catalog.fixture.json" with { type: "json" }
import { sessionReady, successfulQueryCompletion, calendarStatusCompletion, testSetupStatusCompletion, inventoryLockerCompletion } from "../src/app_test_fixtures"

interface Request { id: string; type: string; payload: { commandId: string; arguments: Record<string, unknown> } }

// A small synthetic transport for browser smoke tests, not a game-service emulator.
// Mutations are captured without completing; the test supplies its explicit result.
export async function mockWorkbench(page: Page) {
  const requests: Request[] = []
  let socket: WebSocketRoute
  const send = (message: object) => socket.send(JSON.stringify(message))
  await page.routeWebSocket("**/ws*", (route) => {
    socket = route
    route.onMessage((raw) => {
      const request = JSON.parse(String(raw)) as Request
      if (request.type === "session.hello") { send(sessionReady("session-1")); return }
      if (request.type === "catalog.request") {
        send({ version: 1, id: request.id, type: "catalog.snapshot", payload: { sceneGeneration: 7, optionsRevision: 9, commands: initialCatalog, optionSets: [] } })
        return
      }
      if (request.type !== "command.execute") return
      requests.push(request)
      const commandId = request.payload.commandId
      const complete = (data: object) => send(successfulQueryCompletion(request.id, commandId, `${commandId} synthetic result.`, data))
      if (commandId === "calendar.status") send(calendarStatusCompletion(request.id))
      else if (commandId === "test-setup.status") send(testSetupStatusCompletion(request.id))
      else if (commandId === "inventory.locker.status") send(inventoryLockerCompletion(request.id, "locker-1", 100))
      else if (commandId === "combat.status") complete({ active: false, monsters: [], party: [], forecast: [], monsterTypes: [] })
    })
  })
  return { send, executions: (commandId: string) => requests.filter((request) => request.payload.commandId === commandId) }
}
