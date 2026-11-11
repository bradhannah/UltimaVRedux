import { createApp, h, nextTick, reactive } from "vue"
import { expect, it, vi } from "vitest"
import PreferenceNumberControl from "./PreferenceNumberControl.vue"
import catalog from "./initial_catalog.fixture.json"
import type { CommandDefinition } from "./protocol"

it("offers only listed speeds, retains drafts on refresh, and emits only allowed values", async () => {
  const commandID = "combat.presentation.speed.set"
  const apply = vi.fn()
  const props = reactive({ commandId: commandID, command: (catalog as CommandDefinition[]).find(command => command.id === commandID), value: 100, disabled: false, onApply: apply })
  const root = document.createElement("div")
  const app = createApp({ setup: () => () => h(PreferenceNumberControl, props) })
  app.mount(root)
  try {
    expect(root.querySelector("input")).toBeNull()
    const select = root.querySelector("select")!
    const button = root.querySelector("button")!
    expect([...select.options].map((option) => option.textContent)).toEqual(
      ["0.5x", "1x", "1.5x", "2x", "3x", "4x", "6x", "8x", "12x", "16x", "20x", "Ludicrous"])
    expect(select.value).toBe("100")
    expect(button.disabled).toBe(true)
    select.value = "1200"
    select.dispatchEvent(new Event("change"))
    await nextTick()
    props.value = 200
    await nextTick()
    expect(select.value).toBe("1200")
    button.click()
    expect(apply).toHaveBeenCalledWith({ percent: "1200" })
    props.disabled = true
    await nextTick()
    button.click()
    expect(apply).toHaveBeenCalledTimes(1)
  } finally { app.unmount() }
})
