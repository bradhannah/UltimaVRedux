/** One catalog command call that a task page sends when the person applies staged changes. */
export interface PageStep {
  commandID: string
  argumentsByID: Record<string, string>
}

/** Outcome line of the most recent page action, shown under the page header. */
export interface PageNotice {
  success: boolean
  text: string
}

export function plural(count: number, one: string, many = `${one}s`): string {
  return `${count} ${count === 1 ? one : many}`
}

/** What the Go to page asks for when a person presses "Go there" on a map position. */
export interface WorldPositionApply {
  placement: string
  route?: string
  locationId: number
  floor?: number
  x?: number
  y?: number
  room?: number
  expectedLocationId: number
  expectedFloor: number
  expectedX: number
  expectedY: number
  expectedSourceFingerprint: string
}
