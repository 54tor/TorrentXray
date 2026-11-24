import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { entriesFromFileList, isMediaPath, isTorrentPath } from './dropped-files.js';

describe('dropped-files', () => {
    it('reconnaitLesExtensionsSansTenirCompteDeLaCasse', () => {
        assert.equal(isMediaPath('Dossier/Film.MKV'), true);
        assert.equal(isMediaPath('Dossier/film.nfo'), false);
        assert.equal(isTorrentPath('a.TORRENT'), true);
    });

    it('utiliseLeCheminRelatifDuSelecteurDeDossier', () => {
        const fichierDansDossier = Object.assign(new File(['x'], 'e01.mkv'), { webkitRelativePath: 'Saison/e01.mkv' });
        const fichierSeul = new File(['y'], 'seul.mkv');

        const valeurObtenue = entriesFromFileList([fichierDansDossier, fichierSeul]);

        assert.deepEqual(valeurObtenue.map((entree) => entree.path), ['Saison/e01.mkv', 'seul.mkv']);
    });
});
