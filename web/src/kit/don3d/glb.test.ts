import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { donMotionNames, miss6Mirror, missMirror } from './chara'
import { Glb, type GlbJson } from './glb'
import { DonMotions } from './motions'
import { classifyMaterials, facePlaneSize, loadDonPart } from './part'

// A glb from JSON and a BIN chunk.
function makeGlb(json: GlbJson, bin: Uint8Array): Uint8Array {
  const pad = (n: number) => (n + 3) & ~3
  const jsonBytes = new TextEncoder().encode(JSON.stringify(json))
  const jsonLength = pad(jsonBytes.length)
  const binLength = pad(bin.length)
  const out = new Uint8Array(12 + 8 + jsonLength + 8 + binLength)
  const view = new DataView(out.buffer)
  view.setUint32(0, 0x46546c67, true)
  view.setUint32(4, 2, true)
  view.setUint32(8, out.length, true)
  view.setUint32(12, jsonLength, true)
  view.setUint32(16, 0x4e4f534a, true)
  out.fill(0x20, 20, 20 + jsonLength)
  out.set(jsonBytes, 20)
  view.setUint32(20 + jsonLength, binLength, true)
  view.setUint32(24 + jsonLength, 0x004e4942, true)
  out.set(bin, 28 + jsonLength)
  return out
}

describe('Glb', () => {
  const bin = new Uint8Array(32)
  const view = new DataView(bin.buffer)
  view.setFloat32(0, 1.5, true)
  view.setFloat32(4, -2, true)
  view.setFloat32(8, 0.25, true)
  bin.set([0, 128, 255, 9], 12) // UNSIGNED_BYTE, normalised and not
  view.setUint16(16, 65535, true)
  view.setUint16(18, 7, true)
  const glb = Glb.parse(
    makeGlb(
      {
        bufferViews: [
          { byteOffset: 0, byteLength: 12 },
          { byteOffset: 12, byteLength: 4 },
          { byteOffset: 16, byteLength: 4 },
        ],
        accessors: [
          { bufferView: 0, componentType: 5126, count: 1, type: 'VEC3' },
          { bufferView: 1, componentType: 5121, count: 3, type: 'SCALAR', normalized: true },
          { bufferView: 2, componentType: 5123, count: 2, type: 'SCALAR' },
          { bufferView: 1, byteOffset: 3, componentType: 5121, count: 1, type: 'SCALAR' },
        ],
      },
      bin,
    ),
  )

  it('reads floats, normalised integers and indices', () => {
    expect([...glb.floats(0)]).toEqual([1.5, -2, 0.25])
    expect([...glb.floats(1)]).toEqual([0, 128 / 255, 1].map(Math.fround))
    expect([...glb.ints(2)]).toEqual([65535, 7])
    expect([...glb.ints(3)]).toEqual([9])
    expect(glb.count(2)).toBe(2)
  })

  it('refuses what is not a glb', () => {
    expect(() => Glb.parse(new Uint8Array(40))).toThrow(/magic/)
    expect(() => Glb.parse(new Uint8Array(4))).toThrow(/short/)
  })
})

describe('classifyMaterials', () => {
  const glb = Glb.parse(
    makeGlb(
      {
        textures: [{ source: 2 }],
        materials: [
          {
            name: 'RGB_don_color_S_CUS_0x10000001_',
            extras: { shaderType: 'taikoEffectChangeColors' },
          },
          {
            name: 'face2_FACIAL_S_CUS_0x10000002_A_AB_AA_ADD',
            extras: { shaderType: 'taikoEffectFace' },
          },
          { name: 'BODY__color_S_CUS_0x10000000__AT_ONE__CULLNONE', alphaMode: 'BLEND' },
          {
            name: 'BODY__color_S_CUS_0x10000000__A_AB_AA_ADD_AT_ONE__CULLNONE',
            alphaMode: 'BLEND',
            pbrMetallicRoughness: {
              baseColorTexture: { index: 0 },
              baseColorFactor: [0.5, 0.5, 0.5, 1],
            },
          },
          { name: 'lambert2', alphaMode: 'BLEND' },
        ],
      },
      new Uint8Array(4),
    ),
  )
  const { materials, faceMaterial } = classifyMaterials(glb)

  it('sorts materials as YataiDON draws them', () => {
    expect(faceMaterial).toBe(1)
    expect(materials[0]).toMatchObject({ recolor: true, cutout: true, blend: false })
    // the face is never additive or blended
    expect(materials[1]).toMatchObject({ face: true, additive: false, blend: false })
    // alpha-tested, not blended, two-sided
    expect(materials[2]).toMatchObject({ cutout: true, blend: false, twoSided: true })
    // additive glint: blended, white, its texture
    expect(materials[3]).toMatchObject({
      additive: true,
      blend: true,
      image: 2,
      color: [1, 1, 1, 1],
    })
    // a plain glTF BLEND sheet is blended
    expect(materials[4]).toMatchObject({ blend: true, cutout: false })
  })
})

// The bucket's files, mirrored locally (tools/3dchara); skipped where they are not.
const assets = process.env.DON3D_ASSETS ?? resolve(process.cwd(), '../../3dchara-staging')
const haveAssets = existsSync(resolve(assets, 'catalog.json'))
const read = (path: string) => new Uint8Array(readFileSync(resolve(assets, path)))

describe.skipIf(!haveAssets)('the 3dchara files', () => {
  it('reads a costume: skinned primitives, a face plate, its pictures', () => {
    const part = loadDonPart(read('cos/0.glb'), false)
    expect(part.primitives.length).toBe(3)
    expect(part.faceMaterial).toBe(2)
    expect(part.jointNames.length).toBeGreaterThan(10)
    expect(part.inverseBind.length).toBe(part.jointNames.length * 16)
    for (const prim of part.primitives) {
      const n = prim.positions.length / 3
      expect(prim.normals.length).toBe(n * 3)
      expect(prim.uv.length).toBe(n * 2)
      expect(prim.joints?.length).toBe(n * 4)
      expect(Math.max(...prim.indices)).toBeLessThan(n)
    }
    expect(part.images.every((image) => image?.mimeType === 'image/png')).toBe(true)
    // PNG signature
    expect([...part.images[0]!.bytes.subarray(0, 4)]).toEqual([0x89, 0x50, 0x4e, 0x47])
  })

  const plate = (path: string, normalize: boolean) => {
    const part = loadDonPart(read(path), normalize)
    const prim = part.primitives.find((p) => p.material === part.faceMaterial)!
    let minX = Infinity
    let maxX = -Infinity
    let minY = Infinity
    let maxY = -Infinity
    for (let i = 0; i < prim.positions.length; i += 3) {
      minX = Math.min(minX, prim.positions[i])
      maxX = Math.max(maxX, prim.positions[i])
      minY = Math.min(minY, prim.positions[i + 1])
      maxY = Math.max(maxY, prim.positions[i + 1])
    }
    return Math.max(maxX - minX, maxY - minY)
  }

  it('fits the face plate: always for a head, only an oversized one for a costume', () => {
    expect(plate('head/0.glb', true)).toBeCloseTo(facePlaneSize, 5)
    expect(plate('cos/30.glb', false)).toBeCloseTo(facePlaneSize, 5)
    expect(plate('cos/0.glb', false)).toBeLessThanOrEqual(facePlaneSize * 1.02)
  })

  it('reads the 52 motions in DonAnim order', () => {
    const motions = DonMotions.load(read('base/animations.glb'))
    expect(motions.count).toBe(52)
    donMotionNames.forEach((name, i) =>
      expect(motions.names[i].replace(/_/g, '')).toBe('don' + name.toLowerCase()),
    )
    expect(motions.names[miss6Mirror]).toBe('don_miss6_mirror')
    expect(motions.names[missMirror]).toBe('don_miss_mirror')
    const part = loadDonPart(read('cos/0.glb'), false)
    // every joint of a part is in the skeleton
    expect(part.jointNames.every((name) => motions.joints.has(name))).toBe(true)
    const normal = motions.posesOf(20)
    expect(motions.frameCounts[20]).toBeGreaterThan(1)
    expect(normal.length).toBe(motions.frameCounts[20] * motions.jointCount * 16)
    expect(normal.every(Number.isFinite)).toBe(true)
    expect(motions.posesOf(20)).toBe(normal) // sampled once
  })
})
