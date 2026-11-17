import {
  BackSide,
  BufferAttribute,
  BufferGeometry,
  Camera,
  ClampToEdgeWrapping,
  CustomBlending,
  DataTexture,
  DoubleSide,
  DynamicDrawUsage,
  FrontSide,
  LinearFilter,
  LinearMipmapLinearFilter,
  Matrix4,
  Mesh,
  NoColorSpace,
  OneFactor,
  OneMinusSrcAlphaFactor,
  PlaneGeometry,
  RepeatWrapping,
  RGBAFormat,
  Scene,
  SRGBColorSpace,
  ShaderMaterial,
  SrcAlphaFactor,
  Texture,
  UnsignedByteType,
  Vector2,
  Vector4,
  WebGLRenderer,
  WebGLRenderTarget,
} from 'three'
import {
  loadBase,
  loadPaint,
  loadPart,
  loadPuchi,
  partImage,
  partPixels,
  type DonBase,
} from './assets'
import { catalogUrl, resolveDonLook, type DonCatalog, type DonLook } from './catalog'
import {
  DonAnim,
  DonAnimator,
  modelMatrix,
  parseHexColor,
  recolorPixels,
  scale720,
  type DonMotionName,
} from './chara'
import { drawStage, type DonPartData } from './part'
import {
  compositeFragment,
  modelFragment,
  modelVertex,
  outlineFragment,
  outlineVertex,
  puchiFragment,
  puchiVertex,
  screenVertex,
} from './shaders'

// The area a Don is laid out in, in YataiDON's 1080-line units (its 720-line skin times 1.5): it is
// fitted into the canvas (centred), and the Don's feet are at (pivotX, pivotY) of it, measured from
// its bottom left. The defaults are the game's DonPreview pictures: 480 × 560 with the feet at
// (0.5, 0.15), room for the tallest costumes and a puchichara.
export type DonFrame = {
  width: number
  height: number
  pivotX: number
  pivotY: number
  drawScale: number
}
export const defaultDonFrame: DonFrame = {
  width: 480,
  height: 560,
  pivotX: 0.5,
  pivotY: 0.15,
  drawScale: 1,
}

export type DonRendererState = { loading: boolean; error: Error | null }

export type DonRendererOptions = {
  catalog: DonCatalog
  baseUrl: string
  bpm?: number
  frame?: Partial<DonFrame>
  // Keeps the canvas's picture readable after it is shown (toBlob / toDataURL), at some cost.
  preserveDrawingBuffer?: boolean
  onStateChange?: (state: DonRendererState) => void
}

export type DonRenderer = {
  // Dresses the Don; the previous look stays until the new one has loaded. Resolves when it shows,
  // a later look replaced it, or it failed (state.error).
  setLook(look: DonLook): Promise<void>
  // Starts a motion; a one-shot returns to the loop it interrupted.
  setMotion(name: DonMotionName): void
  setMirrored(mirrored: boolean): void
  setBpm(bpm: number): void
  // Holds the motion at a point of its loop (0..1), or lets it run again (null).
  freeze(progress: number | null): void
  // The canvas's size in CSS pixels and the device pixel ratio.
  resize(width: number, height: number, pixelRatio: number): void
  readonly state: DonRendererState
  // Frees everything, the canvas's WebGL context too: a new renderer needs a new canvas.
  dispose(): void
}

const layoutScale = 1.5 // 1080 lines over YataiDON's 720
const supersample = 2
const msaa = 4
const puchiFrameMs = 850
// YataiDON PAINT_PLANE_SCALE: makeup is drawn on a face plate a third larger than the plain face's.
export const paintPlaneScale = 4 / 3

// One part's GPU objects in this renderer: geometry per primitive (skinned on the CPU) and the
// textures of its pictures, recoloured ones by colours.
type PartGpu = {
  data: DonPartData
  geometries: BufferGeometry[]
  textures: (Texture | null)[]
  recolored: Map<string, (Texture | null)[]>
  jointMap: Int32Array
  skin: Float32Array
  // the bind positions skinning starts from: the part's own, but for the face plates while the
  // Don wears makeup (scaled by faceScale about each plate's centre)
  sources: Float32Array[]
  faceScale: number
  lastUsed: number
}

// A dressed Don: its parts, the scene that draws them in YataiDON's order, the face materials.
type Outfit = {
  parts: PartGpu[]
  scene: Scene
  materials: ShaderMaterial[]
  faceRects: Vector4[]
  // faces in the sheet the face plates show (the base one's or the makeup's)
  faceCount: number
  puchi: Texture | null
  key: string
}

export function createDonRenderer(
  canvas: HTMLCanvasElement,
  options: DonRendererOptions,
): DonRenderer {
  const { catalog, baseUrl } = options
  const frame: DonFrame = { ...defaultDonFrame, ...options.frame }
  const renderer = new WebGLRenderer({
    canvas,
    alpha: true,
    premultipliedAlpha: true,
    antialias: false,
    preserveDrawingBuffer: options.preserveDrawingBuffer ?? false,
  })
  renderer.setPixelRatio(1)
  renderer.autoClear = false
  renderer.sortObjects = false
  renderer.setClearColor(0x000000, 0)

  // Unity's "Forced On" anisotropic filtering (the Ultra quality level): at least 9
  const anisotropy = Math.min(9, renderer.capabilities.getMaxAnisotropy())

  const white = new DataTexture(new Uint8Array([255, 255, 255, 255]), 1, 1)
  white.needsUpdate = true

  // shared by every outline material
  const outlineUniform = { value: new Vector2(0, 0.3) }
  const screenSizeUniform = { value: new Vector2(1, 1) }

  const camera = new Camera()
  camera.matrixAutoUpdate = false
  const screenCamera = new Camera()
  const quad = new PlaneGeometry(2, 2)
  const compositeMaterial = new ShaderMaterial({
    uniforms: { map: { value: null }, texel: { value: new Vector2() }, thickness: { value: 3 } },
    vertexShader: screenVertex,
    fragmentShader: compositeFragment,
    depthTest: false,
    depthWrite: false,
    ...premultipliedBlending(),
  })
  const compositeScene = new Scene().add(noCull(new Mesh(quad, compositeMaterial)))
  const puchiMaterial = new ShaderMaterial({
    uniforms: {
      map: { value: null },
      rect: { value: new Vector4() },
      frame: { value: 0 },
      mirrored: { value: 0 },
    },
    vertexShader: puchiVertex,
    fragmentShader: puchiFragment,
    depthTest: false,
    depthWrite: false,
    ...premultipliedBlending(),
  })
  const puchiScene = new Scene().add(noCull(new Mesh(quad, puchiMaterial)))

  let target: WebGLRenderTarget | null = null
  let cssWidth = 0
  let cssHeight = 0
  let pixelRatio = 1
  let width = 0 // canvas pixels
  let height = 0

  let base: DonBase | null = null
  let faceTexture: Texture | null = null
  let animator: DonAnimator | null = null
  let outfit: Outfit | null = null
  const gpuParts = new Map<DonPartData, PartGpu>()
  const puchiTextures = new Map<string, Texture>()
  const paintTextures = new Map<string, { texture: Texture; count: number }>()
  let clock = 0

  let bpm = options.bpm ?? 120
  let mirrored = false
  let motion: DonMotionName = 'Normal'
  let frozenAt: number | null = null

  let dirty = true
  let posed = { anim: -1, frame: -1, outfit: null as Outfit | null }
  let shownFace = -1
  let raf = 0
  let disposed = false
  let lookToken = 0
  let pending = 0
  let failure: Error | null = null // the latest look's
  let state: DonRendererState = { loading: false, error: null }
  const setState = (next: DonRendererState) => {
    if (next.loading === state.loading && next.error === state.error) return
    state = next
    options.onStateChange?.(state)
  }

  const now = () => performance.now()

  // ---- loading ----

  async function ensureBase(): Promise<DonBase> {
    if (base) return base
    const loaded = await loadBase(catalog, baseUrl)
    if (disposed || base) return base ?? loaded
    base = loaded
    faceTexture = faceSheetTexture(loaded.faceSheet)
    const t = now()
    animator = new DonAnimator(loaded.motions, loaded.faceTimelines, loaded.faceCount, t)
    animator.bpm = bpm
    animator.setMirror(mirrored, t)
    animator.setAnim(DonAnim[motion], t)
    applyFrozen()
    dirty = true
    return loaded
  }

  function partGpu(
    data: DonPartData,
    bitmaps: (ImageBitmap | null)[],
    motions: DonBase['motions'],
  ): PartGpu {
    let gpu = gpuParts.get(data)
    if (!gpu) {
      const geometries = data.primitives.map((prim) => {
        const geometry = new BufferGeometry()
        geometry.setAttribute(
          'position',
          new BufferAttribute(new Float32Array(prim.positions), 3).setUsage(DynamicDrawUsage),
        )
        geometry.setAttribute(
          'normal',
          new BufferAttribute(new Float32Array(prim.normals), 3).setUsage(DynamicDrawUsage),
        )
        geometry.setAttribute('uv', new BufferAttribute(prim.uv, 2))
        geometry.setAttribute('thickness', new BufferAttribute(prim.thickness, 1))
        geometry.setIndex(new BufferAttribute(prim.indices, 1))
        return geometry
      })
      const textures = bitmaps.map((bitmap) => (bitmap ? imageTexture(bitmap, anisotropy) : null))
      const joints = data.jointNames.length
      const jointMap = new Int32Array(joints)
      data.jointNames.forEach((name, j) => (jointMap[j] = motions.joints.get(name) ?? -1))
      gpu = {
        data,
        geometries,
        textures,
        recolored: new Map(),
        jointMap,
        skin: new Float32Array(joints * 16),
        sources: data.primitives.map((prim) => prim.positions),
        faceScale: 1,
        lastUsed: 0,
      }
      gpuParts.set(data, gpu)
    }
    gpu.lastUsed = ++clock
    return gpu
  }

  // The part's pictures with the recolourable ones recoloured (YataiDON recolor_texture).
  async function recolored(
    gpu: PartGpu,
    colors: [string, string, string],
  ): Promise<(Texture | null)[]> {
    const key = colors.join(',').toUpperCase()
    const cached = gpu.recolored.get(key)
    if (cached) {
      // the most recently used last
      gpu.recolored.delete(key)
      gpu.recolored.set(key, cached)
      return cached
    }
    const [body, face, rim] = colors.map(parseHexColor)
    const result = gpu.textures.slice()
    const images = new Set(
      gpu.data.materials.filter((m) => m.recolor && m.image >= 0).map((m) => m.image),
    )
    for (const index of images) {
      const image = gpu.data.images[index]
      if (!image) continue
      const source = await partPixels(image)
      const bytes = new Uint8Array(source.data)
      recolorPixels(bytes, body, face, rim)
      const texture = new DataTexture(
        bytes,
        source.width,
        source.height,
        RGBAFormat,
        UnsignedByteType,
      )
      setPictureSampling(texture, anisotropy)
      texture.needsUpdate = true
      result[index] = texture
    }
    // two colour sets a part (a page switching between 1P's and 2P's)
    if (gpu.recolored.size >= 2) {
      const [oldest, textures] = gpu.recolored.entries().next().value as [
        string,
        (Texture | null)[],
      ]
      textures.forEach((t, i) => t !== gpu.textures[i] && t?.dispose())
      gpu.recolored.delete(oldest)
    }
    gpu.recolored.set(key, result)
    return result
  }

  async function setLook(look: DonLook): Promise<void> {
    const token = ++lookToken
    pending++
    failure = null
    setState({ loading: true, error: null })
    try {
      const resolved = resolveDonLook(catalog, look)
      const [loadedBase, datas, puchiBitmap, paintBitmap] = await Promise.all([
        ensureBase(),
        Promise.all(
          resolved.parts.map((p) =>
            loadPart(catalogUrl(baseUrl, p.item.file, catalog.mirrorUrls), p.normalizeFace),
          ),
        ),
        resolved.puchi
          ? loadPuchi(catalogUrl(baseUrl, resolved.puchi.file, catalog.mirrorUrls))
          : Promise.resolve(null),
        resolved.paint
          ? loadPaint(catalogUrl(baseUrl, resolved.paint.file, catalog.mirrorUrls))
          : Promise.resolve(null),
      ])
      const bitmaps = await Promise.all(
        datas.map((data) =>
          Promise.all(data.images.map((image) => (image ? partImage(image) : null))),
        ),
      )
      if (disposed || token !== lookToken) return
      const gpus = datas.map((data, i) => partGpu(data, bitmaps[i], loadedBase.motions))
      const textures = await Promise.all(gpus.map((gpu) => recolored(gpu, resolved.colors)))
      if (disposed || token !== lookToken) return
      let puchi: Texture | null = null
      if (resolved.puchi && puchiBitmap) {
        const url = catalogUrl(baseUrl, resolved.puchi.file, catalog.mirrorUrls)
        puchi = puchiTextures.get(url) ?? null
        if (!puchi) {
          // drawn straight onto the canvas: no colour space
          puchi = imageTexture(puchiBitmap, 1)
          puchi.colorSpace = NoColorSpace
          puchi.wrapS = puchi.wrapT = ClampToEdgeWrapping
          puchiTextures.set(url, puchi)
        }
      }
      // the makeup's sheet in place of the plain face (YataiDON Chara3D::set_paint)
      let face = { texture: faceTexture, count: loadedBase.faceCount }
      if (resolved.paint && paintBitmap) {
        const url = catalogUrl(baseUrl, resolved.paint.file, catalog.mirrorUrls)
        let sheet = paintTextures.get(url)
        if (!sheet) {
          sheet = {
            texture: faceSheetTexture(paintBitmap),
            count: paintBitmap.width > 0 ? Math.floor(paintBitmap.height / paintBitmap.width) : 0,
          }
          paintTextures.set(url, sheet)
        }
        face = sheet
      }
      const faceScale = resolved.paint ? paintPlaneScale : 1
      for (const gpu of gpus) scaleFacePlates(gpu, faceScale)
      const key =
        JSON.stringify(resolved.parts.map((p) => [p.kind, p.item.id])) +
        resolved.colors +
        (resolved.paint?.id ?? 0)
      dress(gpus, textures, puchi, face, key)
      trimParts()
    } catch (error) {
      if (token === lookToken) failure = error instanceof Error ? error : new Error(String(error))
    } finally {
      pending--
      if (!disposed) setState({ loading: pending > 0, error: failure })
    }
  }

  function dress(
    parts: PartGpu[],
    textures: (Texture | null)[][],
    puchi: Texture | null,
    face: { texture: Texture | null; count: number },
    key: string,
  ) {
    const old = outfit
    const scene = new Scene()
    const materials: ShaderMaterial[] = []
    const faceRects: Vector4[] = []
    const meshes: Mesh[] = []
    const outlines: Mesh[] = []
    parts.forEach((part, p) => {
      const data = part.data
      const main = data.materials.map((info, m) => {
        const uvRect = new Vector4(1, 1, 0, 0)
        if (info.face) faceRects.push(uvRect)
        const map = info.face ? face.texture : info.image >= 0 ? textures[p][info.image] : null
        const material = new ShaderMaterial({
          uniforms: {
            map: { value: map ?? white },
            // a Unity colour property in a linear project: gamma-space numbers made linear
            color: {
              value: new Vector4(
                srgbToLinear(info.color[0]),
                srgbToLinear(info.color[1]),
                srgbToLinear(info.color[2]),
                info.color[3],
              ),
            },
            uvRect: { value: uvRect },
            alphaClip: { value: info.face ? 0 : 1 },
          },
          vertexShader: modelVertex,
          fragmentShader: modelFragment,
          side: info.twoSided ? DoubleSide : FrontSide,
          ...modelBlending(),
        })
        material.name = info.name || 'material ' + m
        materials.push(material)
        return material
      })
      const outline = data.materials.map((info, m) => {
        const material = new ShaderMaterial({
          uniforms: {
            map: { value: main[m].uniforms.map.value },
            uvRect: main[m].uniforms.uvRect,
            outline: outlineUniform,
            screenSize: screenSizeUniform,
          },
          vertexShader: outlineVertex,
          fragmentShader: outlineFragment,
          side: BackSide,
          ...modelBlending(),
        })
        material.name = (info.name || 'material ' + m) + ' outline'
        materials.push(material)
        return material
      })
      // 1. opaque and alpha-tested meshes 2. the face plane 3. alpha-blended sheets
      for (let stage = 0; stage < 3; stage++)
        data.primitives.forEach((prim, i) => {
          const info = data.materials[prim.material]
          if (info && drawStage(info) === stage)
            meshes.push(fixed(new Mesh(part.geometries[i], main[prim.material])))
        })
      // the black line: back faces pushed out along their normals, under the model
      data.primitives.forEach((prim, i) => {
        const info = data.materials[prim.material]
        if (info && !info.face)
          outlines.push(fixed(new Mesh(part.geometries[i], outline[prim.material])))
      })
    })
    // each part's meshes, then every part's outline hull
    scene.add(...meshes, ...outlines)
    outfit = { parts, scene, materials, faceRects, faceCount: face.count, puchi, key }
    posed = { anim: -1, frame: -1, outfit: null }
    shownFace = -1
    dirty = true
    if (old) old.materials.forEach((m) => m.dispose())
  }

  // Frees the GPU objects of parts nobody wears beyond the last eight.
  function trimParts() {
    const wearing = new Set(outfit?.parts)
    const idle = [...gpuParts.values()]
      .filter((g) => !wearing.has(g))
      .sort((a, b) => a.lastUsed - b.lastUsed)
    for (const gpu of idle.slice(0, Math.max(0, idle.length - 8))) {
      disposePart(gpu)
      gpuParts.delete(gpu.data)
    }
  }

  function disposePart(gpu: PartGpu) {
    gpu.geometries.forEach((g) => g.dispose())
    gpu.textures.forEach((t) => t?.dispose())
    for (const textures of gpu.recolored.values())
      textures.forEach((t, i) => t !== gpu.textures[i] && t?.dispose())
  }

  // ---- motion ----

  function applyFrozen() {
    if (!animator) return
    if (frozenAt === null) animator.setFrozen(false)
    else {
      animator.setFrozen(true)
      animator.setAnimProgress(frozenAt)
    }
  }

  // CPU skinning: each joint's pose times the part's inverse bind matrix, blended by weight.
  function pose(motions: DonBase['motions'], anim: number, frameIndex: number) {
    if (!outfit) return
    const poses = motions.posesOf(anim)
    const joints = motions.jointCount
    const m = new Matrix4()
    const ib = new Matrix4()
    for (const part of outfit.parts) {
      const data = part.data
      for (let j = 0; j < part.jointMap.length; j++) {
        const k = part.jointMap[j]
        // bones the motions do not have stay at the part's bind pose
        if (k < 0) m.identity()
        else {
          m.fromArray(poses, (frameIndex * joints + k) * 16)
          m.multiply(ib.fromArray(data.inverseBind, j * 16))
        }
        part.skin.set(m.elements, j * 16)
      }
      skinPart(part)
    }
  }

  // ---- drawing ----

  function layout() {
    const ppu = Math.min(width / frame.width, height / frame.height) // canvas pixels a layout unit
    const ox = (width - frame.width * ppu) / 2
    const oy = (height - frame.height * ppu) / 2
    return {
      ppu,
      x: ox + frame.pivotX * frame.width * ppu,
      y: oy + (1 - frame.pivotY) * frame.height * ppu,
    }
  }

  function ensureTarget(): number {
    const max = Math.min(4096, renderer.capabilities.maxTextureSize)
    const ss = Math.min(supersample, max / Math.max(width, height))
    const w = Math.max(16, Math.round(width * ss))
    const h = Math.max(16, Math.round(height * ss))
    if (!target || target.width !== w || target.height !== h) {
      target?.dispose()
      target = new WebGLRenderTarget(w, h, {
        samples: Math.min(msaa, renderer.capabilities.maxSamples),
        depthBuffer: true,
        generateMipmaps: false,
        minFilter: LinearFilter,
        magFilter: LinearFilter,
        // as the game's linear-space target: sRGB storage, blending in linear
        colorSpace: SRGBColorSpace,
      })
      dirty = true
    }
    return w / width
  }

  function render3d(ss: number) {
    if (!target || !outfit) return
    const { ppu, x, y } = layout()
    const tw = target.width
    const th = target.height
    // Drawn in layout units, as the game does (its depth range is in those units too), over the
    // whole target.
    const ppuTarget = ppu * ss
    const cx = tw / ppuTarget / 2
    const cy = th / ppuTarget / 2
    // YataiDON's camera: orthographic, from z = -100 looking at +z, up -y (a y-down 2D area).
    const view = new Matrix4().set(1, 0, 0, -cx, 0, -1, 0, cy, 0, 0, -1, -100, 0, 0, 0, 1)
    camera.matrix.copy(view).invert()
    camera.matrixWorldNeedsUpdate = true
    const near = 0.01
    const far = 1000
    // prettier-ignore
    camera.projectionMatrix.set(
      1 / cx, 0, 0, 0,
      0, 1 / cy, 0, 0,
      0, 0, -2 / (far - near), -(far + near) / (far - near),
      0, 0, 0, 1,
    )
    camera.projectionMatrixInverse.copy(camera.projectionMatrix).invert()
    const size = scale720 * layoutScale * frame.drawScale
    const model = modelMatrix(x / ppu, y / ppu, size, mirrored)
    for (const child of outfit.scene.children) {
      child.matrix.copy(model)
      child.matrixWorldNeedsUpdate = true
    }
    outlineUniform.value.x = (2.5 * frame.drawScale * (1080 * ppuTarget)) / 720
    screenSizeUniform.value.set(tw, th)
    renderer.setRenderTarget(target)
    renderer.clear(true, true, true)
    renderer.render(outfit.scene, camera)
    renderer.setRenderTarget(null)
  }

  function composite(ss: number, t: number) {
    if (!target) return
    const { ppu, x, y } = layout()
    renderer.setRenderTarget(null)
    renderer.clear(true, true, true)
    if (outfit) {
      compositeMaterial.uniforms.map.value = target.texture
      compositeMaterial.uniforms.texel.value.set(1 / target.width, 1 / target.height)
      // YataiDON's 3 texels of a target at twice the screen: 1.5 screen pixels at 1080 lines
      compositeMaterial.uniforms.thickness.value = 1.5 * frame.drawScale * ppu * ss
      renderer.render(compositeScene, screenCamera)
    }
    const puchi = outfit?.puchi
    if (puchi) {
      // YataiDON Chara3D::draw_puchi: 150 px square, 90 px to the Don's back side and 120 up, two
      // frames 850 ms each, bobbing 6 px over two frames.
      const s = layoutScale * frame.drawScale * ppu
      const frameIndex = Math.floor(t / puchiFrameMs) % 2
      const bob = Math.sin((t / (puchiFrameMs * 2)) * 2 * Math.PI) * 6 * s
      const half = (150 * s) / 2
      const px = x + (mirrored ? 90 : -90) * s
      const py = y - (120 * s - bob)
      const toClipX = (v: number) => (v / width) * 2 - 1
      const toClipY = (v: number) => 1 - (v / height) * 2
      puchiMaterial.uniforms.map.value = puchi
      puchiMaterial.uniforms.rect.value.set(
        toClipX(px - half),
        toClipY(py + half),
        toClipX(px + half),
        toClipY(py - half),
      )
      puchiMaterial.uniforms.frame.value = frameIndex
      puchiMaterial.uniforms.mirrored.value = mirrored ? 1 : 0
      renderer.render(puchiScene, screenCamera)
    }
  }

  function tick() {
    raf = requestAnimationFrame(tick)
    if (width <= 0 || height <= 0) return
    const t = now()
    if (animator && base) {
      animator.update(t, t)
      const anim = animator.animIndex
      const frameIndex = animator.poseFrame
      if (
        outfit &&
        (posed.anim !== anim || posed.frame !== frameIndex || posed.outfit !== outfit)
      ) {
        pose(base.motions, anim, frameIndex)
        posed = { anim, frame: frameIndex, outfit }
        dirty = true
      }
      if (outfit && animator.face !== shownFace && outfit.faceCount > 0) {
        // the face sheet's cell, faces stacked top to bottom
        shownFace = animator.face
        const count = outfit.faceCount
        for (const rect of outfit.faceRects) rect.set(1, 1 / count, 0, shownFace / count)
        dirty = true
      }
    }
    const ss = ensureTarget()
    if (dirty) render3d(ss)
    if (dirty || outfit?.puchi) composite(ss, t)
    dirty = false
  }

  raf = requestAnimationFrame(tick)

  return {
    setLook,
    setMotion(name) {
      motion = name
      if (animator) {
        animator.setAnim(DonAnim[name], now())
        applyFrozen()
      }
    },
    setMirrored(value) {
      if (mirrored === value) return
      mirrored = value
      animator?.setMirror(value, now())
      applyFrozen()
      dirty = true
    },
    setBpm(value) {
      bpm = value
      if (animator) animator.bpm = value
    },
    freeze(progress) {
      frozenAt = progress
      applyFrozen()
    },
    resize(w, h, ratio) {
      if (w === cssWidth && h === cssHeight && ratio === pixelRatio) return
      cssWidth = w
      cssHeight = h
      pixelRatio = ratio
      width = Math.max(0, Math.round(w * ratio))
      height = Math.max(0, Math.round(h * ratio))
      if (width > 0 && height > 0) renderer.setSize(width, height, false)
      dirty = true
    },
    get state() {
      return state
    },
    dispose() {
      if (disposed) return
      disposed = true
      cancelAnimationFrame(raf)
      outfit?.materials.forEach((m) => m.dispose())
      outfit = null
      for (const gpu of gpuParts.values()) disposePart(gpu)
      gpuParts.clear()
      puchiTextures.forEach((t) => t.dispose())
      paintTextures.forEach((s) => s.texture.dispose())
      faceTexture?.dispose()
      white.dispose()
      target?.dispose()
      quad.dispose()
      compositeMaterial.dispose()
      puchiMaterial.dispose()
      renderer.dispose()
      // frees the context now rather than at garbage collection (browsers keep only a few)
      renderer.forceContextLoss()
    },
  }
}

function srgbToLinear(c: number): number {
  return c <= 0.04045 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4)
}

function fixed(mesh: Mesh): Mesh {
  mesh.matrixAutoUpdate = false
  mesh.frustumCulled = false
  return mesh
}

function noCull(mesh: Mesh): Mesh {
  mesh.frustumCulled = false
  return mesh
}

// The model's blend (YataiDON's CUSTOM_SEPARATE): colour SrcAlpha / 1-SrcAlpha, alpha One /
// 1-SrcAlpha, so the target holds colour multiplied by alpha.
function modelBlending() {
  return {
    blending: CustomBlending,
    blendSrc: SrcAlphaFactor,
    blendDst: OneMinusSrcAlphaFactor,
    blendSrcAlpha: OneFactor,
    blendDstAlpha: OneMinusSrcAlphaFactor,
    depthTest: true,
    depthWrite: true,
  } as const
}

function premultipliedBlending() {
  return {
    blending: CustomBlending,
    blendSrc: OneFactor,
    blendDst: OneMinusSrcAlphaFactor,
    blendSrcAlpha: OneFactor,
    blendDstAlpha: OneMinusSrcAlphaFactor,
  } as const
}

// glb pictures: as uploaded (glTF's top-left UV origin, straight alpha), sRGB (the game draws in
// linear space: sampled, filtered and mipmapped linear), mipmapped and trilinear (a costume's 1024
// px sheets are drawn a few hundred pixels tall), anisotropic as the game's quality level forces.
function setPictureSampling(texture: Texture, anisotropy: number) {
  texture.flipY = false
  texture.premultiplyAlpha = false
  texture.colorSpace = SRGBColorSpace
  texture.anisotropy = anisotropy
  texture.generateMipmaps = true
  texture.minFilter = LinearMipmapLinearFilter
  texture.magFilter = LinearFilter
  texture.wrapS = texture.wrapT = RepeatWrapping
}

// A face sheet as the face plates sample it: no mipmaps, clamped, one face a cell.
function faceSheetTexture(bitmap: ImageBitmap): Texture {
  const texture = new Texture(bitmap)
  texture.flipY = false
  texture.premultiplyAlpha = false
  texture.colorSpace = SRGBColorSpace
  texture.generateMipmaps = false
  texture.minFilter = LinearFilter
  texture.magFilter = LinearFilter
  texture.wrapS = texture.wrapT = ClampToEdgeWrapping
  texture.needsUpdate = true
  return texture
}

// Brings the part's face plates to `factor` times their own size, each about the centre of its box
// (YataiDON Chara3D::scale_face_planes); skinning then starts from those positions. A plate no
// joint moves is written into its geometry here.
export function scaleFacePlates(
  part: Pick<PartGpu, 'data' | 'sources' | 'faceScale'> & { geometries?: BufferGeometry[] },
  factor: number,
) {
  if (part.faceScale === factor) return
  part.faceScale = factor
  part.data.primitives.forEach((prim, p) => {
    if (part.data.materials[prim.material]?.face !== true) return
    const src = prim.positions
    let scaled = src
    if (factor !== 1 && src.length >= 3) {
      const min = [Infinity, Infinity, Infinity]
      const max = [-Infinity, -Infinity, -Infinity]
      for (let i = 0; i < src.length; i++) {
        min[i % 3] = Math.min(min[i % 3], src[i])
        max[i % 3] = Math.max(max[i % 3], src[i])
      }
      const centre = min.map((m, k) => (m + max[k]) / 2)
      scaled = new Float32Array(src.length)
      for (let i = 0; i < src.length; i++)
        scaled[i] = centre[i % 3] + (src[i] - centre[i % 3]) * factor
    }
    part.sources[p] = scaled
    const geometry = part.geometries?.[p]
    if (geometry && !(prim.joints && prim.weights && part.data.jointNames.length > 0)) {
      const attr = geometry.getAttribute('position') as BufferAttribute
      ;(attr.array as Float32Array).set(scaled)
      attr.needsUpdate = true
    }
  })
}

function imageTexture(bitmap: ImageBitmap, anisotropy: number): Texture {
  const texture = new Texture(bitmap)
  setPictureSampling(texture, anisotropy)
  texture.needsUpdate = true
  return texture
}

// The part's primitives in the pose of its skin matrices (the game's Chara3D.SkinPart): each vertex
// by the weighted sum of its joints' matrices; a vertex without weight keeps its bind position.
function skinPart(part: PartGpu) {
  const skin = part.skin
  const joints = part.jointMap.length
  part.data.primitives.forEach((prim, p) => {
    if (!prim.joints || !prim.weights || joints === 0) return
    const geometry = part.geometries[p]
    const posAttr = geometry.getAttribute('position') as BufferAttribute
    const nrmAttr = geometry.getAttribute('normal') as BufferAttribute
    const pos = posAttr.array as Float32Array
    const nrm = nrmAttr.array as Float32Array
    const src = part.sources[p]
    const srcN = prim.normals
    const n = src.length / 3
    const m = new Float64Array(12)
    for (let v = 0; v < n; v++) {
      m.fill(0)
      let total = 0
      for (let w = 0; w < 4; w++) {
        const weight = prim.weights[v * 4 + w]
        if (weight <= 0) continue
        const j = prim.joints[v * 4 + w]
        if (j >= joints) continue
        const o = j * 16
        // columns 0..2 (rotation/scale) and 3 (translation), rows 0..2
        m[0] += skin[o] * weight
        m[1] += skin[o + 1] * weight
        m[2] += skin[o + 2] * weight
        m[3] += skin[o + 4] * weight
        m[4] += skin[o + 5] * weight
        m[5] += skin[o + 6] * weight
        m[6] += skin[o + 8] * weight
        m[7] += skin[o + 9] * weight
        m[8] += skin[o + 10] * weight
        m[9] += skin[o + 12] * weight
        m[10] += skin[o + 13] * weight
        m[11] += skin[o + 14] * weight
        total += weight
      }
      const i = v * 3
      const x = src[i]
      const y = src[i + 1]
      const z = src[i + 2]
      const nx = srcN[i]
      const ny = srcN[i + 1]
      const nz = srcN[i + 2]
      if (total <= 0) {
        pos[i] = x
        pos[i + 1] = y
        pos[i + 2] = z
        nrm[i] = nx
        nrm[i + 1] = ny
        nrm[i + 2] = nz
        continue
      }
      pos[i] = m[0] * x + m[3] * y + m[6] * z + m[9]
      pos[i + 1] = m[1] * x + m[4] * y + m[7] * z + m[10]
      pos[i + 2] = m[2] * x + m[5] * y + m[8] * z + m[11]
      nrm[i] = m[0] * nx + m[3] * ny + m[6] * nz
      nrm[i + 1] = m[1] * nx + m[4] * ny + m[7] * nz
      nrm[i + 2] = m[2] * nx + m[5] * ny + m[8] * nz
    }
    posAttr.needsUpdate = true
    nrmAttr.needsUpdate = true
  })
}
