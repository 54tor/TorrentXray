import { decode, infoHashFromTorrent } from './bencode.js';

const SHA1_BYTE_LENGTH = 20;
const textDecoder = new TextDecoder('utf-8');

function readText(value) {
  return value instanceof Uint8Array ? textDecoder.decode(value) : '';
}

export async function parseTorrentLayout(torrentBytes) {
  const root = decode(torrentBytes);
  const info = root instanceof Map ? root.get('info') : undefined;
  if (!(info instanceof Map)) {
    throw new Error('Torrent without info dictionary.');
  }

  const pieceLength = info.get('piece length');
  const pieceHashes = info.get('pieces');
  if (
    !Number.isSafeInteger(pieceLength) ||
    pieceLength <= 0 ||
    !(pieceHashes instanceof Uint8Array) ||
    pieceHashes.length % SHA1_BYTE_LENGTH !== 0
  ) {
    throw new Error('Torrent without a valid v1 piece table.');
  }

  const name = readText(info.get('name'));
  const rawFiles = info.get('files');
  const isMultiFile = Array.isArray(rawFiles);
  const files = [];
  let offset = 0;

  const sizedEntries = isMultiFile
    ? rawFiles.map((rawFile) => ({
        path: rawFile.get('path').map(readText).join('/'),
        size: rawFile.get('length'),
      }))
    : [{ path: name, size: info.get('length') }];

  for (const entry of sizedEntries) {
    files.push({ path: entry.path, size: entry.size, offset });
    offset += entry.size;
  }

  const pieceCount = pieceHashes.length / SHA1_BYTE_LENGTH;

  return {
    name,
    infoHash: await infoHashFromTorrent(torrentBytes),
    pieceLength,
    pieceHashes,
    pieceCount,
    totalSize: offset,
    isMultiFile,
    files,
    zeroPieceIndexes: await findZeroPieces(pieceHashes, pieceLength, offset),
  };
}

async function findZeroPieces(pieceHashes, pieceLength, totalSize) {
  const pieceCount = pieceHashes.length / SHA1_BYTE_LENGTH;
  const lastPieceLength = totalSize - (pieceCount - 1) * pieceLength;
  const fullPieceZeroHash = toHex(
    new Uint8Array(await crypto.subtle.digest('SHA-1', new Uint8Array(pieceLength))),
  );
  const lastPieceZeroHash = toHex(
    new Uint8Array(
      await crypto.subtle.digest('SHA-1', new Uint8Array(Math.max(0, lastPieceLength))),
    ),
  );

  const zeroPieceIndexes = [];
  for (let pieceIndex = 0; pieceIndex < pieceCount; pieceIndex += 1) {
    const expectedHash = toHex(
      pieceHashes.subarray(pieceIndex * SHA1_BYTE_LENGTH, (pieceIndex + 1) * SHA1_BYTE_LENGTH),
    );
    const zeroHash = pieceIndex === pieceCount - 1 ? lastPieceZeroHash : fullPieceZeroHash;
    if (expectedHash === zeroHash) {
      zeroPieceIndexes.push(pieceIndex);
    }
  }

  return zeroPieceIndexes;
}

function normalizePath(path) {
  return path.replaceAll('\\', '/').replace(/^\/+/, '');
}

export function matchLocalFiles(layout, localEntries) {
  const locals = localEntries.map((entry) => ({ ...entry, path: normalizePath(entry.path) }));
  const usedLocalIndexes = new Set();

  const findLocalIndex = (predicate) =>
    locals.findIndex((local, localIndex) => !usedLocalIndexes.has(localIndex) && predicate(local));

  return layout.files.map((torrentFile, torrentFileIndex) => {
    const fullPath = layout.isMultiFile ? `${layout.name}/${torrentFile.path}` : torrentFile.path;
    const baseName = torrentFile.path.split('/').pop();

    let matchKind = 'full_path';
    let localIndex = findLocalIndex((local) => local.path === fullPath);
    if (localIndex < 0) {
      matchKind = 'relative_path';
      localIndex = findLocalIndex(
        (local) => local.path === torrentFile.path || local.path.endsWith(`/${torrentFile.path}`),
      );
    }
    if (localIndex < 0) {
      matchKind = 'name_and_size';
      localIndex = findLocalIndex(
        (local) => local.path.split('/').pop() === baseName && local.file.size === torrentFile.size,
      );
    }
    if (localIndex >= 0) {
      usedLocalIndexes.add(localIndex);
    }

    const local = localIndex >= 0 ? locals[localIndex] : null;

    return {
      torrentFileIndex,
      torrentFile,
      localFile: local?.file ?? null,
      localPath: local?.path ?? null,
      matchKind: local === null ? null : matchKind,
      isSizeMatching: local !== null && local.file.size === torrentFile.size,
    };
  });
}

function toHex(bytes) {
  return [...bytes].map((byte) => byte.toString(16).padStart(2, '0')).join('');
}

async function readPiece(layout, matches, pieceIndex, pieceBuffer) {
  const pieceStart = pieceIndex * layout.pieceLength;
  const pieceEnd = Math.min(pieceStart + layout.pieceLength, layout.totalSize);
  let writtenBytes = 0;
  let isComplete = true;

  for (const match of matches) {
    const fileStart = match.torrentFile.offset;
    const fileEnd = fileStart + match.torrentFile.size;
    const overlapStart = Math.max(pieceStart, fileStart);
    const overlapEnd = Math.min(pieceEnd, fileEnd);
    if (overlapStart >= overlapEnd) {
      continue;
    }

    const expectedLength = overlapEnd - overlapStart;
    if (match.localFile === null) {
      isComplete = false;
      writtenBytes += expectedLength;
      continue;
    }

    const segment = new Uint8Array(
      await match.localFile.slice(overlapStart - fileStart, overlapEnd - fileStart).arrayBuffer(),
    );
    pieceBuffer.set(segment, writtenBytes);
    writtenBytes += expectedLength;
    if (segment.length < expectedLength) {
      return { status: 'bad', length: writtenBytes };
    }
  }

  return { status: isComplete ? 'read' : 'unverifiable', length: writtenBytes };
}

export async function verifyPieces(
  layout,
  matches,
  { onProgress, isCancelled, pieceIndexes } = {},
) {
  const pieceBuffer = new Uint8Array(layout.pieceLength);
  const badPieces = [];
  const unverifiablePieceIndexes = [];
  const indexesToCheck =
    pieceIndexes ?? Array.from({ length: layout.pieceCount }, (_, pieceIndex) => pieceIndex);

  for (const [position, pieceIndex] of indexesToCheck.entries()) {
    if (isCancelled?.()) {
      throw new Error('cancelled');
    }

    const piece = await readPiece(layout, matches, pieceIndex, pieceBuffer);
    if (piece.status === 'unverifiable') {
      unverifiablePieceIndexes.push(pieceIndex);
    } else if (piece.status === 'bad') {
      badPieces.push(pieceIndex);
    } else {
      const digest = new Uint8Array(
        await crypto.subtle.digest('SHA-1', pieceBuffer.subarray(0, piece.length)),
      );
      const expected = layout.pieceHashes.subarray(
        pieceIndex * SHA1_BYTE_LENGTH,
        (pieceIndex + 1) * SHA1_BYTE_LENGTH,
      );
      if (toHex(digest) !== toHex(expected)) {
        badPieces.push(pieceIndex);
      }
    }

    onProgress?.((position + 1) / indexesToCheck.length);
  }

  const zeroPieceIndexes = pieceIndexes
    ? layout.zeroPieceIndexes.filter((pieceIndex) => pieceIndexes.includes(pieceIndex))
    : layout.zeroPieceIndexes;
  const corruptedPieces = [...new Set([...badPieces, ...zeroPieceIndexes])].sort(
    (first, second) => first - second,
  );

  return { badPieces: corruptedPieces, zeroPieceIndexes, unverifiablePieceIndexes };
}

function isPieceTouchingFile(layout, pieceIndex, torrentFile) {
  const pieceStart = pieceIndex * layout.pieceLength;
  const pieceEnd = Math.min(pieceStart + layout.pieceLength, layout.totalSize);

  return torrentFile.offset < pieceEnd && torrentFile.offset + torrentFile.size > pieceStart;
}

export function classifyFiles(layout, matches, { badPieces, unverifiablePieceIndexes }) {
  return matches.map((match) => {
    let status = 'healthy';
    if (match.localFile === null) {
      status = 'missing';
    } else if (!match.isSizeMatching) {
      status = 'size_mismatch';
    } else if (
      badPieces.some((pieceIndex) => isPieceTouchingFile(layout, pieceIndex, match.torrentFile))
    ) {
      status = 'suspect';
    } else if (
      unverifiablePieceIndexes.some((pieceIndex) =>
        isPieceTouchingFile(layout, pieceIndex, match.torrentFile),
      )
    ) {
      status = 'unverifiable';
    }

    return {
      match,
      path: match.torrentFile.path,
      size: match.torrentFile.size,
      status,
      localPath: match.localPath,
    };
  });
}

function groupConsecutive(pieceIndexes) {
  const ranges = [];
  for (const pieceIndex of pieceIndexes) {
    const lastRange = ranges.at(-1);
    if (lastRange && lastRange[1] === pieceIndex - 1) {
      lastRange[1] = pieceIndex;
    } else {
      ranges.push([pieceIndex, pieceIndex]);
    }
  }

  return ranges;
}

export function toPieceRanges(badPieces) {
  return groupConsecutive(badPieces);
}

export function describePieceRanges(layout, { badPieces, zeroPieceIndexes }) {
  const zeroPieces = new Set(zeroPieceIndexes);
  const zeroRanges = groupConsecutive(badPieces.filter((pieceIndex) => zeroPieces.has(pieceIndex)));
  const mismatchRanges = groupConsecutive(
    badPieces.filter((pieceIndex) => !zeroPieces.has(pieceIndex)),
  );

  const describe =
    (kind) =>
    ([firstPiece, lastPiece]) => {
      const startOffset = firstPiece * layout.pieceLength;
      const endOffset = Math.min((lastPiece + 1) * layout.pieceLength, layout.totalSize);
      const files = layout.files
        .filter(
          (torrentFile) =>
            torrentFile.offset < endOffset && torrentFile.offset + torrentFile.size > startOffset,
        )
        .map((torrentFile) => torrentFile.path);

      return { kind, firstPiece, lastPiece, startOffset, endOffset, files };
    };

  return [...zeroRanges.map(describe('zero')), ...mismatchRanges.map(describe('mismatch'))].sort(
    (first, second) => first.firstPiece - second.firstPiece,
  );
}
