import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { computeOverallRatio, detectAnomalies, groupErrorLines } from './mkv-prescan.js';

const healthyTrack = { type: 'video', number: 1, packetCount: 100, zeroSizePackets: 0, lastTimestampSeconds: 59.9 };

describe('detectAnomalies', () => {
    it('neSignaleRienPourUnFichierComplet', () => {
        const valeurObtenue = detectAnomalies({ durationSeconds: 60, tracks: [healthyTrack], readError: null });

        assert.deepEqual(valeurObtenue, []);
    });

    it('signaleUnePisteQuiSArreteAvantLaFinDuConteneur', () => {
        const pisteTronquee = { ...healthyTrack, lastTimestampSeconds: 28 };

        const valeurObtenue = detectAnomalies({ durationSeconds: 60, tracks: [pisteTronquee], readError: null });

        assert.deepEqual(valeurObtenue.map((anomalie) => anomalie.code), ['track_ends_early']);
    });

    it('signaleUneErreurDeLectureEtLesPaquetsVides', () => {
        const pisteAvecPaquetsVides = { ...healthyTrack, zeroSizePackets: 3 };

        const valeurObtenue = detectAnomalies({ durationSeconds: 60, tracks: [pisteAvecPaquetsVides], readError: 'invalid EBML' });

        assert.deepEqual(valeurObtenue.map((anomalie) => anomalie.code), ['read_error', 'zero_size_packets']);
    });

    it('tolereUnEcartInferieurAuSeuil', () => {
        const pisteJusteEnDessous = { ...healthyTrack, lastTimestampSeconds: 58.5 };

        assert.deepEqual(detectAnomalies({ durationSeconds: 60, tracks: [pisteJusteEnDessous], readError: null }), []);
    });
});

describe('groupErrorLines', () => {
    it('regroupeLesLignesIdentiquesAvecLeurNombreDOccurrences', () => {
        const valeurObtenue = groupErrorLines(['a', 'b', 'a'], 10);

        assert.deepEqual(valeurObtenue, [['a', 2], ['b', 1]]);
    });

    it('limiteLeNombreDeMessagesDistincts', () => {
        const valeurObtenue = groupErrorLines(['a', 'b', 'c'], 2);

        assert.equal(valeurObtenue.length, 2);
    });
});

describe('computeOverallRatio', () => {
    it('faitLaMoyenneDesPistesLuesEnParallele', () => {
        assert.ok(Math.abs(computeOverallRatio([0.8, 0.2], 0) - 0.5) < 0.005);
    });

    it('neReculeJamais', () => {
        assert.equal(computeOverallRatio([0.1, 0.1], 0.6), 0.6);
    });

    it('plafonneChaquePisteA100Pourcent', () => {
        assert.ok(Math.abs(computeOverallRatio([3, 0], 0) - 0.5) < 0.005);
    });
});
