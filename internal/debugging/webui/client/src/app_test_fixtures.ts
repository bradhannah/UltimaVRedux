// Synthetic fixtures shared by App component tests and the source-browser harness.
export function sessionReady(sessionId: string, resumed = false): object {
  return { version: 1, id: "hello", type: "session.ready", payload: { sessionId, sceneGeneration: 7, resumed } }
}

export function successfulQueryCompletion(requestId: string, commandId: string, summary: string, data: object): object {
  return {
    version: 1, id: requestId, type: "command.completed",
    payload: { sessionId: "session-1", requestId, commandId, sceneGeneration: 7, effectiveExecutionMode: "query", success: true, summary, data },
  }
}

export function calendarStatusCompletion(requestId: string): object {
  return successfulQueryCompletion(requestId, "calendar.status", "Calendar status loaded.", {
    year: 138, month: 7, day: 12, hour: 16, minute: 15, turn: 4242,
    date: "Wednesday, July 12, 138", time: "4:15 PM", moonPhase: "Crescent Waxing", moonPhaseIndex: 1,
  })
}

export function testSetupStatusCompletion(requestId: string): object {
  return successfulQueryCompletion(requestId, "test-setup.status", "Test setup status loaded.", {
    partyFingerprint: "party-1",
    wipeReview: { fingerprint: "wipe-1", members: [{ slot: 0, name: "Avatar", hp: 100, status: "good" }] },
    locationId: 2, locationName: "Britain", mapType: "small", floor: 0,
    date: "1-1-1", time: "12:00", food: 100, combatActive: false, dialogActive: false,
    gameplayWorkflowActive: false, scriptedSceneActive: false, gameplayTerminal: false, finaleActive: false, allPartyDead: false,
    party: [{ rosterIndex: 0, partyPosition: 1, name: "Avatar", class: "Avatar", status: "Good", currentHp: 100, maximumHp: 100, currentMp: 20, experience: 500, level: 5 }],
  })
}

export function inventoryLockerCompletion(requestId: string, fingerprint: string, gold: number, garlic = 4, dagger = 1): object {
  return successfulQueryCompletion(requestId, "inventory.locker.status", "Inventory locker status loaded.", {
    snapshotFingerprint: fingerprint,
    provisions: [{ name: "gold", quantity: gold, maximum: 9999 }],
    reagents: [{ id: 2, name: "Garlic", quantity: garlic, maximum: 99 }],
    potions: [{ id: 0, name: "Blue", quantity: 1, maximum: 99 }], scrolls: [],
    equipment: [
      { id: 0, name: "Leather Helm", category: "helm", slot: "Helm", ammo: false, quantity: 0, maximum: 99 },
      { id: 4, name: "Small Shield", category: "shield", slot: "Shield", ammo: false, quantity: 1, maximum: 99 },
      { id: 16, name: "Dagger", category: "weapon", slot: "right_hand", ammo: false, quantity: dagger, maximum: 99 },
      { id: 27, name: "Arrows", category: "ammo", slot: "right_hand", ammo: true, quantity: 20, maximum: 99 },
    ],
    equipped: [{ rosterIndex: 0, name: "Avatar", slots: [{ slot: "Weapon", equipmentId: 30, equipmentName: "Long Sword" }] }],
    usablePresetReview: {
      reviewFingerprint: "usable-review-1",
      changes: [
        { category: "quest_items", path: "quest_items.3", before: 0, after: 1 },
        { category: "potions", path: "potions.0", before: 1, after: 5 },
      ],
    },
  })
}
