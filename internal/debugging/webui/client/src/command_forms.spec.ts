import { describe, expect, it } from "vitest"

import { hasUnsupportedParameters, reconcileDraft, serializeDraft, validateDraft } from "./command_forms"
import type { CommandDefinition } from "./protocol"
import catalog from "./initial_catalog.fixture.json"

const formCommand: CommandDefinition = {
  id: "test.static-form",
  category: "Test",
  name: "Static Form",
  description: "Test static controls.",
  executionModes: ["query"],
  destructive: false,
  parameters: [
    { id: "name", name: "Name", kind: "string", required: true },
    { id: "count", name: "Count", kind: "integer", required: true, minimum: 0, maximum: 7, default: 0 },
    { id: "enabled", name: "Enabled", kind: "boolean", required: false, default: false },
    {
      id: "section",
      name: "Section",
      kind: "enum",
      required: true,
      default: "all",
      choices: [{ value: "all", label: "All Maps" }, { value: "dungeons", label: "Dungeons" }],
    },
  ],
}

describe("generated command forms", () => {
  it("validates canonical combat-speed steps from the shared catalog", () => {
    const command = (catalog as CommandDefinition[]).find(command => command.id === "combat.presentation.speed.set")!
    for (const percent of [50, 100, 150, 200, 300, 400, 600, 800, 1200, 1600, 2000]) expect(validateDraft(command, { percent: String(percent) })).toEqual({})
    for (const percent of [25, 75, 125, 250, 350, 500, 2001, 50.5]) expect(validateDraft(command, { percent: String(percent) }).percent).toBeTruthy()
    expect(validateDraft(command, { percent: "250" }).percent).toContain("Choose one of 50, 100")
  })
  it("retains zero and false defaults as editable values", () => {
    expect(reconcileDraft(formCommand)).toEqual({ name: "", count: "0", enabled: "false", section: "all" })
  })

  it("serializes native JSON types and omits optional unset values", () => {
    const argumentsByID = serializeDraft(formCommand, {
      name: "Avatar",
      count: "7",
      enabled: "false",
      section: "dungeons",
    })

    expect(argumentsByID).toEqual({ name: "Avatar", count: 7, enabled: false, section: "dungeons" })
    expect(typeof argumentsByID.count).toBe("number")
    expect(typeof argumentsByID.enabled).toBe("boolean")
  })

  it("reports required, integer-bound, and enum errors before submission", () => {
    expect(validateDraft(formCommand, { name: "", count: "8", enabled: "", section: "unknown" })).toEqual({
      name: "A value is required.",
      count: "Must be at most 7.",
      section: "Choose a listed option.",
    })
  })

  it("resolves dynamic character choices and clears stale selections", () => {
    const characterCommand: CommandDefinition = {
      id: "party.character.update",
      category: "Party",
      name: "Character Alteration",
      description: "Alter a character.",
      executionModes: ["live_safe"],
      destructive: false,
      parameters: [{
        id: "character",
        name: "Character",
        kind: "character",
        required: true,
        optionsProvider: "party.active-characters",
      }],
    }
    const options = { "party.active-characters": [{ value: "0.QXZhdGFy", label: "Position 1 · Avatar" }] }

    expect(hasUnsupportedParameters(characterCommand)).toBe(true)
    expect(hasUnsupportedParameters(characterCommand, options)).toBe(false)
    expect(reconcileDraft(characterCommand, { character: "0.QXZhdGFy" }, options)).toEqual({ character: "0.QXZhdGFy" })
    expect(reconcileDraft(characterCommand, { character: "1.U2hhbWlubw" }, options)).toEqual({ character: "" })
    expect(validateDraft(characterCommand, { character: "0.QXZhdGFy" }, options)).toEqual({})
    expect(serializeDraft(characterCommand, { character: "0.QXZhdGFy" })).toEqual({ character: "0.QXZhdGFy" })
  })
})
