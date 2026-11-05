import protocolContract from "./protocol_contract.json"

export const protocolVersion = protocolContract.protocolVersion

export type ParameterKind = keyof typeof protocolContract.parameterKinds
export type ExecutionMode = keyof typeof protocolContract.executionModes
export type MessageType = keyof typeof protocolContract.messageTypes

export interface Choice {
  value: string
  label: string
  metadata?: Record<string, unknown>
}

export interface OptionSet {
  provider: string
  choices: Choice[]
  unavailableReason?: string
}

export interface ParameterDefinition {
  id: string
  name: string
  description?: string
  kind: ParameterKind
  required: boolean
  default?: unknown
  minimum?: number
  maximum?: number
	step?: number
  pattern?: string
  choices?: Choice[]
  optionsProvider?: string
  controlHint?: string
  availability?: string[]
}

export interface CommandDefinition {
  id: string
  category: string
  name: string
  description: string
  aliases?: string[]
  searchTerms?: string[]
  parameters?: ParameterDefinition[]
  executionModes: ExecutionMode[]
  destructive: boolean
}

export interface CommandResult {
  sessionId: string
  requestId: string
  commandId: string
  sceneGeneration: number
  invocation?: CommandInvocation
  effectiveExecutionMode?: ExecutionMode
  success: boolean
  summary?: string
  errorCode?: string
  errorMessage?: string
  changes?: StateChange[]
  data?: unknown
}

export interface CommandInvocation {
  commandId: string
  arguments: Record<string, unknown>
}

export interface StateChange {
  path: string
  before?: unknown
  after?: unknown
}

export interface Envelope<T = unknown> {
  version: number
  id?: string
  type: MessageType
  payload?: T
}

export interface SessionHello {
  sessionId?: string
}

export interface SessionReady {
  sessionId: string
  sceneGeneration: number
  resumed: boolean
}

export interface CatalogSnapshot {
  sceneGeneration: number
  optionsRevision: number
  commands: CommandDefinition[]
  optionSets?: OptionSet[]
}

export interface CommandExecute {
  commandId: string
  expectedSceneGeneration?: number
  expectedOptionsRevision?: number
  arguments?: Record<string, unknown>
}

export interface CommandAccepted {
  requestId: string
  commandId: string
}

export interface CommandAck {
  requestId: string
}

export interface OperationStatus {
  requestId: string
}

export interface ProtocolError {
  code: string
  message: string
}

export function envelope<T>(id: string, type: MessageType, payload: T): Envelope<T> {
  return { version: protocolVersion, id, type, payload }
}

type Equal<Left, Right> =
  (<Value>() => Value extends Left ? 1 : 2) extends
  (<Value>() => Value extends Right ? 1 : 2) ? true : false
type Assert<Value extends true> = Value
type OptionalKeys<Value> = {
  [Key in keyof Value]-?: object extends Pick<Value, Key> ? Key : never
}[keyof Value]
type RequiredKeys<Value> = Exclude<keyof Value, OptionalKeys<Value>>
type RequiredContractKeys<Name extends keyof typeof protocolContract.fields> =
  keyof typeof protocolContract.fields[Name]["required"]
type OptionalContractKeys<Name extends keyof typeof protocolContract.fields> =
  keyof typeof protocolContract.fields[Name]["optional"]
type WireFieldsMatch<Name extends keyof typeof protocolContract.fields, Value> =
  Equal<RequiredKeys<Value>, RequiredContractKeys<Name>> extends true
    ? Equal<OptionalKeys<Value>, OptionalContractKeys<Name>>
    : false

type ProtocolWireParity = [
  Assert<WireFieldsMatch<"Choice", Choice>>,
  Assert<WireFieldsMatch<"OptionSet", OptionSet>>,
  Assert<WireFieldsMatch<"ParameterDefinition", ParameterDefinition>>,
  Assert<WireFieldsMatch<"CommandDefinition", CommandDefinition>>,
  Assert<WireFieldsMatch<"CommandInvocation", CommandInvocation>>,
  Assert<WireFieldsMatch<"StateChange", StateChange>>,
  Assert<WireFieldsMatch<"CommandResult", CommandResult>>,
  Assert<WireFieldsMatch<"Envelope", Envelope>>,
  Assert<WireFieldsMatch<"SessionHello", SessionHello>>,
  Assert<WireFieldsMatch<"SessionReady", SessionReady>>,
  Assert<WireFieldsMatch<"CatalogSnapshot", CatalogSnapshot>>,
  Assert<WireFieldsMatch<"CommandExecute", CommandExecute>>,
  Assert<WireFieldsMatch<"CommandAccepted", CommandAccepted>>,
  Assert<WireFieldsMatch<"CommandAck", CommandAck>>,
  Assert<WireFieldsMatch<"OperationStatus", OperationStatus>>,
  Assert<WireFieldsMatch<"ProtocolError", ProtocolError>>,
]

export type { ProtocolWireParity }

// Room arrival (world.position.set placement "dungeon_outside_room", parameter "room").
// Mirrors debugging.WorldPositionDungeonRoomData; WorldPositionDestinationData gains
// `rooms?: WorldPositionDungeonRoomData[]` for dungeons, ordered by floor then room number.
export type WorldPlacementDungeonOutsideRoom = "dungeon_outside_room"

export interface WorldPositionDungeonRoomData {
  room: number
  label: string
  floor: number
  roomX: number
  roomY: number
  available: boolean
  unavailableReason?: string
  /** The approach cell: one step forward from here enters the room. */
  setup?: { floor: number, x: number, y: number }
  facingLabel?: string
  /** Fixed monsters from the room preview, for example "Ghost ×3, Insects". */
  summary?: string
}

// New command IDs: party.restore (reviewed with expectedFingerprint like party.fill, returns the
// same before/after roster payload) and combat.monster.sleep-all (encounterId + expectedSnapshot
// like combat.monster.kill-all; its data is the refreshed combat.status payload).
export type PartyRestoreCommandID = "party.restore"
export type CombatMonsterSleepAllCommandID = "combat.monster.sleep-all"
