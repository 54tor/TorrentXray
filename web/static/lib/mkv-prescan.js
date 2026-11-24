const EARLY_END_TOLERANCE_SECONDS = 2;
const EARLY_END_TOLERANCE_RATIO = 0.01;
const PROGRESS_REPORT_MIN_STEP = 0.002;

export function computeOverallRatio(trackRatios, previousRatio) {
  if (trackRatios.length === 0) {
    return previousRatio;
  }
  const meanRatio =
    trackRatios.reduce((sum, trackRatio) => sum + Math.min(1, trackRatio), 0) / trackRatios.length;

  return Math.max(previousRatio, meanRatio);
}

export function detectAnomalies({ durationSeconds, tracks, readError }) {
  const anomalies = [];

  if (readError) {
    anomalies.push({ code: 'read_error', detail: readError });
  }

  for (const track of tracks) {
    if (track.zeroSizePackets > 0) {
      anomalies.push({
        code: 'zero_size_packets',
        detail: `${track.type} #${track.number}: ${track.zeroSizePackets}`,
      });
    }

    const tolerance = Math.max(
      EARLY_END_TOLERANCE_SECONDS,
      durationSeconds * EARLY_END_TOLERANCE_RATIO,
    );
    const endsEarly =
      durationSeconds > 0 &&
      track.packetCount > 0 &&
      track.lastTimestampSeconds < durationSeconds - tolerance;
    if (endsEarly) {
      anomalies.push({
        code: 'track_ends_early',
        detail: `${track.type} #${track.number}: ${track.lastTimestampSeconds.toFixed(1)} s / ${durationSeconds.toFixed(1)} s`,
      });
    }

    if (track.packetCount === 0) {
      anomalies.push({ code: 'empty_track', detail: `${track.type} #${track.number}` });
    }
  }

  return anomalies;
}

async function readTrackPackets(track, EncodedPacketSink, onPacket) {
  let packetCount = 0;
  let zeroSizePackets = 0;
  let totalBytes = 0;
  let lastTimestampSeconds = 0;

  for await (const packet of new EncodedPacketSink(track).packets()) {
    packetCount += 1;
    totalBytes += packet.data.byteLength;
    if (packet.data.byteLength === 0) {
      zeroSizePackets += 1;
    }
    lastTimestampSeconds = Math.max(lastTimestampSeconds, packet.timestamp);
    onPacket(packet.timestamp);
  }

  return { packetCount, zeroSizePackets, totalBytes, lastTimestampSeconds };
}

export async function prescanMkv(file, { libraryUrl, onProgress }) {
  const startedAt = performance.now();
  const { Input, ALL_FORMATS, BlobSource, EncodedPacketSink } = await import(libraryUrl);
  const input = new Input({ formats: ALL_FORMATS, source: new BlobSource(file) });
  const summary = { containerName: null, durationSeconds: 0, tracks: [], readError: null };

  try {
    summary.containerName = (await input.getFormat()).name;
    summary.durationSeconds = await input.computeDuration();
    const inputTracks = await input.getTracks();

    const trackRatios = inputTracks.map(() => 0);
    let overallRatio = 0;
    let reportedRatio = -1;
    let packetCount = 0;
    const reportProgress = (trackIndex, timestamp) => {
      packetCount += 1;
      trackRatios[trackIndex] =
        summary.durationSeconds > 0 ? timestamp / summary.durationSeconds : 0;
      overallRatio = computeOverallRatio(trackRatios, overallRatio);
      if (overallRatio - reportedRatio >= PROGRESS_REPORT_MIN_STEP) {
        reportedRatio = overallRatio;
        onProgress?.({ ratio: overallRatio, packetCount });
      }
    };

    summary.tracks = await Promise.all(
      inputTracks.map(async (track, trackIndex) => ({
        type: track.type,
        number: track.number,
        codec: track.codec,
        language: await track.getLanguageCode(),
        ...(await readTrackPackets(track, EncodedPacketSink, (timestamp) =>
          reportProgress(trackIndex, timestamp),
        )),
      })),
    );
    onProgress?.({ ratio: 1, packetCount });
  } catch (readFailure) {
    summary.readError = readFailure?.message ?? String(readFailure);
  }

  return {
    ...summary,
    anomalies: detectAnomalies(summary),
    elapsedMilliseconds: Math.round(performance.now() - startedAt),
  };
}

export function groupErrorLines(errorLines, maxDistinctLines) {
  const occurrencesByLine = new Map();
  for (const errorLine of errorLines) {
    occurrencesByLine.set(errorLine, (occurrencesByLine.get(errorLine) ?? 0) + 1);
  }

  return [...occurrencesByLine.entries()].slice(0, maxDistinctLines);
}
