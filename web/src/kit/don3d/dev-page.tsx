// Development only: a page to look at the 3D Don (`pnpm dev`, then /dev/don3d.html). Not linked
// from the site and not in the build (vite builds index.html only). Query parameters:
//   assets=<3dchara folder URL> (default http://127.0.0.1:8765/), costume=0|1, cos, head, body,
//   puchi, colors=p1|p2, motion=<DonMotionName>, mirror=1, freeze=<0..1>, bg=<css colour>,
//   w, h (CSS pixels of each preview), grid=1 (the game's DonPreview shots side by side; freeze=0
//   lets them move), snap=1 (each shot drawn at sw × sh pixels, default the game's 960 × 1120, and
//   POSTed as <prefix><name>.png to upload=<URL>, default http://127.0.0.1:8766/; only=<name>).
// The assets need serving with CORS, e.g. a small http.server that adds Access-Control-Allow-Origin.
import { StrictMode, useEffect, useRef, useState } from 'react'
import { createRoot } from 'react-dom/client'
import '../i18n'
import { DonPreview } from './don-preview'
import { createDonRenderer } from './renderer'
import type { DonCatalog, DonLook } from './catalog'
import { donMotionNames, type DonMotionName } from './chara'

const params = new URLSearchParams(location.search)
const num = (key: string, fallback: number) => Number(params.get(key) ?? fallback)
const assets = params.get('assets') ?? 'http://127.0.0.1:8765/'
const width = num('w', 240)
const height = num('h', 280)
const background = params.get('bg') ?? '#bbbbbb'

type Shot = {
  name: string
  look: DonLook
  motion: DonMotionName
  progress: number
  mirror: boolean
}

function shots(catalog: DonCatalog): Shot[] {
  const basic: DonLook = { costume: true, cos: 0, head: 0, body: 0, puchi: 0, colors: null }
  const p2 = { ...basic, colors: catalog.defaultColors.p2 }
  return [
    { name: 'normal', look: basic, motion: 'Normal', progress: 0.25, mirror: false },
    { name: 'normal_2p_mirror', look: p2, motion: 'Normal', progress: 0.25, mirror: true },
    { name: 'combo', look: basic, motion: 'Combo', progress: 0.4, mirror: false },
    { name: 'sabi', look: basic, motion: 'Sabi', progress: 0.3, mirror: false },
    { name: 'miss', look: basic, motion: 'Miss', progress: 0.5, mirror: false },
    { name: 'select_loop', look: basic, motion: 'SelectLoop', progress: 0.2, mirror: false },
    { name: 'cos30', look: { ...basic, cos: 30 }, motion: 'Normal', progress: 0.25, mirror: false },
    {
      name: 'head3_body4',
      look: { ...basic, costume: false, head: 3, body: 4 },
      motion: 'Normal',
      progress: 0.25,
      mirror: false,
    },
    {
      name: 'puchi1',
      look: { ...basic, puchi: 1 },
      motion: 'Normal',
      progress: 0.25,
      mirror: false,
    },
  ]
}

// snap=1: draws each shot at the game's DonPreview size (960 × 1120 pixels: the 480 × 560 area at
// two pixels a unit) and posts it as <name>.png to `upload`.
function Snapper({ catalog }: { catalog: DonCatalog }) {
  const ref = useRef<HTMLDivElement>(null)
  const [done, setDone] = useState('')
  useEffect(() => {
    const box = ref.current
    if (!box) return
    const canvas = document.createElement('canvas')
    Object.assign(canvas.style, { width: '480px', height: '560px', background })
    box.prepend(canvas)
    const upload = params.get('upload') ?? 'http://127.0.0.1:8766/'
    const renderer = createDonRenderer(canvas, {
      catalog,
      baseUrl: assets,
      preserveDrawingBuffer: true,
    })
    renderer.resize(num('sw', 960), num('sh', 1120), 1)
    let cancelled = false
    const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))
    void (async () => {
      const only = params.get('only')
      for (const shot of shots(catalog)) {
        if (cancelled) return
        if (only && shot.name !== only) continue
        await renderer.setLook(shot.look)
        renderer.setMirrored(shot.mirror)
        renderer.setMotion(shot.motion)
        renderer.freeze(shot.progress)
        // a few drawn frames (a hidden page's are slow)
        for (let i = 0; i < 4; i++) await new Promise((r) => requestAnimationFrame(r))
        await wait(100)
        const blob = await new Promise<Blob | null>((resolve) =>
          canvas.toBlob(resolve, 'image/png'),
        )
        if (blob && !cancelled)
          await fetch(upload + (params.get('prefix') ?? '') + shot.name + '.png', {
            method: 'POST',
            body: blob,
          })
      }
      setDone('done')
    })()
    return () => {
      cancelled = true
      renderer.dispose()
      canvas.remove()
    }
  }, [catalog])
  return (
    <div ref={ref}>
      <p id="snap-state">{done}</p>
    </div>
  )
}

function Page() {
  const [catalog, setCatalog] = useState<DonCatalog | null>(null)
  const [error, setError] = useState('')
  useEffect(() => {
    fetch(assets + 'catalog.json')
      .then((r) => r.json() as Promise<DonCatalog>)
      .then(setCatalog, (e: unknown) => setError(String(e)))
  }, [])
  if (error) return <p>{error}</p>
  if (!catalog) return <p>catalog…</p>
  if (params.get('snap') === '1') return <Snapper catalog={catalog} />
  const box = { width, height, background }
  const freeze = params.get('freeze')
  if (params.get('grid') === '1')
    return (
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
        {shots(catalog).map((shot) => (
          <div key={shot.name} data-shot={shot.name} style={box}>
            <DonPreview
              catalog={catalog}
              baseUrl={assets}
              look={shot.look}
              motion={shot.motion}
              mirrored={shot.mirror}
              frozenAt={freeze === '0' ? null : shot.progress}
              className="size-full"
            />
          </div>
        ))}
      </div>
    )
  const motion = params.get('motion') ?? 'Normal'
  const look: DonLook = {
    costume: params.get('costume') !== '0',
    cos: num('cos', 0),
    head: num('head', 0),
    body: num('body', 0),
    puchi: num('puchi', 0),
    colors: params.get('colors') === 'p2' ? catalog.defaultColors.p2 : null,
  }
  return (
    <div style={box}>
      <DonPreview
        catalog={catalog}
        baseUrl={assets}
        look={look}
        motion={
          (donMotionNames as readonly string[]).includes(motion)
            ? (motion as DonMotionName)
            : 'Normal'
        }
        mirrored={params.get('mirror') === '1'}
        frozenAt={freeze === null ? null : Number(freeze)}
        className="size-full"
      />
    </div>
  )
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <Page />
  </StrictMode>,
)
