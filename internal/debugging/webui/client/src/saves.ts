export type SaveSource = "project" | "personal" | "rollback"

export interface SaveEligibility {
  canCapture: boolean
  canLoad: boolean
  canDelete: boolean
  reason?: string
}

export interface SaveSnapshotSummary {
  reference: string
  source: SaveSource
  id: string
  name: string
  description?: string
  createdAt: string
  gameVersion: string
  revision: string
  compatibility: { compatible: boolean; reason?: string }
  eligibility: SaveEligibility
}

export interface SaveSnapshotListData {
  snapshots: SaveSnapshotSummary[]
  captureEligibility: SaveEligibility
}

export interface SaveSnapshotDetail {
  summary: SaveSnapshotSummary
  snapshotVersion: number
  location: number
  locationName: string
  x: number
  y: number
  floor: number
  year: number
  month: number
  day: number
  hour: number
  minute: number
  partyMembers: number
  moonPhase: string
  moonPhaseIndex: number
}

export interface SaveSnapshotGetData { snapshot: SaveSnapshotDetail }

export function saveSlug(name: string): string {
  return name.toLocaleLowerCase().trim().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "")
}
