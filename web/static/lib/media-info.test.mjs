import assert from 'node:assert/strict';
import { test } from 'node:test';

import { readMediaInfoText } from './media-info.js';

function fakeLibraryUrl(analyzeDataSource) {
  const source = `
    export default async function mediaInfoFactory(options) {
      globalThis.receivedOptions = options;
      return {
        analyzeData: ${analyzeDataSource},
        close() { globalThis.wasClosed = true; },
      };
    }`;
  return `data:text/javascript;base64,${Buffer.from(source).toString('base64')}`;
}

test('returns the text produced by MediaInfo and reads the file by chunks', async () => {
  const file = new Blob([new Uint8Array([1, 2, 3, 4, 5])]);
  const libraryUrl = fakeLibraryUrl(`async (size, readChunk) => {
    const chunk = await readChunk(2, 3);
    return 'size=' + size + ' chunk=' + Array.from(chunk).join(',');
  }`);

  const valeurObtenue = await readMediaInfoText(file, { libraryUrl, wasmUrl: 'http://x/m.wasm' });

  assert.equal(valeurObtenue, 'size=5 chunk=4,5');
  assert.equal(globalThis.receivedOptions.format, 'text');
  assert.equal(globalThis.receivedOptions.locateFile(), 'http://x/m.wasm');
  assert.equal(globalThis.wasClosed, true);
});

test('falls back to a message and still closes MediaInfo when analysis throws', async () => {
  globalThis.wasClosed = false;
  const libraryUrl = fakeLibraryUrl('async () => { throw new Error("boom"); }');

  const valeurObtenue = await readMediaInfoText(new Blob([]), { libraryUrl, wasmUrl: 'x' });

  assert.match(valeurObtenue, /indisponible/);
  assert.equal(globalThis.wasClosed, true);
});
