import { describe, expect, it } from 'vitest'
import {
  baseAnim,
  DonAnim,
  DonAnimator,
  donMotionNames,
  missMirror,
  miss6Mirror,
  modelMatrix,
  parseHexColor,
  recolorPixels,
} from './chara'
import { FacePlayer, parseFaceTimelines } from './face'
import { catalogUrl, resolveDonLook, type DonCatalog } from './catalog'
import type { DonPartData } from './part'
import { paintPlaneScale, scaleFacePlates } from './renderer'

// animations.glb's names: the 26 motions, then their _mirror versions with miss6 before miss.
function motionNames(): string[] {
  const snake = donMotionNames.map((n) => 'don_' + n.replace(/(?<!^)([A-Z])/g, '_$1').toLowerCase())
  const mirrored = snake.map((n) => n + '_mirror')
  ;[mirrored[14], mirrored[15]] = [mirrored[15], mirrored[14]]
  return [...snake, ...mirrored]
}

const frames = 11 // ten frames a loop
const motions = { names: motionNames(), frameCounts: motionNames().map(() => frames) }
const timelines = parseFaceTimelines([
  {
    id: 46,
    type: 'texture_change',
    duration: 1000,
    loop: true,
    textures: [
      [0, 650, 4],
      [650, 1000, 0],
    ],
  },
  {
    id: 17,
    type: 'texture_change',
    duration: 1000,
    textures: [
      [0, 200, 4],
      [200, 1000, 7],
    ],
  },
])

// Steps the animator a keyframe at a time until it has run `count` of them.
function run(animator: DonAnimator, start: number, count: number): number {
  let t = start
  for (let i = 0; i < count; i++) animator.update((t += 1000), t)
  return t
}

describe('motion names', () => {
  it('match animations.glb order, with miss6 and miss swapped in the mirrored half', () => {
    const names = motionNames()
    expect(names[DonAnim.Normal]).toBe('don_normal')
    expect(names[DonAnim.SelectLoop]).toBe('don_select_loop')
    expect(names[miss6Mirror]).toBe('don_miss6_mirror')
    expect(names[missMirror]).toBe('don_miss_mirror')
  })
})

describe('baseAnim', () => {
  it('maps the mirrored half back, 40 and 41 the right way round', () => {
    expect(baseAnim(DonAnim.Combo)).toBe(DonAnim.Combo)
    expect(baseAnim(DonAnim.Combo + 26)).toBe(DonAnim.Combo)
    expect(baseAnim(40)).toBe(DonAnim.Miss6)
    expect(baseAnim(41)).toBe(DonAnim.Miss)
  })
})

describe('DonAnimator', () => {
  it('starts on DON_NORMAL with its face timeline', () => {
    const a = new DonAnimator(motions, timelines, 12, 0)
    expect(a.animIndex).toBe(DonAnim.Normal)
    expect(a.face).toBe(4)
    a.update(700, 700)
    expect(a.face).toBe(0)
  })

  it('returns a one-shot to the loop it interrupted', () => {
    const a = new DonAnimator(motions, timelines, 12, 0)
    a.setAnim(DonAnim.SelectLoop, 0)
    a.setAnim(DonAnim.Combo, 0)
    expect(a.animIndex).toBe(DonAnim.Combo)
    const t = run(a, 0, frames - 3)
    expect(a.animIndex).toBe(DonAnim.Combo)
    expect(a.animFrame).toBe(frames - 3)
    run(a, t, 1)
    expect(a.animIndex).toBe(DonAnim.SelectLoop)
    expect(a.animFrame).toBe(0)
  })

  it('does not return a chained one-shot to the one it cut off', () => {
    const a = new DonAnimator(motions, timelines, 12, 0)
    a.setAnim(DonAnim.Combo, 0)
    a.setAnim(DonAnim.FullCombo, 0)
    run(a, 0, frames - 2)
    expect(a.animIndex).toBe(DonAnim.Normal)
  })

  it('loops a loop for ever', () => {
    const a = new DonAnimator(motions, timelines, 12, 0)
    run(a, 0, frames * 3)
    expect(a.animIndex).toBe(DonAnim.Normal)
    expect(a.animFrame).toBe((frames * 3) % (frames - 1))
  })

  it('steps by the tempo: DON_NORMAL lasts three beats', () => {
    const a = new DonAnimator(motions, timelines, 12, 0)
    a.bpm = 120
    const msPerFrame = (500 * 3) / frames
    a.update(msPerFrame, 0) // the first update steps (lastFrameMs starts at 0)
    expect(a.animFrame).toBe(1)
    a.update(msPerFrame * 1.5, 0)
    expect(a.animFrame).toBe(1)
    a.update(msPerFrame * 2.01, 0)
    expect(a.animFrame).toBe(2)
  })

  it('plays the mirrored motions when turned round, miss at 41 and miss6 at 40', () => {
    const a = new DonAnimator(motions, timelines, 12, 0)
    a.setMirror(true, 0)
    expect(a.animIndex).toBe(DonAnim.Normal + 26)
    a.setAnim(DonAnim.Miss, 0)
    expect(a.animIndex).toBe(41)
    expect(a.animName(a.animIndex)).toBe('don_miss_mirror')
    a.setAnim(DonAnim.Miss6, 0)
    expect(a.animIndex).toBe(40)
    expect(a.animName(a.animIndex)).toBe('don_miss6_mirror')
    // turning round keeps miss6 miss6 (YataiDON's index % 26 made it miss)
    a.setMirror(false, 0)
    expect(a.animIndex).toBe(DonAnim.Miss6)
    a.setMirror(true, 0)
    expect(a.animIndex).toBe(40)
  })

  it('returns a mirrored one-shot to the mirrored loop', () => {
    const a = new DonAnimator(motions, timelines, 12, 0)
    a.setMirror(true, 0)
    a.setAnim(DonAnim.Miss, 0)
    run(a, 0, frames - 2)
    expect(a.animIndex).toBe(DonAnim.Normal + 26)
  })

  it('keeps the frame when turned round', () => {
    const a = new DonAnimator(motions, timelines, 12, 0)
    run(a, 0, 4)
    a.setMirror(true, 0)
    expect(a.animFrame).toBe(4)
  })

  it('holds a frozen motion at a point of its loop', () => {
    const a = new DonAnimator(motions, timelines, 12, 0)
    a.setFrozen(true)
    a.setAnimProgress(0.25)
    expect(a.animFrame).toBe(2)
    run(a, 0, 5)
    expect(a.animFrame).toBe(2)
  })

  it('holds the revive face through FUKKATU', () => {
    const a = new DonAnimator(motions, timelines, 12, 0)
    a.setAnim(DonAnim.FukkatuStart, 0)
    expect(a.animIndex).toBe(DonAnim.FukkatuStart)
    expect(a.face).toBe(8)
    run(a, 0, 3)
    expect(a.face).toBe(8)
  })
})

describe('FacePlayer', () => {
  const [timeline] = parseFaceTimelines([
    {
      id: 13,
      type: 'texture_change',
      duration: 1000,
      textures: [
        [0, 400, 11],
        [400, 840, 10],
        [840, 1000, 11],
      ],
    },
    { id: 99, type: 'other' },
  ]).values()

  it('shows the first face, then each face in its window', () => {
    const p = new FacePlayer(timeline, 100)
    expect(p.face).toBe(11)
    p.update(600)
    expect(p.face).toBe(10)
    p.update(1000)
    expect(p.face).toBe(11)
  })

  it('stops at the end unless it loops', () => {
    const p = new FacePlayer(timeline, 0)
    p.update(500)
    p.update(1200) // past the end: finished, face stays
    expect(p.face).toBe(10)
    p.update(1500)
    expect(p.face).toBe(10)
    const looping = new FacePlayer({ ...timeline, loop: true }, 0)
    looping.update(500)
    looping.update(1200)
    looping.update(1700) // starts again at 1700
    looping.update(2200)
    expect(looping.face).toBe(10)
    looping.update(2600)
    expect(looping.face).toBe(11)
  })
})

describe('recolorPixels', () => {
  const body: [number, number, number] = [104, 191, 192]
  const face: [number, number, number] = [249, 71, 40]
  const rim: [number, number, number] = [249, 240, 225]

  it('turns reds to the body colour, greens to the face colour, blues to the rim; keeps alpha', () => {
    const px = new Uint8Array([200, 30, 30, 128, 20, 200, 30, 255, 30, 30, 200, 255, 0, 0, 255, 7])
    recolorPixels(px, body, face, rim)
    expect([...px]).toEqual([...body, 128, ...face, 255, ...rim, 255, ...rim, 7])
  })

  it('leaves greys and near-blacks alone', () => {
    const px = new Uint8Array([120, 125, 130, 255, 10, 0, 5, 255, 255, 255, 255, 255])
    const before = [...px]
    recolorPixels(px, body, face, rim)
    expect([...px]).toEqual(before)
  })

  it('breaks ties as YataiDON does', () => {
    // blue equal to green and above red: rim; green equal to red: body
    const px = new Uint8Array([10, 200, 200, 255, 200, 200, 10, 255])
    recolorPixels(px, body, face, rim)
    expect([...px]).toEqual([...rim, 255, ...body, 255])
  })

  it('reads catalog colours', () => {
    expect(parseHexColor('#68BFC0')).toEqual([104, 191, 192])
    expect(parseHexColor('f94728')).toEqual([249, 71, 40])
  })
})

describe('modelMatrix', () => {
  it('stands the Don upright in a y-down area, turned toward the viewer', () => {
    const m = modelMatrix(100, 200, 1000, false)
    const e = m.elements
    // glTF +y (up) goes to -y on screen (up in a y-down area)
    expect(e[5]).toBeLessThan(-900)
    // the feet's origin lands on (x, y)
    expect(e[12]).toBeCloseTo(100)
    expect(e[13]).toBeCloseTo(200)
    // mirrored turns the other way
    expect(Math.sign(modelMatrix(0, 0, 1, true).elements[8])).toBe(-Math.sign(e[8]))
  })
})

describe('resolveDonLook', () => {
  const file = (path: string) => ({ path, size: 1, sha256: 'abcdef0123456789' })
  const item = (kind: string, id: number) => ({ id, name: {}, file: file(`${kind}/${id}.glb`) })
  const catalog: DonCatalog = {
    version: 1,
    base: { animations: file('a'), face: file('f'), faceAnimations: file('j') },
    defaultColors: { p1: ['#68BFC0', '#F94728', '#F9F0E1'], p2: ['#F94728', '#68BFC0', '#F9F0E1'] },
    items: {
      cos: [item('cos', 0), item('cos', 30)],
      head: [item('head', 0), item('head', 3)],
      body: [item('body', 0), item('body', 4)],
      puchi: [{ id: 1, name: {}, file: file('puchi/1.png') }],
    },
  }
  const look = { costume: true, cos: 30, head: 3, body: 4, puchi: 0, colors: null }

  it('wears a costume, or a body then a head with its face normalised', () => {
    expect(
      resolveDonLook(catalog, look).parts.map((p) => [p.kind, p.item.id, p.normalizeFace]),
    ).toEqual([['cos', 30, false]])
    expect(
      resolveDonLook(catalog, { ...look, costume: false }).parts.map((p) => [
        p.kind,
        p.item.id,
        p.normalizeFace,
      ]),
    ).toEqual([
      ['body', 4, false],
      ['head', 3, true],
    ])
  })

  it('falls back to 0, no puchichara for 0, and 1P colours', () => {
    const r = resolveDonLook(catalog, { ...look, cos: 999, puchi: 0 })
    expect(r.parts[0].item.id).toBe(0)
    expect(r.puchi).toBeNull()
    expect(r.colors).toEqual(catalog.defaultColors.p1)
    expect(resolveDonLook(catalog, { ...look, puchi: 1 }).puchi?.id).toBe(1)
    expect(resolveDonLook(catalog, { ...look, puchi: 5 }).puchi).toBeNull()
  })

  it('wears makeup the catalog has; none for 0, absent, or a catalog without makeup', () => {
    const withPaint: DonCatalog = {
      ...catalog,
      items: { ...catalog.items, paint: [{ id: 2, name: {}, file: file('paint/2.png') }] },
    }
    expect(resolveDonLook(withPaint, { ...look, paint: 2 }).paint?.id).toBe(2)
    expect(resolveDonLook(withPaint, { ...look, paint: 0 }).paint).toBeNull()
    expect(resolveDonLook(withPaint, look).paint).toBeNull()
    expect(resolveDonLook(withPaint, { ...look, paint: 9 }).paint).toBeNull()
    expect(resolveDonLook(catalog, { ...look, paint: 2 }).paint).toBeNull()
  })

  it('builds versioned URLs', () => {
    expect(catalogUrl('https://distrib.ourtaiko.org/3dchara', file('cos/0.glb'))).toBe(
      'https://distrib.ourtaiko.org/3dchara/cos/0.glb',
    )
  })

  it('uses the mirror only for files listed by the API', () => {
    const base = 'https://distrib.ourtaiko.org/3dchara/'
    const mirror = {
      mirrorBaseUrl: 'https://static.sakura-bot.cn/3dchara/',
      mirrorPaths: ['head/3.glb'],
    }
    expect(catalogUrl(base, file('head/3.glb'), mirror)).toBe(
      'https://static.sakura-bot.cn/3dchara/head/3.glb',
    )
    expect(catalogUrl(base, file('body/4.glb'), mirror)).toBe(base + 'body/4.glb')
    expect(catalogUrl(base, file('head/3.glb'), {})).toBe(base + 'head/3.glb')
  })
})

describe('scaleFacePlates', () => {
  // a face plate 0.2 wide around (1, 2, 3), and a body triangle that must not move
  const plate = new Float32Array([0.9, 1.9, 3, 1.1, 1.9, 3, 1.1, 2.1, 3, 0.9, 2.1, 3])
  const body = new Float32Array([0, 0, 0, 1, 0, 0, 0, 1, 0])
  const data = {
    primitives: [
      { material: 0, positions: plate },
      { material: 1, positions: body },
    ],
    materials: [{ face: true }, { face: false }],
    jointNames: [],
  } as unknown as DonPartData
  const part = () => ({ data, sources: [plate, body], faceScale: 1 })

  it('makes each plate a third larger about its centre, and back', () => {
    const p = part()
    scaleFacePlates(p, paintPlaneScale)
    const s = p.sources[0]
    expect(s[0]).toBeCloseTo(1 - 0.1 * paintPlaneScale)
    expect(s[4]).toBeCloseTo(2 - 0.1 * paintPlaneScale)
    expect(s[6]).toBeCloseTo(1 + 0.1 * paintPlaneScale)
    expect(s[2]).toBeCloseTo(3)
    expect(p.sources[1]).toBe(body)
    expect(plate[0]).toBeCloseTo(0.9) // the shared part data is untouched
    scaleFacePlates(p, 1)
    expect(p.sources[0]).toBe(plate)
  })
})
