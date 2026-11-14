import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'

// The site's three areas each have a router of their own, which only knows its own pages.
// Going anywhere else on the site passes through here: the root (src/root.tsx) listens and
// swaps the area shown, without loading the page anew.
type Visitor = (address: string, replace: boolean) => void
let visitor: Visitor | null = null
/** The root says how addresses are visited; without it (tests) the browser loads them. */
export function setVisitor(next: Visitor) {
  visitor = next
}

/** Goes to an address of this site (/fanmade/charts/…, /players/…), in whichever area. */
export function visit(address: string, { replace = false } = {}) {
  if (visitor) visitor(address, replace)
  else if (replace) location.replace(address)
  else location.assign(address)
}

type Mover = (path: string, options: { replace: boolean }) => void
let mover: Mover | null = null
/** Moves inside the area being shown, by its router; false when no router is up. */
export function moveInArea(path: string, replace: boolean) {
  mover?.(path, { replace })
  return !!mover
}
/** Inside an area's router: lets the root move within the area. */
export function RouterBridge() {
  const navigate = useNavigate()
  useEffect(() => {
    const mine: Mover = (path, options) => void navigate(path, options)
    mover = mine
    return () => {
      if (mover === mine) mover = null
    }
  }, [navigate])
  return null
}
