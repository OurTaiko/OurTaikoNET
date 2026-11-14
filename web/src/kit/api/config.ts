import { SOURCE, onAreaChange } from '../source'

// Browser requests stay on the website origin; the proxy selects the backend.
/** What belongs to the whole site: signing in, players. */
export const SITE_API_ROOT = '/api/v1'
/** The song source being shown; follows it. */
export let API_ROOT = `${SITE_API_ROOT}/sources/${SOURCE}`
onAreaChange(() => {
  API_ROOT = `${SITE_API_ROOT}/sources/${SOURCE}`
})
