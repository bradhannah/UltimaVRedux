import type { Choice, CommandDefinition, ParameterDefinition } from "./protocol"

export type CommandDraft = Record<string, string>
export type FormErrors = Record<string, string>
export type OptionSets = Record<string, Choice[]>

const supportedParameterKinds = new Set(["string", "integer", "boolean", "enum", "character"])

export function hasUnsupportedParameters(command: CommandDefinition, optionSets: OptionSets = {}): boolean {
  return (command.parameters ?? []).some((parameter) =>
    !supportedParameterKinds.has(parameter.kind) || Boolean(parameter.optionsProvider) && optionSets[parameter.optionsProvider ?? ""] === undefined,
  )
}

export function reconcileDraft(command: CommandDefinition, existing: CommandDraft = {}, optionSets: OptionSets = {}): CommandDraft {
  const draft: CommandDraft = {}
  for (const parameter of command.parameters ?? []) {
    if (!supportedParameterKinds.has(parameter.kind)) {
      draft[parameter.id] = existing[parameter.id] ?? ""
      continue
    }
    if (existing[parameter.id] !== undefined) {
      const choices = choicesForParameter(parameter, optionSets)
      draft[parameter.id] = parameter.optionsProvider && !choices.some((choice) => choice.value === existing[parameter.id]) ? "" : existing[parameter.id]
    } else if (parameter.default !== undefined) {
      draft[parameter.id] = String(parameter.default)
    } else {
      draft[parameter.id] = ""
    }
  }

  return draft
}

export function resetDraft(command: CommandDefinition, optionSets: OptionSets = {}): CommandDraft {
  return reconcileDraft(command, {}, optionSets)
}

export function validateDraft(command: CommandDefinition, draft: CommandDraft, optionSets: OptionSets = {}): FormErrors {
  const errors: FormErrors = {}
  for (const parameter of command.parameters ?? []) {
    const value = draft[parameter.id] ?? ""
    const error = validateParameter(parameter, value, optionSets)
    if (error) errors[parameter.id] = error
  }

  return errors
}

export function serializeDraft(command: CommandDefinition, draft: CommandDraft): Record<string, string | number | boolean> {
	const argumentsByID: Record<string, string | number | boolean> = {}
  for (const parameter of command.parameters ?? []) {
    const value = draft[parameter.id] ?? ""
    if (value === "") continue
    switch (parameter.kind) {
      case "integer":
        argumentsByID[parameter.id] = Number(value)
        break
      case "boolean":
        argumentsByID[parameter.id] = value === "true"
        break
      case "string":
      case "enum":
      case "character":
        argumentsByID[parameter.id] = value
        break
      default:
        break
    }
  }

  return argumentsByID
}

export function choicesForParameter(parameter: ParameterDefinition, optionSets: OptionSets = {}): Choice[] {
  if (parameter.optionsProvider) return optionSets[parameter.optionsProvider] ?? []

  return parameter.choices ?? []
}

export function validateParameter(parameter: ParameterDefinition, value: string, optionSets: OptionSets = {}): string | undefined {
  if (!supportedParameterKinds.has(parameter.kind)) return `Unsupported parameter kind: ${parameter.kind}.`
  if (parameter.optionsProvider && optionSets[parameter.optionsProvider] === undefined) return "Dynamic options are not available."
  if (value === "") return parameter.required ? "A value is required." : undefined

  switch (parameter.kind) {
    case "integer":
      return validateInteger(parameter, value)
    case "boolean":
      return value === "true" || value === "false" ? undefined : "Choose On or Off."
    case "enum":
    case "character":
      return validateChoice(parameter, value, optionSets)
    case "string":
      return validateString(parameter, value)
    default:
      return `Unsupported parameter kind: ${parameter.kind}.`
  }
}

function validateInteger(parameter: ParameterDefinition, value: string): string | undefined {
  if (!value.trim()) return "Enter a whole number. Blank is not zero."
  const number = Number(value)
  if (!Number.isSafeInteger(number)) return "Enter a whole number within the browser's exact integer range."
  if (parameter.minimum !== undefined && number < parameter.minimum) return `Must be at least ${parameter.minimum}.`
  if (parameter.maximum !== undefined && number > parameter.maximum) return `Must be at most ${parameter.maximum}.`
	if (parameter.step !== undefined && (number - (parameter.minimum ?? 0)) % parameter.step !== 0) return `Use steps of ${parameter.step} from ${parameter.minimum ?? 0}.`
  if (parameter.choices?.length && !parameter.choices.some((choice) => Number(choice.value) === number)) {
    return `Choose one of ${parameter.choices.map((choice) => choice.value).join(", ")}.`
  }

  return undefined
}

function validateChoice(parameter: ParameterDefinition, value: string, optionSets: OptionSets): string | undefined {
  if (!choicesForParameter(parameter, optionSets).some((choice) => choice.value === value)) return "Choose a listed option."

  return undefined
}

function validateString(parameter: ParameterDefinition, value: string): string | undefined {
  if (!parameter.pattern) return undefined
  try {
    if (!new RegExp(parameter.pattern).test(value)) return "Does not match the required pattern."
  } catch {
    return "Schema pattern is unsupported by this browser."
  }

  return undefined
}
