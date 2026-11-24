const MEDIA_EXTENSIONS = ['.mkv', '.mka', '.webm'];
const TORRENT_EXTENSION = '.torrent';

function hasExtension(path, extensions) {
  const lowerCasePath = path.toLowerCase();

  return extensions.some((extension) => lowerCasePath.endsWith(extension));
}

export function isMediaPath(path) {
  return hasExtension(path, MEDIA_EXTENSIONS);
}

export function isTorrentPath(path) {
  return hasExtension(path, [TORRENT_EXTENSION]);
}

function readAllDirectoryEntries(directoryReader) {
  return new Promise((resolve, reject) => {
    const collected = [];
    const readNextBatch = () =>
      directoryReader.readEntries((batch) => {
        if (batch.length === 0) {
          resolve(collected);
          return;
        }
        collected.push(...batch);
        readNextBatch();
      }, reject);
    readNextBatch();
  });
}

async function walkEntry(entry, found) {
  if (entry.isFile) {
    const file = await new Promise((resolve, reject) => entry.file(resolve, reject));
    found.push({ path: entry.fullPath.replace(/^\//, ''), file });
    return;
  }

  for (const childEntry of await readAllDirectoryEntries(entry.createReader())) {
    await walkEntry(childEntry, found);
  }
}

export async function collectDroppedEntries(dataTransfer) {
  // webkitGetAsEntry must be called synchronously, before any await.
  const rootEntries = [...dataTransfer.items]
    .map((item) => item.webkitGetAsEntry?.())
    .filter(Boolean);
  if (rootEntries.length === 0) {
    return entriesFromFileList(dataTransfer.files);
  }
  const found = [];
  for (const rootEntry of rootEntries) {
    await walkEntry(rootEntry, found);
  }

  return found;
}

export function entriesFromFileList(fileList) {
  return [...fileList].map((file) => ({ path: file.webkitRelativePath || file.name, file }));
}
