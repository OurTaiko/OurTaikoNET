import {
  donSource,
  type DonCatalog,
  type DonCatalogFile,
  type DonRoot,
  type DonSource,
} from './catalog'
import { parseFaceTimelines, type FaceTimeline } from './face'
import { DonMotions } from './motions'
import { isOtanim, parseOtanim, type Otanim } from './otanim'
import { loadDonPart, type DonImage, type DonPartData } from './part'

// What every Don on the page shares, fetched once and kept: the motions, the face sheet and its
// timelines, parsed parts (the last few), decoded pictures. GPU objects are the renderer's.

export type DonBase = {
  motions: DonMotions
  faceTimelines: Map<number, FaceTimeline>
  faceSheet: ImageBitmap
  faceCount: number
}

async function fetchOk(url: string): Promise<Response> {
  const response = await fetch(url)
  if (!response.ok) throw new Error(`${url}: HTTP ${response.status}`)
  return response
}

function hex(digest: ArrayBuffer): string {
  return Array.from(new Uint8Array(digest), (b) => b.toString(16).padStart(2, '0')).join('')
}

// A catalog file: the mirror's copy when it arrives whole with the catalog's SHA-256, else the
// canonical one (fetched as it always was).
export async function fetchDonFile(source: DonSource): Promise<Response> {
  if (source.mirror) {
    try {
      const response = await fetch(source.mirror)
      if (response.ok) {
        const bytes = await response.arrayBuffer()
        if (hex(await crypto.subtle.digest('SHA-256', bytes)) === source.sha256) {
          const type = response.headers.get('Content-Type')
          return new Response(bytes, { headers: type ? { 'Content-Type': type } : {} })
        }
      }
    } catch {
      // the canonical one below
    }
  }
  return fetchOk(source.url)
}

export function decodeImage(
  bytes: Uint8Array | Blob,
  mimeType = 'image/png',
): Promise<ImageBitmap> {
  const blob = bytes instanceof Blob ? bytes : new Blob([bytes as BlobPart], { type: mimeType })
  return createImageBitmap(blob, { premultiplyAlpha: 'none', colorSpaceConversion: 'none' })
}

// A promise kept only while it does not fail, so a later call tries again.
function remember<K, V>(map: Map<K, Promise<V>>, key: K, make: () => Promise<V>): Promise<V> {
  let promise = map.get(key)
  if (!promise) {
    promise = make()
    map.set(key, promise)
    promise.catch(() => map.delete(key))
  }
  return promise
}

const bases = new Map<string, Promise<DonBase>>()

export function loadBase(catalog: DonCatalog, root: DonRoot): Promise<DonBase> {
  const { animations, face, faceAnimations } = catalog.base
  const source = (file: DonCatalogFile) => donSource(root, file)
  // kept by the canonical addresses: a mirror's copy has the same bytes
  return remember(
    bases,
    [animations, face, faceAnimations].map((file) => source(file).url).join('|'),
    async () => {
      const [motionBytes, faceBlob, faceJson] = await Promise.all([
        fetchDonFile(source(animations)).then((r) => r.arrayBuffer()),
        fetchDonFile(source(face)).then((r) => r.blob()),
        fetchDonFile(source(faceAnimations)).then((r) => r.json() as Promise<unknown>),
      ])
      const faceSheet = await decodeImage(faceBlob)
      return {
        motions: DonMotions.load(motionBytes),
        faceTimelines: parseFaceTimelines(faceJson),
        faceSheet,
        faceCount: faceSheet.width > 0 ? Math.floor(faceSheet.height / faceSheet.width) : 0,
      }
    },
  )
}

// Parsed parts by canonical URL: the ones in use and the last few (going back and forth between costumes
// does not fetch them again).
const parts = new Map<string, Promise<DonPartData>>()
export const keptParts = 24

export function loadPart(source: DonSource, normalizeFace: boolean): Promise<DonPartData> {
  const key = source.url + (normalizeFace ? '#face' : '')
  const cached = parts.get(key)
  if (cached) {
    parts.delete(key)
    parts.set(key, cached)
    return cached
  }
  const promise = remember(parts, key, async () =>
    loadDonPart(await (await fetchDonFile(source)).arrayBuffer(), normalizeFace),
  )
  while (parts.size > keptParts) parts.delete(parts.keys().next().value as string)
  return promise
}

const bitmaps = new WeakMap<DonImage, Promise<ImageBitmap>>()

export function partImage(image: DonImage): Promise<ImageBitmap> {
  let promise = bitmaps.get(image)
  if (!promise) {
    promise = decodeImage(image.bytes, image.mimeType)
    bitmaps.set(image, promise)
  }
  return promise
}

const pixels = new WeakMap<DonImage, Promise<ImageData>>()

// An image's RGBA bytes, for recolouring.
export function partPixels(image: DonImage): Promise<ImageData> {
  let promise = pixels.get(image)
  if (!promise) {
    promise = partImage(image).then((bitmap) => {
      const canvas = new OffscreenCanvas(bitmap.width, bitmap.height)
      const context = canvas.getContext('2d', { willReadFrequently: true })
      if (!context) throw new Error('no 2D canvas')
      context.drawImage(bitmap, 0, 0)
      return context.getImageData(0, 0, bitmap.width, bitmap.height)
    })
    pixels.set(image, promise)
  }
  return promise
}

// A puchichara: a two-frame picture, or an animated one (.otanim; bitmap is then its texture).
export type DonPuchi = { bitmap: ImageBitmap; anim: Otanim | null }

const puchis = new Map<string, Promise<DonPuchi>>()
const paints = new Map<string, Promise<ImageBitmap>>()

// A makeup's face sheet (square faces stacked top to bottom, like the base one).
export function loadPaint(source: DonSource): Promise<ImageBitmap> {
  return remember(paints, source.url, async () =>
    decodeImage(await (await fetchDonFile(source)).blob()),
  )
}

// The file says which it is: an .otanim starts with "OTAN".
export function loadPuchi(source: DonSource): Promise<DonPuchi> {
  return remember(puchis, source.url, async () => {
    const bytes = new Uint8Array(await (await fetchDonFile(source)).arrayBuffer())
    if (!isOtanim(bytes)) return { bitmap: await decodeImage(bytes), anim: null }
    const anim = await parseOtanim(bytes)
    return { bitmap: await decodeImage(anim.texture), anim }
  })
}
