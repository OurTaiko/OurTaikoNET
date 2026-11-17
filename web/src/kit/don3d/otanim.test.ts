import { describe, expect, it } from 'vitest'
import {
  isOtanim,
  otanimClipFor,
  otanimFrameAt,
  otanimMesh,
  OtanimPlayer,
  parseOtanim,
} from './otanim'

// A small .otanim written from the format document (the otanim folder's SPEC.md), as the game's OtanimFixture
// does: one square part 100 units wide, clips moving it to the given left x per frame.
async function deflate(bytes: Uint8Array): Promise<Uint8Array> {
  const stream = new Blob([bytes as BlobPart])
    .stream()
    .pipeThrough(new CompressionStream('deflate-raw'))
  return new Uint8Array(await new Response(stream).arrayBuffer())
}

class Writer {
  bytes: number[] = []
  u8(v: number) {
    this.bytes.push(v & 255)
  }
  u16(v: number) {
    this.u8(v)
    this.u8(v >> 8)
  }
  u32(v: number) {
    this.u16(v & 0xffff)
    this.u16(v >>> 16)
  }
  f32(v: number) {
    const b = new Uint8Array(new Float32Array([v]).buffer)
    b.forEach((x) => this.u8(x))
  }
  str(s: string) {
    const b = new TextEncoder().encode(s)
    this.u16(b.length)
    b.forEach((x) => this.u8(x))
  }
  raw(b: Uint8Array | number[]) {
    b.forEach((x) => this.u8(x))
  }
  get array() {
    return new Uint8Array(this.bytes)
  }
}

type FixtureClip = { name: string; loop: boolean; fps: number; lefts: number[]; loopStart?: number }

async function fixture(...clips: FixtureClip[]): Promise<Uint8Array> {
  const unit = 1 / 16
  const out = new Writer()
  out.raw(new TextEncoder().encode('OTAN'))
  out.u16(1)
  out.u16(0)
  const chunk = (tag: string, payload: Uint8Array) => {
    out.raw(new TextEncoder().encode(tag))
    out.u32(payload.length)
    out.raw(payload)
  }
  const info = new Writer()
  ;[-10, -10, 210, 110].forEach((v) => info.f32(v))
  info.f32(unit)
  info.u16(1)
  info.u16(clips.length)
  info.str('fixture')
  chunk('INFO', info.array)
  chunk('TEXR', new Uint8Array([0x89, 0x50, 0x4e, 0x47]))
  const part = new Writer()
  part.str('square')
  part.u8(0)
  part.u16(4)
  part.u16(6)
  for (const [u, v] of [
    [0, 1],
    [0, 0],
    [1, 0],
    [1, 1],
  ]) {
    part.u16(u * 65535)
    part.u16(v * 65535)
  }
  ;[0, 1, 2, 2, 3, 0].forEach((i) => part.u16(i))
  chunk('PART', await deflate(part.array))
  for (const c of clips) {
    const w = new Writer()
    w.str(c.name)
    w.f32(c.fps)
    w.u8(c.loop ? 1 : 0)
    w.u32(c.lefts.length)
    w.u32(c.loopStart ?? 0)
    let previous: number[] | null = null
    for (const left of c.lefts) {
      w.u16(1)
      w.u16(0)
      const q = [
        [left, 0],
        [left, 100],
        [left + 100, 100],
        [left + 100, 0],
      ].flatMap(([x, y]) => [Math.round(x / unit), Math.round(y / unit)])
      w.u8(previous ? 1 : 0)
      w.raw([255, 255, 255, 255])
      q.forEach((v, i) => w.u16((previous ? v - previous[i] : v) & 0xffff))
      previous = q
    }
    chunk('CLIP', await deflate(w.array))
  }
  chunk('XTRA', new Uint8Array([1, 2, 3])) // unknown chunks are skipped
  return out.array
}

describe('otanim', () => {
  it('reads a file back as written', async () => {
    const bytes = await fixture(
      { name: 'idle', loop: true, fps: 10, lefts: [0, 50, 100] },
      { name: 'gogo', loop: false, fps: 4, lefts: [0, 100] },
    )
    expect(isOtanim(bytes)).toBe(true)
    const file = await parseOtanim(bytes)
    expect(file.name).toBe('fixture')
    expect(file.view).toEqual([-10, -10, 210, 110])
    expect(Array.from(file.parts[0].indices)).toEqual([0, 1, 2, 2, 3, 0])
    const idle = file.clips[0]
    expect(idle.frames).toHaveLength(3)
    // frames after the first are stored as changes; they read back whole
    expect(idle.positions[idle.frames[2][0].offset]).toBe(100)
    expect(idle.positions[idle.frames[2][0].offset + 5]).toBe(100)
  })

  it('refuses what is not one', async () => {
    expect(isOtanim(new Uint8Array([0x89, 0x50, 0x4e, 0x47]))).toBe(false)
    const bytes = await fixture({ name: 'idle', loop: true, fps: 10, lefts: [0] })
    await expect(parseOtanim(bytes.slice(0, bytes.length - 40))).rejects.toThrow()
    bytes[4] = 9
    await expect(parseOtanim(bytes)).rejects.toThrow(/version/)
  })

  it('plays an intro once, then loops the rest; a one-shot holds', async () => {
    const file = await parseOtanim(
      await fixture(
        { name: 'gogo', loop: true, fps: 10, lefts: [0, 10, 20, 30, 40], loopStart: 2 },
        { name: 'happy', loop: false, fps: 4, lefts: [0, 100] },
      ),
    )
    const [gogo, happy] = file.clips
    expect(otanimFrameAt(gogo, 0.15)).toMatchObject({ frame: 1, next: 2 })
    expect(otanimFrameAt(gogo, 0.45)).toMatchObject({ frame: 4, next: 2 })
    expect(otanimFrameAt(gogo, 0.85).frame).toBe(2)
    expect(otanimFrameAt(happy, 5)).toEqual({ frame: 1, next: -1, toward: 0 })
  })

  it('picks the clip the motion asks for', async () => {
    const file = await parseOtanim(
      await fixture(
        { name: 'idle', loop: true, fps: 10, lefts: [0] },
        { name: 'gogo', loop: true, fps: 10, lefts: [0] },
      ),
    )
    expect(otanimClipFor(file, 'Normal').name).toBe('idle')
    expect(otanimClipFor(file, 'Sabi').name).toBe('gogo')
    expect(otanimClipFor(file, 'Miss').name).toBe('idle')
  })

  it('eases frames and fits the box', async () => {
    const file = await parseOtanim(
      await fixture({ name: 'idle', loop: true, fps: 10, lefts: [0, 50, 100] }),
    )
    const box: [number, number, number, number] = [-220, -120, 220, 120]
    let m = otanimMesh(file, file.clips[0], 0, box, false)
    expect(m.positions[0]).toBeCloseTo(-200)
    expect(m.positions[1]).toBeCloseTo(-100)
    expect(Array.from(m.indices)).toEqual([0, 1, 2, 2, 3, 0])
    m = otanimMesh(file, file.clips[0], 0.025, box, false)
    expect(m.positions[0]).toBeCloseTo(-175)
    m = otanimMesh(file, file.clips[0], 0, box, true)
    expect(m.positions[0]).toBeCloseTo(200)
    expect(m.uvs[3]).toBeCloseTo(0) // v down from the image's top, as stored
  })
})

describe('OtanimPlayer', () => {
  it('loops each clip, cuts only the resting one, lets the rest finish their pass', async () => {
    const file = await parseOtanim(
      await fixture(
        { name: 'idle', loop: true, fps: 10, lefts: [0, 10, 20, 30] }, // 0.4 s
        { name: 'happy', loop: false, fps: 10, lefts: [0, 10, 20, 30, 40] }, // 0.4 s, once
        { name: 'gogo', loop: true, fps: 10, lefts: [0, 10, 20, 30, 40, 50], loopStart: 2 }, // 0.6 s, then 0.4 s loops
      ),
    )
    const [idle, happy, gogo] = file.clips
    const player = new OtanimPlayer(file)
    expect(player.update(0, idle).clip).toBe(idle)
    // idle gives way at once
    expect(player.update(150, happy)).toEqual({ clip: happy, seconds: 0 })
    // the Don went back to Normal: happy plays on to its end…
    expect(player.update(400, idle).clip).toBe(happy)
    // …and is asked for again: it starts over instead of holding its last frame
    expect(player.update(500, happy).seconds).toBeCloseTo(0.35)
    expect(player.update(600, happy).seconds).toBeCloseTo(0.05)
    // asked for idle after a pass ended: idle
    expect(player.update(950, idle).clip).toBe(idle)
    expect(player.update(1000, gogo).clip).toBe(gogo)
    // GOGO over: the intro and the loop's first pass play out (0.6 s), then idle
    expect(player.update(1300, idle).clip).toBe(gogo)
    expect(player.update(1550, idle).clip).toBe(gogo)
    expect(player.update(1650, idle).clip).toBe(idle)
  })
})
