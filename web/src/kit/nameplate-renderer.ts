import { useEffect, type RefObject } from 'react'
import { otanimMesh, type OtanimClip } from './don3d/otanim'
import type { LayerItem } from './nameplate-layers'

// Every layered nameplate on a page is drawn by one WebGL context (a page may hold many plates,
// and a browser allows only a few contexts) into its own 2D canvas, on one animation clock: a
// plate out of sight is not drawn, and a still one is drawn once. The game draws the same files
// the same way (OtanimView): premultiplied, additive parts added, back to front.

/** One picture of a plate, back to front. */
export type DrawLayer = LayerItem & {
  clip: OtanimClip
  // Writes its shape (where its picture is opaque) for the layers after it to be cut to, and
  // draws nothing: the band's shape, which an effect that stays inside the band is cut to.
  mask?: boolean
  // Cut to the shape the last mask wrote.
  cut?: boolean
}

/** What a canvas shows of the plate: [x0, y0, x1, y1] in plate pixels from its top left. */
export type PlateFrame = [number, number, number, number]

const vertexSource = `#version 300 es
in vec2 position;
in vec2 uv;
in vec4 tint;
uniform vec2 size;
out vec2 vUv;
out vec4 vTint;
void main() {
  vUv = uv;
  vTint = tint;
  gl_Position = vec4(position / size * 2.0 - 1.0, 0.0, 1.0);
}`

// As the Don's puchichara (don3d/shaders.ts puchiAnimFragment): an additive part has 2 added to
// its u and is drawn with no alpha, so the premultiplied blend adds it.
const fragmentSource = `#version 300 es
precision mediump float;
uniform sampler2D map;
uniform float premultiplied;
uniform float mask;
in vec2 vUv;
in vec4 vTint;
out vec4 color;
void main() {
  float additive = step(1.5, vUv.x);
  vec4 t = texture(map, vec2(vUv.x - additive * 2.0, vUv.y));
  if (premultiplied < 0.5) t.rgb *= t.a;
  if (mask > 0.5 && t.a * vTint.a < 0.5) discard;
  color = vec4(t.rgb * vTint.rgb, t.a * vTint.a * (1.0 - additive));
}`

class PlateRenderer {
  readonly canvas = document.createElement('canvas')
  private readonly gl: WebGL2RenderingContext
  private readonly program: WebGLProgram
  private readonly at: { position: number; uv: number; tint: number }
  private readonly uniforms: Record<'size' | 'premultiplied' | 'mask', WebGLUniformLocation | null>
  private readonly buffers: {
    position: WebGLBuffer
    uv: WebGLBuffer
    tint: WebGLBuffer
    index: WebGLBuffer
  }
  private readonly textures = new WeakMap<ImageBitmap, WebGLTexture>()

  constructor() {
    const gl = this.canvas.getContext('webgl2', {
      stencil: true,
      premultipliedAlpha: true,
      antialias: true,
    })
    if (!gl) throw new Error('WEBGL_UNAVAILABLE')
    this.gl = gl
    const shader = (type: number, source: string) => {
      const s = gl.createShader(type)!
      gl.shaderSource(s, source)
      gl.compileShader(s)
      if (!gl.getShaderParameter(s, gl.COMPILE_STATUS))
        throw new Error(gl.getShaderInfoLog(s) ?? 'shader')
      return s
    }
    const program = gl.createProgram()
    gl.attachShader(program, shader(gl.VERTEX_SHADER, vertexSource))
    gl.attachShader(program, shader(gl.FRAGMENT_SHADER, fragmentSource))
    gl.linkProgram(program)
    if (!gl.getProgramParameter(program, gl.LINK_STATUS))
      throw new Error(gl.getProgramInfoLog(program) ?? 'program')
    this.program = program
    this.at = {
      position: gl.getAttribLocation(program, 'position'),
      uv: gl.getAttribLocation(program, 'uv'),
      tint: gl.getAttribLocation(program, 'tint'),
    }
    this.uniforms = {
      size: gl.getUniformLocation(program, 'size'),
      premultiplied: gl.getUniformLocation(program, 'premultiplied'),
      mask: gl.getUniformLocation(program, 'mask'),
    }
    this.buffers = {
      position: gl.createBuffer(),
      uv: gl.createBuffer(),
      tint: gl.createBuffer(),
      index: gl.createBuffer(),
    }
  }

  private texture(picture: ImageBitmap): WebGLTexture {
    let texture = this.textures.get(picture)
    if (texture) return texture
    const gl = this.gl
    texture = gl.createTexture()
    gl.bindTexture(gl.TEXTURE_2D, texture)
    gl.pixelStorei(gl.UNPACK_PREMULTIPLY_ALPHA_WEBGL, false)
    gl.pixelStorei(gl.UNPACK_FLIP_Y_WEBGL, false)
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, picture)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
    this.textures.set(picture, texture)
    return texture
  }

  private attribute(location: number, buffer: WebGLBuffer, data: Float32Array, size: number) {
    const gl = this.gl
    gl.bindBuffer(gl.ARRAY_BUFFER, buffer)
    gl.bufferData(gl.ARRAY_BUFFER, data, gl.STREAM_DRAW)
    gl.enableVertexAttribArray(location)
    gl.vertexAttribPointer(location, size, gl.FLOAT, false, 0, 0)
  }

  /** Draws `layers` at `seconds` into `target`, which shows `frame` of the plate. */
  draw(target: HTMLCanvasElement, layers: DrawLayer[], seconds: number, frame: PlateFrame) {
    const gl = this.gl
    const width = target.width
    const height = target.height
    if (this.canvas.width !== width || this.canvas.height !== height) {
      this.canvas.width = width
      this.canvas.height = height
    }
    gl.viewport(0, 0, width, height)
    gl.clearColor(0, 0, 0, 0)
    gl.clearStencil(0)
    gl.colorMask(true, true, true, true)
    gl.clear(gl.COLOR_BUFFER_BIT | gl.STENCIL_BUFFER_BIT)
    gl.useProgram(this.program)
    gl.enable(gl.BLEND)
    gl.blendFuncSeparate(gl.ONE, gl.ONE_MINUS_SRC_ALPHA, gl.ONE, gl.ONE_MINUS_SRC_ALPHA)
    gl.uniform2f(this.uniforms.size, width, height)
    // The 408x96 plate where the frame puts it, y up from the canvas's bottom.
    const scale = width / (frame[2] - frame[0])
    const box: [number, number, number, number] = [
      -frame[0] * scale,
      height - (96 - frame[1]) * scale,
      (408 - frame[0]) * scale,
      height + frame[1] * scale,
    ]
    for (const layer of layers) {
      const mesh = otanimMesh(layer.file, layer.clip, seconds, box, false)
      if (mesh.indices.length === 0) continue
      this.attribute(this.at.position, this.buffers.position, mesh.positions, 2)
      this.attribute(this.at.uv, this.buffers.uv, mesh.uvs, 2)
      this.attribute(this.at.tint, this.buffers.tint, mesh.colors, 4)
      gl.bindBuffer(gl.ELEMENT_ARRAY_BUFFER, this.buffers.index)
      gl.bufferData(gl.ELEMENT_ARRAY_BUFFER, mesh.indices, gl.STREAM_DRAW)
      gl.activeTexture(gl.TEXTURE0)
      gl.bindTexture(gl.TEXTURE_2D, this.texture(layer.picture))
      gl.uniform1f(this.uniforms.premultiplied, layer.file.premultiplied ? 1 : 0)
      gl.uniform1f(this.uniforms.mask, layer.mask ? 1 : 0)
      if (layer.mask) {
        gl.enable(gl.STENCIL_TEST)
        gl.stencilFunc(gl.ALWAYS, 1, 0xff)
        gl.stencilOp(gl.KEEP, gl.KEEP, gl.REPLACE)
        gl.colorMask(false, false, false, false)
      } else if (layer.cut) {
        gl.enable(gl.STENCIL_TEST)
        gl.stencilFunc(gl.EQUAL, 1, 0xff)
        gl.stencilOp(gl.KEEP, gl.KEEP, gl.KEEP)
        gl.colorMask(true, true, true, true)
      } else {
        gl.disable(gl.STENCIL_TEST)
        gl.colorMask(true, true, true, true)
      }
      gl.drawElements(gl.TRIANGLES, mesh.indices.length, gl.UNSIGNED_INT, 0)
    }
    gl.disable(gl.STENCIL_TEST)
    const context = target.getContext('2d')
    if (!context) return
    context.clearRect(0, 0, width, height)
    context.drawImage(this.canvas, 0, 0)
  }
}

let renderer: PlateRenderer | null | undefined
function sharedRenderer(): PlateRenderer | null {
  if (renderer === undefined) {
    try {
      renderer = new PlateRenderer()
    } catch {
      // Without WebGL 2 a plate is its text alone.
      renderer = null
    }
  }
  return renderer
}

type Entry = {
  canvas: HTMLCanvasElement
  frame: PlateFrame
  layers: DrawLayer[]
  since: number
  visible: boolean
  drawn: boolean
}

const entries = new Set<Entry>()
let request = 0

// Whether a list of layers moves at all: a still plate is drawn once.
const animated = (layers: DrawLayer[]) => layers.some((layer) => layer.clip.frames.length > 1)
const still = () =>
  typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches

function tick(now: number) {
  request = 0
  const draw = sharedRenderer()
  if (!draw) return
  let again = false
  for (const entry of entries) {
    if (!entry.visible || entry.canvas.width === 0 || entry.canvas.height === 0) continue
    const moves = animated(entry.layers) && !still()
    if (entry.drawn && !moves) continue
    draw.draw(entry.canvas, entry.layers, moves ? (now - entry.since) / 1000 : 0, entry.frame)
    entry.drawn = true
    again ||= moves
  }
  if (again) wake()
}

function wake() {
  if (!request && typeof requestAnimationFrame === 'function') request = requestAnimationFrame(tick)
}

let observer: IntersectionObserver | undefined
const byCanvas = new WeakMap<Element, Entry>()
function watch(entry: Entry) {
  if (typeof IntersectionObserver !== 'function') {
    entry.visible = true
    return
  }
  observer ??= new IntersectionObserver((seen) => {
    for (const item of seen) {
      const found = byCanvas.get(item.target)
      if (!found) continue
      found.visible = item.isIntersecting
      if (found.visible) wake()
    }
  })
  byCanvas.set(entry.canvas, entry)
  observer.observe(entry.canvas)
}

/**
 * Draws `layers` (null while they load) into the canvas, its size following the element's, on
 * the shared clock: each plate's clips start together when its layers are first shown.
 */
export function usePlateCanvas(
  canvas: RefObject<HTMLCanvasElement | null>,
  layers: DrawLayer[] | null,
  frame: PlateFrame,
) {
  const [x0, y0, x1, y1] = frame
  useEffect(() => {
    const element = canvas.current
    if (!element || !layers || layers.length === 0) return
    const entry: Entry = {
      canvas: element,
      frame: [x0, y0, x1, y1],
      layers,
      since: performance.now(),
      visible: false,
      drawn: false,
    }
    const fit = () => {
      const ratio = Math.min(globalThis.devicePixelRatio || 1, 2)
      const width = Math.round(element.clientWidth * ratio)
      const height = Math.round(element.clientHeight * ratio)
      if (width === element.width && height === element.height) return
      element.width = width
      element.height = height
      entry.drawn = false
      wake()
    }
    fit()
    const resize = typeof ResizeObserver === 'function' ? new ResizeObserver(fit) : null
    resize?.observe(element)
    entries.add(entry)
    watch(entry)
    wake()
    return () => {
      resize?.disconnect()
      observer?.unobserve(element)
      entries.delete(entry)
    }
  }, [canvas, layers, x0, y0, x1, y1])
}
