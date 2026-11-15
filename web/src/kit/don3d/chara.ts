import { Matrix4 } from 'three'
import { FacePlayer, type FaceTimeline } from './face'

// The motions of animations.glb, in its order (YataiDON's AnimIndex). The second half are the same
// motions for a Don facing the other way; the animator picks them itself when mirrored.
export const donMotionNames = [
  'BalloonFailure',
  'BalloonLoop',
  'BalloonNoBeat',
  'BalloonSuccess',
  'Bind',
  'Combo',
  'EntryIn',
  'EntryLoop',
  'FukkatuLoop',
  'FukkatuStart',
  'FullCombo',
  'FullGage',
  'GeneralJump',
  'GeneralLoop',
  'Miss',
  'Miss6',
  'MissNormal',
  'NormDown',
  'NormLoop',
  'NormUp',
  'Normal',
  'Sabi',
  'SabiStart',
  'SelectLoop',
  'SelectPanelDown',
  'SelectPanelUp',
] as const

export type DonMotionName = (typeof donMotionNames)[number]

export const DonAnim = Object.fromEntries(donMotionNames.map((name, i) => [name, i])) as {
  readonly [K in DonMotionName]: number
}
export const mirrorOffset = 26
export const miss6Mirror = 40
export const missMirror = 41

// Which face timeline each motion starts (YataiDON FACE_ANIM_IDS), and the revive face.
const faceAnimIds = [
  13, 14, 15, 16, 65, 17, 22, 19, 30, 29, 23, 24, 25, 26, 40, 40, 42, 43, 44, 45, 46, 58, 59, 62,
  60, 18, 13, 14, 15, 16, 64, 17, 22, 21, 36, 35, 23, 24, 25, 26, 40, 40, 42, 43, 44, 45, 46, 58,
  59, 62, 61, 20,
]
const fukkatuFace = 8

// The motion a (possibly mirrored) index is. In the mirrored half miss6 comes before miss (the
// glb's names sort that way), so 40 is DON_MISS6 and 41 DON_MISS; YataiDON's base_anim (index % 26)
// has them the wrong way round.
export function baseAnim(index: number): number {
  if (index === miss6Mirror) return DonAnim.Miss6
  if (index === missMirror) return DonAnim.Miss
  return index % mirrorOffset
}

// What the animator needs of the motions: their names and lengths.
export type MotionInfo = {
  readonly names: readonly string[]
  readonly frameCounts: readonly number[]
}

// YataiDON's Chara3D motion and face logic (the game's Chara3D SetAnim / Update / SetMirror /
// SetFrozen / SetAnimProgress / ApplyFace), without the drawing. Times are in milliseconds.
export class DonAnimator {
  readonly motions: MotionInfo
  readonly faceTimelines: Map<number, FaceTimeline>
  readonly faceCount: number
  bpm = 120
  mirrored = false
  animIndex: number = DonAnim.Normal
  animFrame = 0
  face = -1
  private prevAnimIndex: number = DonAnim.Normal
  private looping = true
  private frozen = false
  private lastFrameMs = 0
  private facePlayer: FacePlayer | null = null

  constructor(
    motions: MotionInfo,
    faceTimelines: Map<number, FaceTimeline>,
    faceCount: number,
    now: number,
  ) {
    this.motions = motions
    this.faceTimelines = faceTimelines
    this.faceCount = faceCount
    this.applyFace(0)
    this.setAnim(DonAnim.Normal, now)
  }

  get count(): number {
    return this.motions.names.length
  }

  animName(index: number): string {
    return index >= 0 && index < this.count ? this.motions.names[index] : ''
  }

  // The frame of the running motion to draw.
  get poseFrame(): number {
    return Math.min(this.animFrame, (this.motions.frameCounts[this.animIndex] ?? 1) - 1)
  }

  applyFace(face: number): void {
    if (face < 0 || face >= this.faceCount) return
    this.face = face
  }

  // YataiDON Chara3D::set_anim. Loops (and the motions that hold) become what a one-shot returns
  // to when it ends; ENTRY_IN and FUKKATU_START queue their loop first.
  setAnim(anim: number, now: number): void {
    const b = baseAnim(anim)
    if (b === DonAnim.EntryIn) this.setAnim(DonAnim.EntryLoop, now)
    else if (b === DonAnim.FukkatuStart) this.setAnim(DonAnim.FukkatuLoop, now)
    let i = b + (this.mirrored ? mirrorOffset : 0)
    if (this.mirrored && (b === DonAnim.Miss || b === DonAnim.Miss6))
      i = miss6Mirror + missMirror - i
    if (i >= 0 && i < this.count) {
      if (
        b === DonAnim.Normal ||
        b === DonAnim.Sabi ||
        b === DonAnim.BalloonNoBeat ||
        b === DonAnim.Miss6 ||
        this.animName(i).includes('loop')
      )
        this.looping = true
      else {
        // not while chaining one-shots, or one would return to itself
        if (this.looping) this.prevAnimIndex = this.animIndex
        this.looping = false
      }
      this.animIndex = i
      this.animFrame = 0
      this.lastFrameMs = 0
    }
    if (b === DonAnim.FukkatuLoop || b === DonAnim.FukkatuStart) {
      // the revive sequence holds one face instead of running a face animation
      this.facePlayer = null
      this.applyFace(fukkatuFace)
    } else {
      const timeline =
        i >= 0 && i < faceAnimIds.length ? this.faceTimelines.get(faceAnimIds[i]) : undefined
      if (timeline) {
        this.facePlayer = new FacePlayer(timeline, now)
        this.applyFace(this.facePlayer.face)
      }
    }
  }

  // Turns the Don round, keeping the running motion's place.
  setMirror(value: boolean, now: number): void {
    if (this.mirrored === value) return
    this.mirrored = value
    const frame = this.animFrame
    const frameMs = this.lastFrameMs
    this.setAnim(this.animIndex, now)
    this.animFrame = frame
    this.lastFrameMs = frameMs
  }

  setFrozen(value: boolean): void {
    if (this.frozen === value) return
    this.frozen = value
    if (!value) this.lastFrameMs = 0
  }

  // Holds the running motion at a point of its loop (0..1, wrapped).
  setAnimProgress(progress: number): void {
    if (this.animIndex < 0 || this.animIndex >= this.count) return
    const loopFrames = this.motions.frameCounts[this.animIndex] - 1
    if (loopFrames <= 0) return
    const cycle = progress - Math.floor(progress)
    this.animFrame = Math.min(loopFrames - 1, Math.trunc(cycle * loopFrames))
  }

  // YataiDON Chara3D::update: a motion's loop lasts one beat (DON_NORMAL three, DON_SABI six, the
  // balloon loop half), stepped a keyframe at a time; a one-shot then goes back.
  update(currentMs: number, now: number = currentMs): void {
    if (!this.frozen && this.animIndex >= 0 && this.animIndex < this.count) {
      const kf = this.motions.frameCounts[this.animIndex]
      if (this.bpm > 0 && kf > 0) {
        let msPerBeat = 60000 / this.bpm
        const b = baseAnim(this.animIndex)
        if (b === DonAnim.Normal) msPerBeat *= 3
        else if (b === DonAnim.Sabi) msPerBeat *= 6
        if (b === DonAnim.BalloonLoop) msPerBeat /= 2
        const msPerFrame = msPerBeat / kf
        if (currentMs - this.lastFrameMs >= msPerFrame) {
          const loopFrames = kf - 1
          this.lastFrameMs = currentMs
          if (loopFrames <= 0) {
            if (!this.looping) {
              this.setAnim(this.prevAnimIndex, now)
              this.looping = true
            }
          } else {
            this.animFrame = (this.animFrame + 1) % loopFrames
            if (!this.looping && this.animFrame === loopFrames - 1) {
              this.setAnim(this.prevAnimIndex, now)
              this.looping = true
            }
          }
        }
      }
    }
    if (this.facePlayer) {
      this.facePlayer.update(now)
      if (this.facePlayer.face !== this.face) this.applyFace(this.facePlayer.face)
    }
  }
}

// ---- look ----

export type Rgb = [number, number, number]

export function parseHexColor(value: string): Rgb {
  const m = /^#?([0-9a-f]{6})$/i.exec(value.trim())
  if (!m) return [255, 255, 255]
  const n = parseInt(m[1], 16)
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255]
}

// YataiDON recolor_texture: on the recolourable textures, reddish texels take the body colour,
// greenish the face colour and bluish the rim; greys and near-blacks stay. RGBA bytes, in place.
export function recolorPixels(
  pixels: Uint8Array | Uint8ClampedArray,
  body: Rgb,
  face: Rgb,
  rim: Rgb,
): void {
  for (let i = 0; i < pixels.length; i += 4) {
    const r = pixels[i]
    const g = pixels[i + 1]
    const b = pixels[i + 2]
    const strongest = Math.max(r, g, b)
    const weakest = Math.min(r, g, b)
    if (strongest <= 0.05 * 255 || strongest - weakest <= 0.08 * 255) continue
    const o = b > r && b >= g ? rim : g > r && g > b ? face : body
    pixels[i] = o[0]
    pixels[i + 1] = o[1]
    pixels[i + 2] = o[2]
  }
}

// ---- placement ----

// YataiDON Chara3DConfig defaults: the Don's size in 720-line pixels a metre and its turn in degrees.
export const scale720 = 650
export const rotX = 181.25
export const rotY = 27.5
export const rotZ = 0

// YataiDON rotation_xyz (angles in radians, negated as it does), in raylib's layout.
export function rotationXyz(ax: number, ay: number, az: number): Matrix4 {
  const cx = Math.cos(-ax)
  const sx = Math.sin(-ax)
  const cy = Math.cos(-ay)
  const sy = Math.sin(-ay)
  const cz = Math.cos(-az)
  const sz = Math.sin(-az)
  // Matrix4.set takes rows.
  return new Matrix4().set(
    cz * cy,
    sz * cy,
    -sy,
    0,
    cz * sy * sx - sz * cx,
    sz * sy * sx + cz * cx,
    cy * sx,
    0,
    cz * sy * cx + sz * sx,
    sz * sy * cx - cz * sx,
    cy * cx,
    0,
    0,
    0,
    0,
    1,
  )
}

// The Don's transform onto a 2D area (y down) where it stands at (x, y), `size` units a metre.
export function modelMatrix(x: number, y: number, size: number, mirrored: boolean): Matrix4 {
  const deg = Math.PI / 180
  const rot = rotationXyz(rotX * deg, (mirrored ? -rotY : rotY) * deg, rotZ * deg)
  return new Matrix4()
    .makeTranslation(x, y, 400)
    .multiply(new Matrix4().makeScale(size, size, size))
    .multiply(rot)
}
