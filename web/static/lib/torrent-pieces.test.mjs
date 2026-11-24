import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { classifyFiles, describePieceRanges, matchLocalFiles, parseTorrentLayout, toPieceRanges, verifyPieces } from './torrent-pieces.js';

const PIECE_LENGTH = 512;
const textEncoder = new TextEncoder();

function bencodeValue(value) {
    if (typeof value === 'number') {
        return textEncoder.encode(`i${value}e`);
    }
    if (typeof value === 'string') {
        const bytes = textEncoder.encode(value);
        return Buffer.concat([textEncoder.encode(`${bytes.length}:`), bytes]);
    }
    if (value instanceof Uint8Array) {
        return Buffer.concat([textEncoder.encode(`${value.length}:`), value]);
    }
    if (Array.isArray(value)) {
        return Buffer.concat([textEncoder.encode('l'), ...value.map(bencodeValue), textEncoder.encode('e')]);
    }
    const sortedKeys = Object.keys(value).sort();
    return Buffer.concat([textEncoder.encode('d'), ...sortedKeys.flatMap((key) => [bencodeValue(key), bencodeValue(value[key])]), textEncoder.encode('e')]);
}

function contentBytes(length, seed) {
    return Uint8Array.from({ length }, (_, position) => (position * 7 + seed) % 256);
}

function buildTorrent(rootName, files) {
    const wholeContent = Buffer.concat(files.map((file) => file.bytes));
    const pieceHashes = [];
    for (let start = 0; start < wholeContent.length; start += PIECE_LENGTH) {
        pieceHashes.push(createHash('sha1').update(wholeContent.subarray(start, start + PIECE_LENGTH)).digest());
    }

    return new Uint8Array(bencodeValue({
        info: {
            name: rootName,
            'piece length': PIECE_LENGTH,
            pieces: Buffer.concat(pieceHashes),
            files: files.map((file) => ({ length: file.bytes.length, path: file.path.split('/') })),
        },
    }));
}

const episodeOne = { path: 'e01.mkv', bytes: contentBytes(1000, 1) };
const episodeTwo = { path: 'sub/e02.mkv', bytes: contentBytes(700, 2) };

function localEntries(files, rootName) {
    return files.map((file) => ({ path: `${rootName}/${file.path}`, file: new File([file.bytes], file.path.split('/').pop()) }));
}

describe('verification des pieces d un torrent', () => {
    it('apparieParLeCheminCompletQuandLeDossierRacineEstIdentique', async () => {
        const layout = await parseTorrentLayout(buildTorrent('Saison', [episodeOne, episodeTwo]));

        const matches = matchLocalFiles(layout, localEntries([episodeOne, episodeTwo], 'Saison'));

        assert.deepEqual(matches.map((match) => match.matchKind), ['full_path', 'full_path']);
    });

    it('neSignaleRienQuandLeContenuCorrespondAuTorrent', async () => {
        const layout = await parseTorrentLayout(buildTorrent('Saison', [episodeOne, episodeTwo]));
        const matches = matchLocalFiles(layout, localEntries([episodeOne, episodeTwo], 'Saison'));

        const valeurObtenue = await verifyPieces(layout, matches);

        assert.deepEqual(valeurObtenue.badPieces, []);
        assert.deepEqual(classifyFiles(layout, matches, valeurObtenue).map((file) => file.status), ['healthy', 'healthy']);
    });

    it('designeLeFichierCorrompuEtIgnoreLesAutres', async () => {
        const corruptedBytes = Uint8Array.from(episodeTwo.bytes);
        corruptedBytes[300] ^= 0xff;
        const layout = await parseTorrentLayout(buildTorrent('Saison', [episodeOne, episodeTwo]));
        const matches = matchLocalFiles(layout, localEntries([episodeOne, { ...episodeTwo, bytes: corruptedBytes }], 'Saison'));

        const verification = await verifyPieces(layout, matches);
        const statusByPath = Object.fromEntries(classifyFiles(layout, matches, verification).map((file) => [file.path, file.status]));

        assert.ok(verification.badPieces.length > 0);
        assert.equal(statusByPath['sub/e02.mkv'], 'suspect');
    });

    it('retrouveLesFichiersMemeSiLeDossierRacineAEteRenomme', async () => {
        const layout = await parseTorrentLayout(buildTorrent('Saison', [episodeOne, episodeTwo]));

        const matches = matchLocalFiles(layout, localEntries([episodeOne, episodeTwo], 'Autre dossier'));

        assert.equal(matches.every((match) => match.localFile !== null && match.isSizeMatching), true);
        assert.deepEqual(matches.map((match) => match.matchKind), ['relative_path', 'relative_path']);
    });

    it('compteLesPiecesNonVerifiablesQuandUnFichierManque', async () => {
        const layout = await parseTorrentLayout(buildTorrent('Saison', [episodeOne, episodeTwo]));
        const matches = matchLocalFiles(layout, localEntries([episodeOne], 'Saison'));

        const valeurObtenue = await verifyPieces(layout, matches);

        const statusByPath = Object.fromEntries(classifyFiles(layout, matches, valeurObtenue).map((file) => [file.path, file.status]));

        assert.ok(valeurObtenue.unverifiablePieceIndexes.length > 0);
        assert.equal(statusByPath['sub/e02.mkv'], 'missing');
    });

    it('signaleLesPiecesNullesDuTorrentMemeSiLeFichierLocalCorrespond', async () => {
        const truncatedOrigin = { path: 'film.mkv', bytes: new Uint8Array(1500).fill(0, 0) };
        truncatedOrigin.bytes.set(contentBytes(500, 3), 0);
        const layout = await parseTorrentLayout(buildTorrent('Film', [truncatedOrigin]));
        const matches = matchLocalFiles(layout, localEntries([truncatedOrigin], 'Film'));

        const verification = await verifyPieces(layout, matches);
        const statusByPath = Object.fromEntries(classifyFiles(layout, matches, verification).map((file) => [file.path, file.status]));

        assert.deepEqual(verification.zeroPieceIndexes, [1, 2]);
        assert.deepEqual(verification.badPieces, [1, 2]);
        assert.equal(statusByPath['film.mkv'], 'suspect');
    });

    it('decritLesPlagesDePiecesAvecLeurNatureLesOctetsEtLesFichiersTouches', async () => {
        const layout = await parseTorrentLayout(buildTorrent('Saison', [episodeOne, episodeTwo]));

        const valeurObtenue = describePieceRanges(layout, { badPieces: [0, 1, 2], zeroPieceIndexes: [2] });

        assert.deepEqual(valeurObtenue, [
            { kind: 'mismatch', firstPiece: 0, lastPiece: 1, startOffset: 0, endOffset: 1024, files: ['e01.mkv', 'sub/e02.mkv'] },
            { kind: 'zero', firstPiece: 2, lastPiece: 2, startOffset: 1024, endOffset: 1536, files: ['sub/e02.mkv'] },
        ]);
    });

    it('regroupeLesPiecesConsecutivesEnPlages', () => {
        assert.deepEqual(toPieceRanges([3, 4, 5, 9, 11, 12]), [[3, 5], [9, 9], [11, 12]]);
    });
});

describe('verification limitee a certaines pieces', () => {
    it('neVerifieQueLesPiecesDemandeesQuandPieceIndexesEstFourni', async () => {
        const corruptedBytes = Uint8Array.from(episodeTwo.bytes);
        corruptedBytes[300] ^= 0xff; // offset 1300 dans le torrent : piece 2
        const layout = await parseTorrentLayout(buildTorrent('Saison', [episodeOne, episodeTwo]));
        const matches = matchLocalFiles(layout, localEntries([episodeOne, { ...episodeTwo, bytes: corruptedBytes }], 'Saison'));

        const piecesHorsZoneCorrompue = await verifyPieces(layout, matches, { pieceIndexes: [0, 1] });
        const piecesDeLaZoneCorrompue = await verifyPieces(layout, matches, { pieceIndexes: [2] });

        assert.deepEqual(piecesHorsZoneCorrompue.badPieces, []);
        assert.deepEqual(piecesDeLaZoneCorrompue.badPieces, [2]);
    });
});
