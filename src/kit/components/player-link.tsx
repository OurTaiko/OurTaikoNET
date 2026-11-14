import type { ComponentProps } from 'react'
import { Link } from 'react-router-dom'
import { AREA } from '../source'

export const playerPage = (id: string) => `/players/${encodeURIComponent(id)}`

/**
 * A link to a player's page. The page belongs to no source, so from a source's pages it is
 * a plain link, which the root (src/root.tsx) follows by swapping the area in place.
 */
export function PlayerLink({ id, ...props }: { id: string } & Omit<ComponentProps<'a'>, 'href'>) {
  return AREA === '' ? (
    <Link to={playerPage(id)} {...props} />
  ) : (
    <a href={playerPage(id)} {...props} />
  )
}
