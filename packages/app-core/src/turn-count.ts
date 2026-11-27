/** What waits on the founder, as one number: each cluster's Your turn rows, plus one for this Mac's stopped service. */
export function turnCount(perCluster: number[], serviceDown: boolean): number {
  return perCluster.reduce((n, c) => n + c, 0) + (serviceDown ? 1 : 0);
}
