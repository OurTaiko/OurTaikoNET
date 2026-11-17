// .otanim files: baked 2D cut-out animation, an animated puchichara. One texture, parts (triangles
// with fixed texture coordinates) and clips whose frames give every drawn part's vertices. The
// format is SPEC.md in the otanim folder beside this repository (the game's Otanim.cs reads the
// same; that folder's spine2otanim writes them).

export type OtanimPart = {
  name: string
  // 0 normal, 1 additive, 2 multiply, 3 screen (2 and 3 are drawn as normal)
  blend: number
  uvs: Float32Array // u, v per vertex: 0..1, v down from the image's top
  indices: Uint16Array
}

export type OtanimDraw = {
  part: number
  color: [number, number, number, number] // straight RGBA, 0..255
  offset: number // into the clip's positions, 2 floats per vertex
}

export type OtanimClip = {
  name: string
  fps: number
  loop: boolean
  // the frame a loop goes back to: the frames before it are an intro played once
  loopStart: number
  frames: OtanimDraw[][]
  positions: Float32Array // x, y in units, y up
}

export type Otanim = {
  name: string
  premultiplied: boolean
  view: [number, number, number, number] // min x, min y, max x, max y (units, y up)
  unit: number
  texture: Uint8Array // PNG
  parts: OtanimPart[]
  clips: OtanimClip[]
}

export const otanimVersion = 1

export function isOtanim(bytes: Uint8Array): boolean {
  return (
    bytes.length >= 4 &&
    bytes[0] === 0x4f &&
    bytes[1] === 0x54 &&
    bytes[2] === 0x41 &&
    bytes[3] === 0x4e
  )
}

class Reader {
  private p = 0
  private readonly view: DataView
  constructor(private readonly bytes: Uint8Array) {
    this.view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength)
  }
  get atEnd() {
    return this.p >= this.bytes.length
  }
  private need(n: number) {
    if (this.p + n > this.bytes.length) throw new Error('the otanim file ends early')
  }
  u8() {
    this.need(1)
    return this.bytes[this.p++]
  }
  u16() {
    this.need(2)
    const v = this.view.getUint16(this.p, true)
    this.p += 2
    return v
  }
  i16() {
    this.need(2)
    const v = this.view.getInt16(this.p, true)
    this.p += 2
    return v
  }
  u32() {
    this.need(4)
    const v = this.view.getUint32(this.p, true)
    this.p += 4
    return v
  }
  f32() {
    this.need(4)
    const v = this.view.getFloat32(this.p, true)
    this.p += 4
    return v
  }
  bytesOf(n: number) {
    this.need(n)
    const v = this.bytes.subarray(this.p, this.p + n)
    this.p += n
    return v
  }
  str() {
    return new TextDecoder().decode(this.bytesOf(this.u16()))
  }
}

async function inflate(data: Uint8Array): Promise<Uint8Array> {
  const stream = new Blob([data as BlobPart])
    .stream()
    .pipeThrough(new DecompressionStream('deflate-raw'))
  return new Uint8Array(await new Response(stream).arrayBuffer())
}

export async function parseOtanim(bytes: Uint8Array): Promise<Otanim> {
  if (!isOtanim(bytes) || bytes.length < 8) throw new Error('not an otanim file')
  const head = new Reader(bytes)
  head.bytesOf(4)
  const version = head.u16()
  if (version !== otanimVersion) throw new Error(`otanim version ${version} is not supported`)
  const file: Otanim = {
    name: '',
    premultiplied: (head.u16() & 1) !== 0,
    view: [0, 0, 0, 0],
    unit: 0,
    texture: new Uint8Array(),
    parts: [],
    clips: [],
  }
  let partCount = -1
  let clipCount = 0
  let haveTexture = false
  let haveParts = false
  while (!head.atEnd) {
    const tag = new TextDecoder().decode(head.bytesOf(4))
    const payload = head.bytesOf(head.u32())
    if (tag === 'INFO') {
      const r = new Reader(payload)
      file.view = [r.f32(), r.f32(), r.f32(), r.f32()]
      file.unit = r.f32()
      partCount = r.u16()
      clipCount = r.u16()
      file.name = r.str()
    } else if (tag === 'TEXR') {
      file.texture = payload.slice()
      haveTexture = true
    } else if (tag === 'PART') {
      if (partCount < 0) throw new Error('PART before INFO')
      const r = new Reader(await inflate(payload))
      for (let i = 0; i < partCount; i++) {
        const name = r.str()
        const blend = r.u8()
        const vc = r.u16()
        const ic = r.u16()
        const uvs = new Float32Array(vc * 2)
        for (let j = 0; j < uvs.length; j++) uvs[j] = r.u16() / 65535
        const indices = new Uint16Array(ic)
        for (let j = 0; j < ic; j++) {
          indices[j] = r.u16()
          if (indices[j] >= vc) throw new Error(`part ${name} indexes past its vertices`)
        }
        file.parts.push({ name, blend, uvs, indices })
      }
      haveParts = true
    } else if (tag === 'CLIP') {
      if (!haveParts) throw new Error('CLIP before PART')
      file.clips.push(readClip(new Reader(await inflate(payload)), file))
    }
    // unknown chunks are skipped: later versions may add some
  }
  if (!haveTexture || !haveParts || file.clips.length !== clipCount || clipCount === 0)
    throw new Error('the otanim file is missing INFO, TEXR, PART or a CLIP')
  if (file.view[2] <= file.view[0] || file.view[3] <= file.view[1])
    throw new Error('the otanim view is empty')
  return file
}

function readClip(r: Reader, file: Otanim): OtanimClip {
  const name = r.str()
  const fps = r.f32()
  const loop = (r.u8() & 1) !== 0
  const frameCount = r.u32()
  const loopStart = r.u32()
  if (frameCount === 0 || frameCount > 1_000_000 || !(fps > 0))
    throw new Error(`clip ${name} has no frames or no rate`)
  if (loopStart >= frameCount || (!loop && loopStart !== 0))
    throw new Error(`clip ${name} loops back to a frame it does not have`)
  const frames: OtanimDraw[][] = []
  const positions: number[] = []
  let previous = new Map<number, Int16Array>()
  for (let f = 0; f < frameCount; f++) {
    const draws: OtanimDraw[] = []
    const now = new Map<number, Int16Array>()
    for (let d = 0, n = r.u16(); d < n; d++) {
      const part = r.u16()
      if (part >= file.parts.length) throw new Error(`clip ${name} draws a part that is not there`)
      const delta = (r.u8() & 1) !== 0
      const color: [number, number, number, number] = [r.u8(), r.u8(), r.u8(), r.u8()]
      const count = file.parts[part].uvs.length
      const before = previous.get(part)
      if (delta && !before)
        throw new Error(
          `clip ${name} frame ${f} is a change of a part the frame before does not draw`,
        )
      const q = new Int16Array(count)
      draws.push({ part, color, offset: positions.length })
      for (let i = 0; i < count; i++) {
        q[i] = delta ? r.i16() + before![i] : r.i16() // Int16Array wraps as the format says
        positions.push(q[i] * file.unit)
      }
      now.set(part, q)
    }
    previous = now
    frames.push(draws)
  }
  return { name, fps, loop, loopStart, frames, positions: Float32Array.from(positions) }
}

// The frame at `seconds` into the clip and how far toward the next (0..1); next is -1 when there
// is none to ease toward (a one-shot's end). A loop plays its intro once, then loops the rest.
export function otanimFrameAt(
  clip: OtanimClip,
  seconds: number,
): { frame: number; next: number; toward: number } {
  const n = clip.frames.length
  let at = Math.max(0, seconds) * clip.fps
  let frame: number
  let next: number
  if (clip.loop) {
    const start = clip.loopStart
    if (at >= n) at = start + ((at - start) % (n - start))
    frame = Math.min(n - 1, Math.floor(at))
    next = frame + 1 < n ? frame + 1 : start
  } else {
    frame = Math.min(n - 1, Math.floor(at))
    next = frame + 1 < n ? frame + 1 : -1
  }
  return { frame, next, toward: next < 0 ? 0 : at - Math.floor(at) }
}

// Which clips a Don's motion asks for, best first (the game's PuchiAnimGraphic.ClipsFor); a
// puchichara without any of them plays "idle", or its first clip.
const clipsByMotion: Record<string, string[]> = {
  Sabi: ['gogo'],
  SabiStart: ['gogo'],
  Combo: ['combo'],
  FullGage: ['fullgauge', 'combo'],
  FullCombo: ['happy'],
  BalloonSuccess: ['happy'],
  Miss: ['miss'],
  Miss6: ['miss'],
  MissNormal: ['miss'],
  BalloonFailure: ['miss'],
  NormUp: ['clear'],
  NormLoop: ['clear'],
  BalloonLoop: ['balloon'],
  BalloonNoBeat: ['balloon'],
}

export function otanimClipFor(file: Otanim, motion: string): OtanimClip {
  for (const name of clipsByMotion[motion] ?? []) {
    const clip = file.clips.find((c) => c.name === name)
    if (clip) return clip
  }
  return file.clips.find((c) => c.name === 'idle') ?? file.clips[0]
}

// The clip at `seconds` as triangles fitted into `box` (x0, y0, x1, y1, y up), the view's aspect
// kept and centred: positions (x, y), uvs (u, v down; additive parts get 2 added to u), colours
// (premultiplied 0..1) and indices. Frames are eased into each other where the same part is in
// both.
export function otanimMesh(
  file: Otanim,
  clip: OtanimClip,
  seconds: number,
  box: [number, number, number, number],
  mirror: boolean,
): { positions: Float32Array; uvs: Float32Array; colors: Float32Array; indices: Uint32Array } {
  const { frame, next, toward } = otanimFrameAt(clip, seconds)
  const draws = clip.frames[frame]
  const nextDraws = next >= 0 && toward > 0 ? clip.frames[next] : null
  const [vx0, vy0, vx1, vy1] = file.view
  const bw = box[2] - box[0]
  const bh = box[3] - box[1]
  const scale = Math.min(bw / (vx1 - vx0), bh / (vy1 - vy0))
  const bcx = (box[0] + box[2]) / 2
  const bcy = (box[1] + box[3]) / 2
  const ox = bcx - ((vx0 + vx1) / 2) * scale
  const oy = bcy - ((vy0 + vy1) / 2) * scale
  let vertices = 0
  let indexCount = 0
  for (const d of draws) {
    vertices += file.parts[d.part].uvs.length / 2
    indexCount += file.parts[d.part].indices.length
  }
  const positions = new Float32Array(vertices * 2)
  const uvs = new Float32Array(vertices * 2)
  const colors = new Float32Array(vertices * 4)
  const indices = new Uint32Array(indexCount)
  let v = 0
  let ii = 0
  const p = clip.positions
  for (const d of draws) {
    const part = file.parts[d.part]
    const later = nextDraws?.find((n) => n.part === d.part)
    const t = later ? toward : 0
    const c = later ? d.color.map((x, i) => x + (later.color[i] - x) * t) : d.color
    const a = c[3] / 255
    const shift = part.blend === 1 ? 2 : 0
    const start = v
    for (let i = 0; i < part.uvs.length / 2; i++, v++) {
      let x = p[d.offset + 2 * i]
      let y = p[d.offset + 2 * i + 1]
      if (later) {
        x += (p[later.offset + 2 * i] - x) * t
        y += (p[later.offset + 2 * i + 1] - y) * t
      }
      x = ox + x * scale
      if (mirror) x = 2 * bcx - x
      positions[2 * v] = x
      positions[2 * v + 1] = oy + y * scale
      uvs[2 * v] = part.uvs[2 * i] + shift
      uvs[2 * v + 1] = part.uvs[2 * i + 1]
      colors[4 * v] = (c[0] / 255) * a
      colors[4 * v + 1] = (c[1] / 255) * a
      colors[4 * v + 2] = (c[2] / 255) * a
      colors[4 * v + 3] = a
    }
    for (const i of part.indices) indices[ii++] = start + i
  }
  return { positions, uvs, colors, indices }
}

// How many times the clip has come round by `seconds`: a loop's first pass (its intro and its
// loop) is 0, each loop after it one more; a one-shot plays whole each time.
export function otanimPass(clip: OtanimClip, seconds: number): number {
  const n = clip.frames.length
  const at = Math.max(0, seconds) * clip.fps
  if (clip.loop) return at < n ? 0 : 1 + Math.floor((at - n) / (n - clip.loopStart))
  return n > 1 ? Math.floor(at / (n - 1)) : Math.floor(at)
}

// Plays a puchichara: the clip asked for, over and over (a one-shot starts again when it ends; a
// loop plays its intro once, then its loop). Asked for another, it changes at once only from the
// resting clip ("idle", or what stands in for it); any other first plays to the end of its pass,
// so a reaction is never cut short. The game's PuchiAnimGraphic does the same.
export class OtanimPlayer {
  clip: OtanimClip | null = null
  private since = 0
  private pass = 0

  constructor(readonly file: Otanim) {}

  // The clip and the time into it to draw at `ms`, `wanted` being what the Don's state asks for.
  update(ms: number, wanted: OtanimClip): { clip: OtanimClip; seconds: number } {
    const rest = otanimClipFor(this.file, 'Normal')
    if (!this.clip) this.start(wanted, ms)
    else if (wanted !== this.clip) {
      const pass = otanimPass(this.clip, (ms - this.since) / 1000)
      if (this.clip === rest || pass > this.pass) this.start(wanted, ms)
    }
    const clip = this.clip!
    let seconds = (ms - this.since) / 1000
    this.pass = otanimPass(clip, seconds)
    if (!clip.loop) {
      const length = (clip.frames.length - 1) / clip.fps
      seconds = length > 0 ? seconds % length : 0
    }
    return { clip, seconds }
  }

  private start(clip: OtanimClip, ms: number) {
    this.clip = clip
    this.since = ms
    this.pass = 0
  }
}
