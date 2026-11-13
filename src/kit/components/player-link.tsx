import type { ComponentProps } from 'react'
import { Link } from 'react-router-dom'
import { AREA } from '../source'

export const playerPage = (id: string) => `/players/${encodeURIComponent(id)}`

/**
 * A link to a player's page. The page belongs to no source, so from a source's pages it is
 * loaded anew.
 */
export function PlayerLink({ id, ...props }: { id: string } & Omit<ComponentProps<'a'>, 'href'>) {
  return AREA === '' ? (
    <Link to={playerPage(id)} {...props} />
  ) : (
    <a href={playerPage(id)} {...props} />
  )
}
