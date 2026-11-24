import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { decode, infoHashFromTorrent } from './bencode.js';

const textEncoder = new TextEncoder();
const infoNonCanonique = 'd4:name4:film12:piece lengthi512e6:lengthi3ee';
const torrentNonCanonique = textEncoder.encode(`d8:announce3:url4:info${infoNonCanonique}e`);

describe('infoHashFromTorrent', () => {
  it('hacheLesOctetsBrutsDuDictionnaireInfoSansLesReordonner', async () => {
    const valeurAttendue = createHash('sha1').update(infoNonCanonique).digest('hex');

    const valeurObtenue = await infoHashFromTorrent(torrentNonCanonique);

    assert.equal(valeurObtenue, valeurAttendue);
  });

  it('rejetteUnBencodeTronque', async () => {
    await assert.rejects(infoHashFromTorrent(torrentNonCanonique.subarray(0, 20)));
  });
});

describe('decode', () => {
  it('decodeLesDictionnairesListesEntiersEtChaines', () => {
    const valeurObtenue = decode(textEncoder.encode('d1:ali1ei-2ee1:b3:xyze'));

    assert.deepEqual(valeurObtenue.get('a'), [1, -2]);
    assert.equal(new TextDecoder().decode(valeurObtenue.get('b')), 'xyz');
  });
});
