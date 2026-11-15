// YataiDON's face timelines (its Graphics/global/animation.json "texture_change" entries, the ones
// FACE_ANIM_IDS names; base/face_animations.json in the bucket): which face of the sheet shows over
// the first `duration` real-time milliseconds after a motion starts (the game's FaceTimeline).

export type FaceTimeline = {
  duration: number
  loop: boolean
  frames: [start: number, end: number, face: number][]
}

type FaceAnimationEntry = {
  id?: number
  type?: string
  duration?: number
  loop?: boolean
  textures?: [number, number, number][]
}

export function parseFaceTimelines(json: unknown): Map<number, FaceTimeline> {
  const result = new Map<number, FaceTimeline>()
  if (!Array.isArray(json)) return result
  for (const entry of json as FaceAnimationEntry[]) {
    if (entry?.type !== 'texture_change' || entry.id === undefined) continue
    result.set(entry.id, {
      duration: entry.duration ?? 0,
      loop: entry.loop ?? false,
      frames: (entry.textures ?? []).map(([start, end, face]) => [start, end, face]),
    })
  }
  return result
}

// A face timeline playing (YataiDON's TextureChangeAnimation with BaseAnimation's loop).
export class FacePlayer {
  private readonly timeline: FaceTimeline
  private startMs: number
  private finished = false
  face: number

  constructor(timeline: FaceTimeline, now: number) {
    this.timeline = timeline
    this.startMs = now
    this.face = timeline.frames.length > 0 ? timeline.frames[0][2] : 0
  }

  update(now: number): void {
    if (this.timeline.loop && this.finished) {
      this.startMs = now
      this.finished = false
    }
    if (this.finished) return
    const t = now - this.startMs
    if (t <= this.timeline.duration) {
      for (const [start, end, face] of this.timeline.frames)
        if (start < t && t <= end) this.face = face
    } else this.finished = true
  }
}
