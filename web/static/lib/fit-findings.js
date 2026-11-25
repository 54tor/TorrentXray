export const MAX_BODY_BYTES = 262144;
export const MAX_MEDIA_FILES = 50;
const MAX_TORRENT_FILES = 1000;
const MAX_PIECE_RANGES = 200;
const MAX_FILES_PER_RANGE = 20;
const PROBLEM_STATUSES = ['suspect', 'size_mismatch', 'missing', 'unverifiable'];
const MAX_MEDIA_INFO_CHARS = 20000;
const MEDIA_INFO_BUDGETS = [6000, 1500, 0];
const ERROR_LINE_BUDGETS = [100, 20, 0];
const PIECE_RANGE_BUDGETS = [50, 10, 0];

const encoder = new TextEncoder();

export function jsonByteSize(value) {
  return encoder.encode(JSON.stringify(value)).length;
}

function applyServerCaps(findings) {
  const torrent = findings.torrent;
  if (!torrent) {
    return findings;
  }
  const problemFiles = torrent.files.filter((file) => PROBLEM_STATUSES.includes(file.status));
  const healthyFiles = torrent.files.filter((file) => !PROBLEM_STATUSES.includes(file.status));
  return {
    ...findings,
    torrent: {
      ...torrent,
      files: [...problemFiles, ...healthyFiles].slice(0, MAX_TORRENT_FILES),
      pieceRanges: torrent.pieceRanges
        .slice(0, MAX_PIECE_RANGES)
        .map((range) => ({ ...range, files: range.files.slice(0, MAX_FILES_PER_RANGE) })),
    },
  };
}

function withMediaInfoBudget(findings, budget) {
  const checks = findings.checks.map((check) =>
    typeof check.mediaInfo === 'string' && check.mediaInfo.length > budget
      ? { ...check, mediaInfo: check.mediaInfo.slice(0, budget) }
      : check,
  );
  return { ...findings, checks };
}

function withoutHealthyTorrentFiles(findings) {
  if (!findings.torrent) {
    return findings;
  }
  const files = findings.torrent.files.filter((file) => PROBLEM_STATUSES.includes(file.status));
  return { ...findings, torrent: { ...findings.torrent, files } };
}

function withErrorLineBudget(findings, budget) {
  const checks = findings.checks.map((check) => ({
    ...check,
    ffmpeg: { ...check.ffmpeg, errors: check.ffmpeg.errors.slice(0, budget) },
  }));
  return { ...findings, checks };
}

function withPieceRangeBudget(findings, budget) {
  if (!findings.torrent) {
    return findings;
  }
  return {
    ...findings,
    torrent: { ...findings.torrent, pieceRanges: findings.torrent.pieceRanges.slice(0, budget) },
  };
}

export function fitFindingsToServerLimits(findings) {
  if (findings.checks.length > MAX_MEDIA_FILES) {
    return null;
  }
  const reductions = [
    ...MEDIA_INFO_BUDGETS.map((budget) => (candidate) => withMediaInfoBudget(candidate, budget)),
    withoutHealthyTorrentFiles,
    ...ERROR_LINE_BUDGETS.map((budget) => (candidate) => withErrorLineBudget(candidate, budget)),
    ...PIECE_RANGE_BUDGETS.map((budget) => (candidate) => withPieceRangeBudget(candidate, budget)),
  ];
  let candidate = withMediaInfoBudget(applyServerCaps(findings), MAX_MEDIA_INFO_CHARS);
  if (jsonByteSize(candidate) <= MAX_BODY_BYTES) {
    return candidate;
  }
  for (const reduce of reductions) {
    candidate = reduce(candidate);
    if (jsonByteSize(candidate) <= MAX_BODY_BYTES) {
      return candidate;
    }
  }
  return null;
}
