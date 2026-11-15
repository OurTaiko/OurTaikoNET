// The parts of glTF 2.0 binary (.glb) the Don models use: the JSON chunk, the BIN chunk, and
// accessors read out as numbers. Everything stays in glTF's own space (right-handed, +Y up, UV
// origin at the top left), as the game's Glb.cs and YataiDON draw it. Textures are uploaded
// without flipping, so glTF UVs are used as they are.

export type GlbJson = {
  accessors?: GlbAccessor[]
  bufferViews?: { buffer?: number; byteOffset?: number; byteLength: number; byteStride?: number }[]
  images?: { bufferView?: number; mimeType?: string; uri?: string }[]
  textures?: { source?: number }[]
  materials?: GlbMaterial[]
  meshes?: { primitives: GlbPrimitive[] }[]
  nodes?: GlbNode[]
  skins?: { joints: number[]; inverseBindMatrices?: number }[]
  animations?: {
    name?: string
    channels: { sampler: number; target: { node?: number; path: string } }[]
    samplers: { input: number; output: number; interpolation?: string }[]
  }[]
}

export type GlbAccessor = {
  bufferView?: number
  byteOffset?: number
  componentType: number
  count: number
  type: string
  normalized?: boolean
}

export type GlbMaterial = {
  name?: string
  alphaMode?: string
  extras?: { shaderType?: string }
  pbrMetallicRoughness?: {
    baseColorTexture?: { index: number }
    baseColorFactor?: number[]
  }
}

export type GlbPrimitive = {
  attributes: Record<string, number>
  indices?: number
  material?: number
  mode?: number
}

export type GlbNode = {
  name?: string
  children?: number[]
  matrix?: number[]
  translation?: number[]
  rotation?: number[]
  scale?: number[]
}

const magic = 0x46546c67 // "glTF"
const jsonChunk = 0x4e4f534a // "JSON"
const binChunk = 0x004e4942 // "BIN\0"

const components: Record<string, number> = {
  SCALAR: 1,
  VEC2: 2,
  VEC3: 3,
  VEC4: 4,
  MAT2: 4,
  MAT3: 9,
  MAT4: 16,
}

function componentSize(type: number): number {
  switch (type) {
    case 5120:
    case 5121:
      return 1
    case 5122:
    case 5123:
      return 2
    case 5125:
    case 5126:
      return 4
  }
  throw new Error('glb: component type ' + type)
}

export class Glb {
  readonly json: GlbJson
  private readonly view: DataView
  private readonly bytes: Uint8Array
  private readonly binStart: number

  private constructor(json: GlbJson, bytes: Uint8Array, binStart: number) {
    this.json = json
    this.bytes = bytes
    this.view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength)
    this.binStart = binStart
  }

  static parse(data: ArrayBuffer | Uint8Array): Glb {
    const bytes = data instanceof Uint8Array ? data : new Uint8Array(data)
    if (bytes.byteLength < 20) throw new Error('glb: too short')
    const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength)
    if (view.getUint32(0, true) !== magic) throw new Error('glb: bad magic')
    const jsonLength = view.getInt32(12, true)
    if (view.getUint32(16, true) !== jsonChunk || 20 + jsonLength > bytes.byteLength)
      throw new Error('glb: no JSON chunk')
    const json = JSON.parse(
      new TextDecoder().decode(bytes.subarray(20, 20 + jsonLength)),
    ) as GlbJson
    let bin = -1
    const next = 20 + jsonLength
    if (next + 8 <= bytes.byteLength && view.getUint32(next + 4, true) === binChunk) bin = next + 8
    return new Glb(json, bytes, bin)
  }

  // Where an accessor's elements are: first byte, stride, count, components per element.
  private layout(index: number) {
    if (this.binStart < 0) throw new Error('glb: no BIN chunk')
    const a = this.json.accessors?.[index]
    if (!a) throw new Error('glb: no accessor ' + index)
    const width = components[a.type]
    if (!width) throw new Error('glb: accessor type ' + a.type)
    const view = this.json.bufferViews?.[a.bufferView ?? -1]
    if (!view) throw new Error('glb: accessor without a buffer view')
    const start = this.binStart + (view.byteOffset ?? 0) + (a.byteOffset ?? 0)
    const stride = view.byteStride ?? width * componentSize(a.componentType)
    return {
      start,
      stride,
      count: a.count,
      width,
      type: a.componentType,
      normalized: !!a.normalized,
    }
  }

  count(index: number): number {
    return this.json.accessors?.[index]?.count ?? 0
  }

  // Every component as a float; normalized integers come out in [0, 1] (or [-1, 1]).
  floats(index: number): Float32Array {
    const { start, stride, count, width, type, normalized } = this.layout(index)
    const result = new Float32Array(count * width)
    const size = componentSize(type)
    const v = this.view
    for (let i = 0; i < count; i++)
      for (let c = 0; c < width; c++) {
        const at = start + i * stride + c * size
        let value: number
        switch (type) {
          case 5126:
            value = v.getFloat32(at, true)
            break
          case 5121:
            value = normalized ? v.getUint8(at) / 255 : v.getUint8(at)
            break
          case 5120:
            value = normalized ? Math.max(v.getInt8(at) / 127, -1) : v.getInt8(at)
            break
          case 5123:
            value = normalized ? v.getUint16(at, true) / 65535 : v.getUint16(at, true)
            break
          case 5122:
            value = normalized ? Math.max(v.getInt16(at, true) / 32767, -1) : v.getInt16(at, true)
            break
          default:
            value = v.getUint32(at, true)
        }
        result[i * width + c] = value
      }
    return result
  }

  ints(index: number): Uint32Array {
    const { start, stride, count, width, type } = this.layout(index)
    const result = new Uint32Array(count * width)
    const size = componentSize(type)
    const v = this.view
    for (let i = 0; i < count; i++)
      for (let c = 0; c < width; c++) {
        const at = start + i * stride + c * size
        let value: number
        switch (type) {
          case 5121:
            value = v.getUint8(at)
            break
          case 5120:
            value = v.getInt8(at)
            break
          case 5123:
            value = v.getUint16(at, true)
            break
          case 5122:
            value = v.getInt16(at, true)
            break
          case 5125:
            value = v.getUint32(at, true)
            break
          default:
            value = Math.trunc(v.getFloat32(at, true))
        }
        result[i * width + c] = value
      }
    return result
  }

  // An image's encoded bytes (PNG/JPEG) from its buffer view, copied out.
  imageBytes(index: number): { bytes: Uint8Array; mimeType: string } | null {
    const image = this.json.images?.[index]
    if (!image || image.bufferView === undefined || this.binStart < 0) return null
    const view = this.json.bufferViews?.[image.bufferView]
    if (!view) return null
    const start = this.binStart + (view.byteOffset ?? 0)
    return {
      bytes: this.bytes.slice(start, start + view.byteLength),
      mimeType: image.mimeType ?? 'image/png',
    }
  }
}
