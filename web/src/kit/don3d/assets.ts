import { catalogUrl, type DonCatalog, type DonCatalogFile } from './catalog'
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

export function loadBase(catalog: DonCatalog, baseUrl: string): Promise<DonBase> {
  const { animations, face, faceAnimations } = catalog.base
  const url = (file: DonCatalogFile) => catalogUrl(baseUrl, file, catalog.mirrorUrls)
  return remember(
    bases,
    url(animations) + '|' + url(face) + '|' + url(faceAnimations),
    async () => {
      const [motionBytes, faceBlob, faceJson] = await Promise.all([
        fetchOk(url(animations)).then((r) => r.arrayBuffer()),
        fetchOk(url(face)).then((r) => r.blob()),
        fetchOk(url(faceAnimations)).then((r) => r.json() as Promise<unknown>),
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

// Parsed parts by URL: the ones in use and the last few (going back and forth between costumes
// does not fetch them again).
const parts = new Map<string, Promise<DonPartData>>()
export const keptParts = 24

export function loadPart(url: string, normalizeFace: boolean): Promise<DonPartData> {
  const key = url + (normalizeFace ? '#face' : '')
  const cached = parts.get(key)
  if (cached) {
    parts.delete(key)
    parts.set(key, cached)
    return cached
  }
  const promise = remember(parts, key, async () =>
    loadDonPart(await (await fetchOk(url)).arrayBuffer(), normalizeFace),
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
export function loadPaint(url: string): Promise<ImageBitmap> {
  return remember(paints, url, async () => decodeImage(await (await fetchOk(url)).blob()))
}

// The file says which it is: an .otanim starts with "OTAN".
export function loadPuchi(url: string): Promise<DonPuchi> {
  return remember(puchis, url, async () => {
    const bytes = new Uint8Array(await (await fetchOk(url)).arrayBuffer())
    if (!isOtanim(bytes)) return { bitmap: await decodeImage(bytes), anim: null }
    const anim = await parseOtanim(bytes)
    return { bitmap: await decodeImage(anim.texture), anim }
  })
}
