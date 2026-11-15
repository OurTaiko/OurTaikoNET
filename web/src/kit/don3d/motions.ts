import { Matrix4, Quaternion, Vector3 } from 'three'
import { Glb } from './glb'

// animations.glb: a skeleton without meshes and the 52 Don motions (26 and their _mirror
// versions, in DonAnim order). Each motion is sampled at 60 frames a second into model-space joint
// poses, as YataiDON's load_model_animations bakes them; a part's skinning matrix for a joint is
// then the pose times the part's inverse bind matrix (the game's DonMotions). Motions are sampled
// the first time they are asked for.

export const motionFrameRate = 60

type Sampler = {
  times: Float32Array
  values: Float32Array
  width: number
  step: boolean
  cubic: boolean
}

type Trs = { t: Vector3; r: Quaternion; s: Vector3 }

export class DonMotions {
  readonly names: string[]
  readonly frameCounts: number[]
  readonly joints: Map<string, number>
  readonly jointCount: number
  private readonly channels: (Sampler | null)[][][] // [motion][joint][path]
  private readonly external: Matrix4[]
  private readonly parentJoint: number[]
  private readonly rest: Trs[]
  private readonly order: number[]
  private readonly poses: (Float32Array | undefined)[]

  private constructor(glb: Glb) {
    const json = glb.json
    const skin = json.skins?.[0]
    const anims = json.animations ?? []
    const nodes = json.nodes ?? []
    const jointNodes = skin && anims.length ? skin.joints : []
    const joints = jointNodes.length
    this.jointCount = joints
    this.joints = new Map()
    const parentOf = new Array<number>(nodes.length).fill(-1)
    nodes.forEach((node, i) => node.children?.forEach((c) => (parentOf[c] = i)))
    const jointOfNode = new Map<number, number>()
    jointNodes.forEach((node, k) => {
      jointOfNode.set(node, k)
      this.joints.set(nodes[node]?.name ?? '', k)
    })
    // The plain (non-joint) nodes above each joint, which raylib bakes into its pose, and the
    // joint each joint hangs from.
    this.external = []
    this.parentJoint = []
    for (let k = 0; k < joints; k++) {
      const external = new Matrix4()
      let parent = -1
      for (let n = parentOf[jointNodes[k]]; n >= 0; n = parentOf[n]) {
        const pj = jointOfNode.get(n)
        if (pj !== undefined) {
          parent = pj
          break
        }
        external.premultiply(nodeMatrix(nodes[n]))
      }
      this.external.push(external)
      this.parentJoint.push(parent)
    }
    this.rest = jointNodes.map((n) => nodeTrs(nodes[n]))
    this.order = parentsFirst(this.parentJoint)

    this.names = []
    this.frameCounts = []
    this.channels = []
    for (const anim of joints ? anims : []) {
      const channels: (Sampler | null)[][] = Array.from({ length: joints }, () => [
        null,
        null,
        null,
      ])
      let duration = 0
      for (const ch of anim.channels) {
        const node = ch.target.node
        const k = node === undefined ? undefined : jointOfNode.get(node)
        if (k === undefined) continue
        const path = { translation: 0, rotation: 1, scale: 2 }[ch.target.path]
        if (path === undefined) continue
        const s = anim.samplers[ch.sampler]
        const sampler: Sampler = {
          times: glb.floats(s.input),
          values: glb.floats(s.output),
          width: path === 1 ? 4 : 3,
          step: s.interpolation === 'STEP',
          cubic: s.interpolation === 'CUBICSPLINE',
        }
        channels[k][path] = sampler
        if (sampler.times.length > 0)
          duration = Math.max(duration, sampler.times[sampler.times.length - 1])
      }
      this.names.push(anim.name ?? '')
      // the game's (int)(duration * FrameRate) + 1, in single precision as C# computes it
      this.frameCounts.push(Math.trunc(Math.fround(Math.fround(duration) * motionFrameRate)) + 1)
      this.channels.push(channels)
    }
    this.poses = new Array(this.names.length)
  }

  static load(data: ArrayBuffer | Uint8Array): DonMotions {
    return new DonMotions(Glb.parse(data))
  }

  get count(): number {
    return this.names.length
  }

  // The model-space pose of every joint for every frame of a motion: 16 floats (column-major) per
  // joint, frame * jointCount + joint.
  posesOf(motion: number): Float32Array {
    let poses = this.poses[motion]
    if (poses) return poses
    const joints = this.jointCount
    const frames = this.frameCounts[motion]
    const channels = this.channels[motion]
    poses = new Float32Array(frames * joints * 16)
    const t = Array.from({ length: joints }, () => new Vector3())
    const r = Array.from({ length: joints }, () => new Quaternion())
    const sc = Array.from({ length: joints }, () => new Vector3())
    const local = new Matrix4()
    const trs = new Matrix4()
    const tr = new Vector3()
    const ro = new Quaternion()
    const sca = new Vector3()
    const m = new Matrix4()
    for (let f = 0; f < frames; f++) {
      const time = f / motionFrameRate
      for (let k = 0; k < joints; k++) {
        const rest = this.rest[k]
        tr.copy(rest.t)
        ro.copy(rest.r)
        sca.copy(rest.s)
        const [st, sr, ss] = channels[k]
        if (st) sampleVector(st, time, tr)
        if (sr) sampleRotation(sr, time, ro)
        if (ss) sampleVector(ss, time, sca)
        local.multiplyMatrices(this.external[k], trs.compose(tr, ro, sca))
        decompose(local, t[k], r[k], sc[k])
      }
      // raylib BuildPoseFromParentJoints: compose each joint onto its parent's pose.
      for (const k of this.order) {
        const p = this.parentJoint[k]
        if (p < 0) continue
        r[k].premultiply(r[p])
        t[k].multiply(sc[p]).applyQuaternion(r[p]).add(t[p])
        sc[k].multiply(sc[p])
      }
      for (let k = 0; k < joints; k++) {
        m.compose(t[k], r[k], sc[k])
        poses.set(m.elements, (f * joints + k) * 16)
      }
    }
    this.poses[motion] = poses
    return poses
  }
}

function nodeTrs(node: { translation?: number[]; rotation?: number[]; scale?: number[] }): Trs {
  const t = node.translation
  const r = node.rotation
  const s = node.scale
  return {
    t: t ? new Vector3(t[0], t[1], t[2]) : new Vector3(),
    r: r ? new Quaternion(r[0], r[1], r[2], r[3]) : new Quaternion(),
    s: s ? new Vector3(s[0], s[1], s[2]) : new Vector3(1, 1, 1),
  }
}

// A node's local transform (matrix, or translation/rotation/scale).
function nodeMatrix(node: {
  matrix?: number[]
  translation?: number[]
  rotation?: number[]
  scale?: number[]
}): Matrix4 {
  if (node.matrix && node.matrix.length === 16) return new Matrix4().fromArray(node.matrix)
  const { t, r, s } = nodeTrs(node)
  return new Matrix4().compose(t, r, s)
}

function parentsFirst(parent: number[]): number[] {
  const order: number[] = []
  const done = new Array<boolean>(parent.length).fill(false)
  const visit = (k: number) => {
    if (done[k]) return
    if (parent[k] >= 0) visit(parent[k])
    done[k] = true
    order.push(k)
  }
  for (let k = 0; k < parent.length; k++) visit(k)
  return order
}

const c0 = new Vector3()
const c1 = new Vector3()
const c2 = new Vector3()
const right = new Vector3()
const basis = new Matrix4()

// As the game decomposes: scale is the columns' lengths (never negative), the rotation Unity's
// LookRotation(column 2, column 1).
function decompose(m: Matrix4, t: Vector3, r: Quaternion, s: Vector3): void {
  const e = m.elements
  c0.set(e[0], e[1], e[2])
  c1.set(e[4], e[5], e[6])
  c2.set(e[8], e[9], e[10])
  s.set(c0.length(), c1.length(), c2.length())
  t.set(e[12], e[13], e[14])
  if (s.x > 1e-8 && s.y > 1e-8 && s.z > 1e-8) {
    const forward = c2.divideScalar(s.z)
    const up = c1.divideScalar(s.y)
    right.crossVectors(up, forward)
    if (right.lengthSq() < 1e-12) {
      r.setFromUnitVectors(new Vector3(0, 0, 1), forward)
      return
    }
    right.normalize()
    up.crossVectors(forward, right)
    basis.makeBasis(right, up, forward)
    r.setFromRotationMatrix(basis)
  } else r.identity()
}

// The keyframe before time and how far toward the next.
function find(s: Sampler, time: number): [number, number] {
  const times = s.times
  const n = times.length
  if (n === 0 || time <= times[0]) return [0, 0]
  if (time >= times[n - 1]) return [n - 1, 0]
  let lo = 0
  let hi = n - 1
  while (hi - lo > 1) {
    const mid = (lo + hi) >> 1
    if (times[mid] <= time) lo = mid
    else hi = mid
  }
  const span = times[hi] - times[lo]
  return [lo, span > 0 && !s.step ? (time - times[lo]) / span : 0]
}

function value(s: Sampler, key: number, c: number): number {
  return s.cubic ? s.values[(key * 3 + 1) * s.width + c] : s.values[key * s.width + c]
}

function sampleVector(s: Sampler, time: number, out: Vector3): void {
  if (s.times.length === 0) return
  const [i, u] = find(s, time)
  out.set(value(s, i, 0), value(s, i, 1), value(s, i, 2))
  if (u <= 0 || i + 1 >= s.times.length) return
  out.lerp(new Vector3(value(s, i + 1, 0), value(s, i + 1, 1), value(s, i + 1, 2)), u)
}

const qa = new Quaternion()
const qb = new Quaternion()

function sampleRotation(s: Sampler, time: number, out: Quaternion): void {
  if (s.times.length === 0) return
  const [i, u] = find(s, time)
  normalized(qa.set(value(s, i, 0), value(s, i, 1), value(s, i, 2), value(s, i, 3)))
  if (u <= 0 || i + 1 >= s.times.length) {
    out.copy(qa)
    return
  }
  normalized(qb.set(value(s, i + 1, 0), value(s, i + 1, 1), value(s, i + 1, 2), value(s, i + 1, 3)))
  out.slerpQuaternions(qa, qb, u)
}

function normalized(q: Quaternion): Quaternion {
  const m = Math.sqrt(q.x * q.x + q.y * q.y + q.z * q.z + q.w * q.w)
  return m > 1e-8 ? q.set(q.x / m, q.y / m, q.z / m, q.w / m) : q.identity()
}
