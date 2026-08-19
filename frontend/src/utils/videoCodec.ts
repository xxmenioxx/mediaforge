export function normalizeLegacyVideoCodec(
  videoCodec: string,
  workerConfig?: Record<string, unknown>,
) {
  const normalizedWorkerConfig = {
    ...(workerConfig ?? {}),
  };

  if (videoCodec !== 'x265_10bit') {
    return {
      videoCodec,
      workerConfig: normalizedWorkerConfig,
    };
  }

  const currentPixFmt =
    typeof normalizedWorkerConfig.pixFmt === 'string'
      ? normalizedWorkerConfig.pixFmt.trim().toLowerCase()
      : '';

  if (!currentPixFmt || currentPixFmt === 'auto') {
    const encoder = String(
      normalizedWorkerConfig.videoEncoder ?? '',
    ).toLowerCase();

    const hardwareMain10 = [
      'hevc_qsv',
      'hevc_vaapi',
      'hevc_nvenc',
      'hevc_videotoolbox',
      'hevc_amf',
    ].includes(encoder);

    normalizedWorkerConfig.pixFmt = hardwareMain10
      ? 'p010le'
      : 'yuv420p10le';
  }

  return {
    videoCodec: 'x265',
    workerConfig: normalizedWorkerConfig,
  };
}