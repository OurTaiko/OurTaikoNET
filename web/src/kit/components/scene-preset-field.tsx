import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { scenePresetChoices, scenePresetQuery } from '../scene-presets'
import { ChoiceSelect } from './choice-select'

// The play backdrop of a playlist or a chart: none, or one of the game's presets
// (scene-presets.ts). The value the form opened with stays a choice when the catalog no longer
// has it or cannot be read (or, for a chart, the file named one of its own), so saving keeps it.
export function ScenePresetField({
  value,
  onChange,
  disabled,
  label,
  noneLabel,
  help,
}: {
  value: string
  onChange: (value: string) => void
  disabled: boolean
  label: string
  noneLabel: string
  help: string
}) {
  const { data: presets = [] } = useQuery(scenePresetQuery())
  // The value the form opened with: a choice for as long as it is open.
  const [current] = useState(value)
  return (
    <div className="space-y-2">
      <p className="text-sm font-medium">{label}</p>
      <ChoiceSelect
        label={label}
        value={value}
        onValueChange={onChange}
        disabled={disabled}
        items={scenePresetChoices(presets, current).map((preset) => ({
          value: preset,
          label: preset || noneLabel,
        }))}
      />
      <p className="text-sm text-muted-foreground">{help}</p>
    </div>
  )
}
