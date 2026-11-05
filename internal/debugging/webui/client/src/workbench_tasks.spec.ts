import { describe, expect, it } from "vitest"

import { compareToolOrder, partyHealthSummary, placeLabel, taskDefinitions, toolDefinitions } from "./workbench_tasks"

describe("workbench tasks", () => {
  it("places every tool on a known task and groups only More tools", () => {
    const tasks = new Set(taskDefinitions.map((task) => task.id))
    for (const [commandID, tool] of Object.entries(toolDefinitions)) {
      expect(tasks.has(tool.task), commandID).toBe(true)
      expect(Boolean(tool.group), commandID).toBe(tool.task === "more")
    }
  })

  it("orders defined tools first, then unknown commands by id", () => {
    const ids = ["zzz.generic", "calendar.status", "aaa.generic", "inventory.locker.status"]
    expect(ids.sort(compareToolOrder)).toEqual(["inventory.locker.status", "calendar.status", "aaa.generic", "zzz.generic"])
  })

  it("summarises party health in plain language", () => {
    expect(partyHealthSummary(undefined)).toBeUndefined()
    expect(partyHealthSummary([])).toBeUndefined()
    expect(partyHealthSummary([{ status: "Good", currentHp: 30, maximumHp: 30 }])).toEqual({ count: 1, text: "all healthy", needsAttention: false })
    expect(partyHealthSummary([
      { status: "Poisoned", currentHp: 10, maximumHp: 30 },
      { status: "Good", currentHp: 12, maximumHp: 30 },
      { status: "Good", currentHp: 30, maximumHp: 30 },
      { status: "Poisoned", currentHp: 30, maximumHp: 30 },
    ])).toEqual({ count: 4, text: "2 poisoned, 1 wounded", needsAttention: true })
  })

  it("labels places by map type", () => {
    expect(placeLabel({ locationName: "Covetous", mapType: "dungeon", floor: 5 })).toBe("Covetous · Level 6")
    expect(placeLabel({ locationName: "Britain", mapType: "small", floor: 0 })).toBe("Britain")
    expect(placeLabel({ locationName: "Britain", mapType: "small", floor: -1 })).toBe("Britain · Basement")
    expect(placeLabel({ locationName: "Britannia", mapType: "large", floor: -1 })).toBe("Britannia · Underworld")
    expect(placeLabel({ locationName: "Castle", mapType: "small", floor: 2 })).toBe("Castle · Floor 2")
  })
})
