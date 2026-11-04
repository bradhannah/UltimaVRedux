import { describe, expect, it } from "vitest"

import { rankNavigationSurfaces, recentNavigationSurfaces, updateNavigationRecents, type NavigationSurface } from "./command_navigation"
import type { CommandDefinition } from "./protocol"

const surfaces: NavigationSurface[] = [
  surface("more:shrine", "more", "More", "Shrine Progress", "Live virtue progression", [command("shrine.status", "Shrine Status", ["meditation"])]),
  surface("time:logging", "time", "Time & sky", "Shrine Logging", "Shrine log channels", [command("logging.status", "Logging Status", ["stdout"])]),
  surface("more:ships", "more", "More", "Ships", "Vehicle controls", [command("vehicle.status", "Vehicle Status", ["frigate"]), command("ship.repair-full", "Repair Ship", ["hull"])]),
]

describe("command navigation", () => {
  it("ranks exact, prefix, then deterministic all-token matches", () => {
    expect(rankNavigationSurfaces(surfaces, "Ships").map(({ id, score }) => [id, score])).toEqual([["more:ships", 300]])
    expect(rankNavigationSurfaces(surfaces, "Shr").map(({ id, score }) => [id, score])).toEqual([
      ["more:shrine", 200], ["time:logging", 200],
    ])
    expect(rankNavigationSurfaces(surfaces, "repair hull").map(({ id }) => id)).toEqual(["more:ships"])
  })

  it("keeps workspace-qualified surfaces with similar labels distinct", () => {
    expect(rankNavigationSurfaces(surfaces, "More Shrine Progress").map(({ id }) => id)).toEqual(["more:shrine"])
    expect(rankNavigationSurfaces(surfaces, "Time & sky Shrine Logging").map(({ id }) => id)).toEqual(["time:logging"])
  })

  it("aggregates companion intent into its owner surface", () => {
    expect(rankNavigationSurfaces(surfaces, "repair frigate").map(({ id }) => id)).toEqual(["more:ships"])
  })

  it("deduplicates and bounds local recents to eight existing surfaces", () => {
    let recents: string[] = []
    for (let index = 0; index < 10; index++) recents = updateNavigationRecents(recents, `surface-${index}`)
    recents = updateNavigationRecents(recents, "surface-5")
    expect(recents).toHaveLength(8)
    expect(recents[0]).toBe("surface-5")
    expect(recentNavigationSurfaces(surfaces, ["missing", "more:ships"]).map(({ id }) => id)).toEqual(["more:ships"])
  })
})

function command(id: string, name: string, searchTerms: string[]): CommandDefinition {
  return { id, name, category: "Test", description: name, executionModes: ["query"], destructive: false, searchTerms }
}

function surface(id: string, workspace: NavigationSurface["workspace"], workspaceLabel: string, label: string, description: string, commands: NavigationSurface["commands"]): NavigationSurface {
  return { id, workspace, workspaceLabel, label, description, commandId: commands[0]?.id ?? "", commands }
}
