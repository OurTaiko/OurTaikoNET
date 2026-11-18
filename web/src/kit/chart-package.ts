import { downloadZip } from 'client-zip'
import { endpoints } from './api/endpoints'
import type { ChartBase } from './api/types'
import { withChartTitles } from './tja-titles'
import { pathResource, readChartResources, verifiedResource } from './resource-download'

type PackageFile = {
  url: string
  fallbackUrl?: string
  name: string
  sha256?: string
  size?: number
  tja?: boolean
}

/** Stored files that make up a chart package, in archive order. */
const packaged = ['tja', 'audio', 'movie', 'cover', 'bgimage'] as const

/** Characters that Windows, macOS or Linux reject in a file name. */
const unsafe = /[\\/:*?"<>|\p{Cc}]/gu

/** Name the archive after the title shown on the page. */
export function packageFilename(title: string) {
  const name = title
    .replace(unsafe, ' ')
    .replace(/\s+/g, ' ')
    .replace(/^[ .]+|[ .]+$/g, '')
  return `${name || 'chart'}.zip`
}

/** Files keep their stored names, so the TJA's WAVE finds the audio beside it. */
export function entryName(path: string, sha256: string) {
  const extension = /\.([a-z0-9]+)$/i.exec(new URL(path, 'http://localhost').pathname)?.[1]
  if (!extension) throw new Error('PACKAGE_RESOURCE_INVALID')
  return `${sha256}.${extension.toLowerCase()}`
}

/** Without a manifest the API serves the same stored files itself. */
async function apiFiles(chart: ChartBase): Promise<PackageFile[]> {
  // The API does not publish the stored TJA's hash any more (a chart's identity is each
  // block's chartHash); the fallback trusts the API's own file route for it.
  const files: PackageFile[] = [
    { url: endpoints.resource(chart, 'tja'), name: chart.tjaName, tja: true },
    { url: endpoints.resource(chart, 'audio'), name: chart.wave, sha256: chart.audioHash },
  ]
  if (chart.movieHash)
    files.push({
      url: endpoints.resource(chart, 'movie'),
      name: `${chart.movieHash}.mp4`,
      sha256: chart.movieHash,
    })
  if (chart.coverHash && chart.songCoverPath)
    files.push({
      ...(await pathResource(chart.songCoverPath)),
      name: `${chart.coverHash}.webp`,
      sha256: chart.coverHash,
    })
  if (chart.bgimageHash)
    files.push({
      url: endpoints.resource(chart, 'bgimage'),
      name: `${chart.bgimageHash}.webp`,
      sha256: chart.bgimageHash,
    })
  return files
}

async function packageFiles(chart: ChartBase, signal: AbortSignal): Promise<PackageFile[]> {
  let resources
  try {
    resources = await readChartResources(chart.id, signal)
  } catch (error) {
    if (error instanceof Error && error.message === 'RESOURCES_UNAVAILABLE')
      throw new Error('PACKAGE_RESOURCES_UNAVAILABLE', { cause: error })
    throw error
  }
  if (!resources) return apiFiles(chart)
  // The page may be older than the files it would name the archive after.
  if (!resources.tja || resources.audio?.sha256 !== chart.audioHash)
    throw new Error('PACKAGE_CHART_CHANGED')
  const files = await Promise.all(
    packaged.map(async (kind) => {
      const file = resources[kind]
      if (!file) return []
      return [
        {
          ...(await pathResource(file.path, { size: file.size, sha256: file.sha256 })),
          name: entryName(file.path, file.sha256),
          tja: kind === 'tja',
        },
      ]
    }),
  )
  return files.flat()
}

/**
 * Download the chart's stored files and pack them in the browser. Every file
 * is checked against the hash the API published before it enters the archive.
 */
export async function buildChartPackage(chart: ChartBase, signal: AbortSignal): Promise<Blob> {
  const files = await packageFiles(chart, signal)
  const entries = await Promise.all(
    files.map(async ({ url, fallbackUrl, name, sha256: expected, size, tja }) => {
      let input: ArrayBuffer
      try {
        input = await verifiedResource({ url, fallbackUrl, sha256: expected, size }, signal)
      } catch (error) {
        if (signal.aborted) throw error
        throw new Error(
          error instanceof Error && error.message === 'RESOURCE_INTEGRITY_FAILED'
            ? 'PACKAGE_INTEGRITY_FAILED'
            : 'PACKAGE_DOWNLOAD_FAILED',
          { cause: error },
        )
      }
      // After the check, the TJA's headers get the chart's own names; the
      // stored file keeps what was uploaded.
      if (tja) {
        const text = new TextDecoder('utf-8', { fatal: true }).decode(input)
        return {
          name,
          input: new TextEncoder().encode(withChartTitles(text, chart)),
          lastModified: new Date(chart.createdAt),
        }
      }
      return { name, input, lastModified: new Date(chart.createdAt) }
    }),
  )
  return downloadZip(entries).blob()
}

/** Hand a finished archive to the browser's download manager. */
export function saveBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.click()
  // Revoking immediately can cancel the download in some browsers.
  setTimeout(() => URL.revokeObjectURL(url), 60_000)
}
