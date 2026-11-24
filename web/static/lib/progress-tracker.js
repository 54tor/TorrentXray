const BYTE_UNITS = ['o', 'Ko', 'Mo', 'Go', 'To'];
const SECONDS_PER_MINUTE = 60;
const SECONDS_PER_HOUR = 3600;
const MIN_RATIO_FOR_ESTIMATE = 0.01;
const MIN_ELAPSED_FOR_ESTIMATE_SECONDS = 1;

export function formatBytes(byteCount) {
  let unitIndex = 0;
  let scaledValue = byteCount;
  while (scaledValue >= 1024 && unitIndex < BYTE_UNITS.length - 1) {
    scaledValue /= 1024;
    unitIndex += 1;
  }

  return `${scaledValue.toFixed(unitIndex === 0 ? 0 : 2)} ${BYTE_UNITS[unitIndex]}`;
}

export function formatDuration(totalSeconds) {
  const wholeSeconds = Math.max(0, Math.round(totalSeconds));
  const hours = Math.floor(wholeSeconds / SECONDS_PER_HOUR);
  const minutes = Math.floor((wholeSeconds % SECONDS_PER_HOUR) / SECONDS_PER_MINUTE);
  const seconds = wholeSeconds % SECONDS_PER_MINUTE;
  const paddedSeconds = String(seconds).padStart(2, '0');

  return hours > 0
    ? `${hours}:${String(minutes).padStart(2, '0')}:${paddedSeconds}`
    : `${minutes}:${paddedSeconds}`;
}

export function describeProgress({ ratio, totalBytes, stageElapsedSeconds }) {
  const boundedRatio = Math.min(1, Math.max(0, ratio));
  const doneBytes = totalBytes * boundedRatio;
  const hasEnoughData =
    boundedRatio >= MIN_RATIO_FOR_ESTIMATE &&
    stageElapsedSeconds >= MIN_ELAPSED_FOR_ESTIMATE_SECONDS;

  return {
    percent: Math.round(boundedRatio * 100),
    doneBytes,
    totalBytes,
    bytesPerSecond: hasEnoughData && totalBytes > 0 ? doneBytes / stageElapsedSeconds : null,
    remainingSeconds: hasEnoughData
      ? (stageElapsedSeconds * (1 - boundedRatio)) / boundedRatio
      : null,
  };
}
