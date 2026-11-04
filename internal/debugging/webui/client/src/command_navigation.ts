import type { CommandDefinition } from "./protocol"

export type NavigationWorkspace = "game" | "system" | "saves" | "tools"

export interface NavigationSurface {
  id: string
  workspace: NavigationWorkspace
  workspaceLabel: string
  label: string
  description: string
  commandId: string
  commands: CommandDefinition[]
}

export interface RankedNavigationSurface extends NavigationSurface {
  score: number
}

const recentLimit = 8

function normalizedTokens(value: string): string[] {
  return value.toLocaleLowerCase().trim().split(/\s+/).filter(Boolean)
}

function searchableValues(surface: NavigationSurface): string[] {
  return [
    surface.label,
    surface.workspaceLabel,
    surface.description,
    ...surface.commands.flatMap((command) => [
      command.name,
      command.id,
      command.category,
      command.description,
      ...(command.aliases ?? []),
      ...(command.searchTerms ?? []),
    ]),
  ].map((value) => value.toLocaleLowerCase())
}

export function rankNavigationSurfaces(surfaces: NavigationSurface[], query: string): RankedNavigationSurface[] {
  const normalizedQuery = query.toLocaleLowerCase().trim()
  if (!normalizedQuery) return surfaces.map((surface) => ({ ...surface, score: 0 }))
  const tokens = normalizedTokens(normalizedQuery)

  return surfaces.flatMap((surface) => {
    const values = searchableValues(surface)
    const label = surface.label.toLocaleLowerCase()
    const workspaceQualified = `${surface.workspaceLabel} ${surface.label}`.toLocaleLowerCase()
    let score = 0
    if (label === normalizedQuery || workspaceQualified === normalizedQuery) score = 300
    else if (label.startsWith(normalizedQuery) || workspaceQualified.startsWith(normalizedQuery)) score = 200
    else if (tokens.every((token) => values.some((value) => value.split(/\s+/).some((word) => word.startsWith(token))))) score = 100
    if (score === 0) return []
    return [{ ...surface, score }]
  }).sort((left, right) => right.score - left.score
    || left.workspaceLabel.localeCompare(right.workspaceLabel)
    || left.label.localeCompare(right.label)
    || left.id.localeCompare(right.id))
}

export function updateNavigationRecents(recents: string[], surfaceID: string): string[] {
  return [surfaceID, ...recents.filter((id) => id !== surfaceID)].slice(0, recentLimit)
}

export function recentNavigationSurfaces(surfaces: NavigationSurface[], recents: string[]): NavigationSurface[] {
  const byID = new Map(surfaces.map((surface) => [surface.id, surface]))
  return recents.slice(0, recentLimit).flatMap((id) => {
    const surface = byID.get(id)
    return surface ? [surface] : []
  })
}
