import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { describeProgress, formatBytes, formatDuration } from './progress-tracker.js';

describe('progress-tracker', () => {
    it('formateUneDureeAvecEtSansHeures', () => {
        assert.equal(formatDuration(65), '1:05');
        assert.equal(formatDuration(3725), '1:02:05');
        assert.equal(formatDuration(-3), '0:00');
    });

    it('formateLesOctetsDansLUniteAdaptee', () => {
        assert.equal(formatBytes(512), '512 o');
        assert.equal(formatBytes(6.03 * 1024 ** 3), '6.03 Go');
    });

    it('estimeLeTempsRestantEtLeDebitApresUnDebutDeProgression', () => {
        const valeurObtenue = describeProgress({ ratio: 0.25, totalBytes: 4000, stageElapsedSeconds: 10 });

        assert.equal(valeurObtenue.percent, 25);
        assert.equal(valeurObtenue.bytesPerSecond, 100);
        assert.equal(valeurObtenue.remainingSeconds, 30);
    });

    it('nEstimeRienTantQueLaProgressionEstTropFaible', () => {
        const valeurObtenue = describeProgress({ ratio: 0.001, totalBytes: 4000, stageElapsedSeconds: 10 });

        assert.equal(valeurObtenue.remainingSeconds, null);
        assert.equal(valeurObtenue.bytesPerSecond, null);
    });
});
