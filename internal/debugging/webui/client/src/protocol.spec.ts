import { describe, expect, it } from "vitest"

import type { CommandDefinition, CommandResult } from "./protocol"
import catalog from "./initial_catalog.fixture.json"

describe("CommandResult protocol", () => {
  it("carries numeric step metadata losslessly", () => {
    const command = (JSON.parse(JSON.stringify(catalog)) as CommandDefinition[]).find(command => command.id === "combat.presentation.speed.set")!
    expect(command.parameters?.[0]).toMatchObject({ kind: "integer", minimum: 50, maximum: 2000, step: 50 })
    expect(command.parameters?.[0]?.choices?.map((choice) => choice.value)).toEqual(
      ["50", "100", "150", "200", "300", "400", "600", "800", "1200", "1600", "2000"])
  })
  it("preserves a normalized typed invocation without transport ownership fields", () => {
    const result = JSON.parse(`{
      "sessionId":"session-1",
      "requestId":"request-1",
      "commandId":"test.apply",
      "sceneGeneration":7,
      "invocation":{
        "commandId":"test.apply",
        "arguments":{"count":2,"enabled":true,"mode":"all","members":[1,3]}
      },
      "success":true
    }`) as CommandResult

    expect(result.invocation).toEqual({
      commandId: "test.apply",
      arguments: { count: 2, enabled: true, mode: "all", members: [1, 3] },
    })
    expect(Object.keys(result.invocation ?? {})).toEqual(["commandId", "arguments"])
  })

  it("allows pre-normalization failures to omit the invocation", () => {
    const result: CommandResult = {
      sessionId: "session-1",
      requestId: "request-1",
      commandId: "unknown.command",
      sceneGeneration: 7,
      success: false,
      errorCode: "invalid_request",
    }

    expect(result.invocation).toBeUndefined()
  })
})
