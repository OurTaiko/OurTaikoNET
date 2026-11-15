import { Glb } from './glb'

// One part of a Don (a costume, a head or a body) as its glb has it, shared by every Don wearing
// it: the primitives in bind pose, the skin, the materials sorted by how YataiDON draws them, and
// the encoded images. Read-only once loaded (the game's DonPart).

export type DonPrimitive = {
  positions: Float32Array // 3 per vertex
  normals: Float32Array // 3 per vertex
  uv: Float32Array // 2 per vertex, glTF's top-left origin
  thickness: Float32Array // COLOR_0.g: the black line pass's per-vertex thickness
  joints: Uint32Array | null // 4 per vertex, into the part's skin
  weights: Float32Array | null // 4 per vertex
  indices: Uint32Array
  material: number
}

export type DonMaterial = {
  name: string
  image: number
  color: [number, number, number, number]
  recolor: boolean
  face: boolean
  additive: boolean
  cutout: boolean
  blend: boolean
  twoSided: boolean
}

export type DonImage = { bytes: Uint8Array; mimeType: string }

export type DonPartData = {
  primitives: DonPrimitive[]
  materials: DonMaterial[]
  images: (DonImage | null)[]
  jointNames: string[]
  inverseBind: Float32Array // 16 per joint, column-major
  faceMaterial: number
}

// head/body parts always get the standard 0.137 face plate; a costume keeps its own plate (some
// are deliberately small) but never a larger one -- cos 30 (鏡もち) and cos 9 ship a 0.18 plate that
// overflows the drum head (YataiDON Chara3D::load_part).
export const facePlaneSize = 0.137

// Which draw stage a material belongs to: 0 opaque and alpha-tested, 1 the face plate, 2 the
// alpha-blended sheets (front hair, plates, glints), drawn last so their clear texels do not
// hide the face behind them.
export function drawStage(material: DonMaterial): 0 | 1 | 2 {
  return material.face ? 1 : material.blend ? 2 : 0
}

// YataiDON parse_glb_material_indices: the shader type in extras and the cabinet's shader in the
// material's name.
export function classifyMaterials(glb: Glb): { materials: DonMaterial[]; faceMaterial: number } {
  const textures = glb.json.textures ?? []
  const materials: DonMaterial[] = []
  let faceMaterial = -1
  for (const m of glb.json.materials ?? []) {
    const info: DonMaterial = {
      name: m.name ?? '',
      image: -1,
      color: [1, 1, 1, 1],
      recolor: false,
      face: false,
      additive: false,
      cutout: false,
      blend: false,
      twoSided: false,
    }
    const pbr = m.pbrMetallicRoughness
    if (pbr?.baseColorTexture) info.image = textures[pbr.baseColorTexture.index]?.source ?? -1
    const f = pbr?.baseColorFactor
    if (f && f.length === 4) info.color = [f[0], f[1], f[2], f[3]]
    const shader = m.extras?.shaderType
    if (shader === 'taikoEffectChangeColors') info.recolor = true
    else if (shader === 'taikoEffectFace' && faceMaterial < 0) {
      info.face = true
      faceMaterial = materials.length
    }
    const name = info.name.toLowerCase()
    const cutout = name.includes('_color_s_cus_') && !name.includes('_a_ab')
    const gltfBlend = m.alphaMode === 'BLEND'
    if (name.includes('_aa_add')) info.additive = true
    else if (cutout) info.cutout = true
    // alpha-blended: _A_AB by name, or any other glTF BLEND material that is not an alpha-tested
    // one (e.g. a plain "lambert" glint sheet) -- drawn after the face
    if (name.includes('_a_ab') || (gltfBlend && !cutout)) info.blend = true
    if (name.includes('cullnone')) info.twoSided = true
    if (info.face) {
      info.additive = false
      info.blend = false
    }
    if (info.additive) info.color = [1, 1, 1, 1]
    materials.push(info)
  }
  return { materials, faceMaterial }
}

export function loadDonPart(data: ArrayBuffer | Uint8Array, normalizeFace: boolean): DonPartData {
  const glb = Glb.parse(data)
  const { materials, faceMaterial } = classifyMaterials(glb)
  const images = (glb.json.images ?? []).map((_, i) => glb.imageBytes(i))

  let jointNames: string[] = []
  let inverseBind = new Float32Array(0)
  const skin = glb.json.skins?.[0]
  if (skin) {
    const nodes = glb.json.nodes ?? []
    jointNames = skin.joints.map((j) => nodes[j]?.name ?? '')
    const ibm = skin.inverseBindMatrices !== undefined ? glb.floats(skin.inverseBindMatrices) : null
    inverseBind = new Float32Array(skin.joints.length * 16)
    for (let j = 0; j < skin.joints.length; j++) {
      if (ibm) inverseBind.set(ibm.subarray(j * 16, j * 16 + 16), j * 16)
      else inverseBind.set([1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1], j * 16)
    }
  }

  // Every mesh of the file; a skinned mesh ignores its node's transform (glTF 2.0 §3.7.3.3).
  const primitives: DonPrimitive[] = []
  for (const mesh of glb.json.meshes ?? [])
    for (const p of mesh.primitives) {
      if ((p.mode ?? 4) !== 4) continue
      const a = p.attributes
      if (a.POSITION === undefined) continue
      const positions = glb.floats(a.POSITION)
      const n = positions.length / 3
      const normals = a.NORMAL !== undefined ? glb.floats(a.NORMAL) : new Float32Array(n * 3)
      const uv = a.TEXCOORD_0 !== undefined ? glb.floats(a.TEXCOORD_0) : new Float32Array(n * 2)
      const thickness = new Float32Array(n)
      if (a.COLOR_0 !== undefined) {
        const c = glb.floats(a.COLOR_0)
        const width = c.length / Math.max(n, 1)
        for (let i = 0; i < n; i++) thickness[i] = c[i * width + 1]
      } else thickness.fill(1)
      const skinned = a.JOINTS_0 !== undefined && a.WEIGHTS_0 !== undefined
      let indices: Uint32Array
      if (p.indices !== undefined) indices = glb.ints(p.indices)
      else {
        indices = new Uint32Array(n)
        for (let i = 0; i < n; i++) indices[i] = i
      }
      primitives.push({
        positions,
        normals,
        uv,
        thickness,
        joints: skinned ? glb.ints(a.JOINTS_0) : null,
        weights: skinned ? glb.floats(a.WEIGHTS_0) : null,
        indices,
        material: p.material ?? -1,
      })
    }

  const part: DonPartData = { primitives, materials, images, jointNames, inverseBind, faceMaterial }
  if (faceMaterial >= 0) fitFacePlane(part, normalizeFace)
  return part
}

// Shrinks the face plate to the standard size around its centre (always when normalize, else only
// when it is larger than that).
export function fitFacePlane(part: DonPartData, normalize: boolean): void {
  for (const prim of part.primitives) {
    const pos = prim.positions
    if (prim.material !== part.faceMaterial || pos.length === 0) continue
    const min = [pos[0], pos[1], pos[2]]
    const max = [pos[0], pos[1], pos[2]]
    for (let i = 0; i < pos.length; i += 3)
      for (let c = 0; c < 3; c++) {
        min[c] = Math.min(min[c], pos[i + c])
        max[c] = Math.max(max[c], pos[i + c])
      }
    const size = Math.max(max[0] - min[0], max[1] - min[1])
    if (size <= 0.0001 || (!normalize && size <= facePlaneSize * 1.02)) continue
    const factor = facePlaneSize / size
    for (let i = 0; i < pos.length; i += 3)
      for (let c = 0; c < 3; c++) {
        const centre = (min[c] + max[c]) * 0.5
        pos[i + c] = centre + (pos[i + c] - centre) * factor
      }
  }
}
