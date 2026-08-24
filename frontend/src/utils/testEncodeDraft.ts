export function labAudioProfileForTestEncode(
  section: 'video' | 'audio' | 'tracks',
  profile: Record<string, unknown>,
): Record<string, unknown> | undefined {
  return section === 'audio' ? profile : undefined;
}
