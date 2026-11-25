import assert from 'node:assert/strict';
import { test } from 'node:test';
import { fitFindingsToServerLimits, jsonByteSize, MAX_BODY_BYTES } from './fit-findings.js';

function buildCheck(errorCount) {
  const errors = Array.from({ length: errorCount }, (_, lineIndex) => [
    `erreur ${lineIndex} ${'x'.repeat(280)}`,
    3,
  ]);
  return {
    file: { name: 'a.mkv', size: 1 },
    prescan: { anomalies: [], readError: null },
    ffmpeg: { exitCode: 1, totalErrorLines: errorCount * 3, errors },
  };
}

function buildTorrent(fileCount, status) {
  const files = Array.from({ length: fileCount }, (_, fileIndex) => ({
    path: `dir/${fileIndex}.mkv`,
    size: 10,
    status,
    localPath: `dir/${fileIndex}.mkv`,
  }));
  return { badPieceCount: 0, zeroPieceCount: 0, unverifiablePieces: 0, pieceRanges: [], files };
}

test('refuse plus de 50 contrôles', () => {
  const findings = { torrent: null, checks: Array.from({ length: 51 }, () => buildCheck(0)) };
  assert.equal(fitFindingsToServerLimits(findings), null);
});

test('laisse intacte une charge sous la limite', () => {
  const findings = { torrent: null, checks: [buildCheck(2)] };
  assert.deepEqual(fitFindingsToServerLimits(findings), findings);
});

test('réduit une charge trop grosse sous la limite en gardant les totaux', () => {
  const findings = { torrent: null, checks: Array.from({ length: 4 }, () => buildCheck(500)) };
  assert.ok(jsonByteSize(findings) > MAX_BODY_BYTES);
  const fitted = fitFindingsToServerLimits(findings);
  assert.ok(jsonByteSize(fitted) <= MAX_BODY_BYTES);
  assert.equal(fitted.checks[0].ffmpeg.totalErrorLines, 1500);
  assert.equal(fitted.checks[0].ffmpeg.exitCode, 1);
});

test('retire les fichiers sains du torrent avant les erreurs ffmpeg', () => {
  const torrent = buildTorrent(3000, 'healthy');
  torrent.files.push({ path: 'bad.mkv', size: 1, status: 'missing', localPath: '' });
  const fitted = fitFindingsToServerLimits({ torrent, checks: [buildCheck(5)] });
  assert.ok(jsonByteSize(fitted) <= MAX_BODY_BYTES);
  assert.ok(fitted.torrent.files.some((file) => file.status === 'missing'));
  assert.equal(fitted.checks[0].ffmpeg.errors.length, 5);
});

test('retourne null quand rien ne suffit', () => {
  const torrent = buildTorrent(1000, 'missing');
  torrent.files.forEach((file) => {
    file.path = 'p'.repeat(2000) + file.path;
  });
  assert.equal(fitFindingsToServerLimits({ torrent, checks: [] }), null);
});

test('rogne la fiche MediaInfo avant les erreurs ffmpeg', () => {
  const checks = Array.from({ length: 20 }, () => ({
    ...buildCheck(1),
    mediaInfo: 'm'.repeat(20000),
  }));
  const fitted = fitFindingsToServerLimits({ torrent: null, checks });
  assert.ok(jsonByteSize(fitted) <= MAX_BODY_BYTES);
  assert.equal(fitted.checks[0].ffmpeg.errors.length, 1);
});
