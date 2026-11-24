const READ_FAILED_MESSAGE = 'Fiche technique indisponible pour ce fichier.';

export async function readMediaInfoText(file, { libraryUrl, wasmUrl }) {
  const { default: mediaInfoFactory } = await import(libraryUrl);
  const mediaInfo = await mediaInfoFactory({ format: 'text', locateFile: () => wasmUrl });
  try {
    return await mediaInfo.analyzeData(
      file.size,
      async (chunkSize, offset) =>
        new Uint8Array(await file.slice(offset, offset + chunkSize).arrayBuffer()),
    );
  } catch {
    return READ_FAILED_MESSAGE;
  } finally {
    mediaInfo.close();
  }
}
