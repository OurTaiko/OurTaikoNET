/**
 * Up to `limit` items of the lists, each list giving one in turn and the lists in the order of
 * `first` (highest first); a list that runs out leaves its turns to the others. The home
 * page shows both sources' newest charts this way, so that neither crowds the other out.
 */
export function inTurns<T>(lists: T[][], limit: number, first: (item: T) => string): T[] {
  const order = lists
    .filter((list) => list.length)
    .sort((a, b) => first(b[0]).localeCompare(first(a[0])))
  const taken: T[] = []
  for (let turn = 0; taken.length < limit && order.some((list) => turn < list.length); turn++)
    for (const list of order) if (turn < list.length && taken.length < limit) taken.push(list[turn])
  return taken
}
