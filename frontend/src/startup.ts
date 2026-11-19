export async function initializeHomeState(
  refreshHome: () => Promise<void>,
): Promise<void> {
  await refreshHome()
}
