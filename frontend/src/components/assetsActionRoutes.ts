export function assetLabHref(assetPath: string) {
  return `/profile-lab?assetPath=${encodeURIComponent(assetPath)}`;
}
