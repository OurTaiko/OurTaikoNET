/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** The commit the build was made from (./revision.ts); vite.config.ts sets it. */
  readonly VITE_REVISION?: string
}
