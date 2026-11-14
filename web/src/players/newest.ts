/**
 * The `limit` newest items of the lists taken together, newest first, by the time `at` gives
 * for each (an ISO date). The home page shows the newest charts of both sources this way,
 * whichever source they are in.
 */
export function newestOf<T>(lists: T[][], limit: number, at: (item: T) => string): T[] {
  return lists
    .flat()
    .map((item) => ({ item, time: Date.parse(at(item)) || 0 }))
    .sort((a, b) => b.time - a.time)
    .slice(0, limit)
    .map(({ item }) => item)
}
