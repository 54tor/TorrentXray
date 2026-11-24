const C_INT = 0x69;
const C_LIST = 0x6c;
const C_DICT = 0x64;
const C_END = 0x65;
const C_COLON = 0x3a;
const C_0 = 0x30;
const C_9 = 0x39;

export async function infoHashFromTorrent(bytes) {
    const span = findInfoSpan(bytes);
    if (span === null) {
        throw new Error('Torrent sans dictionnaire info.');
    }
    const info = bytes.slice(span[0], span[1]);
    const digest = await crypto.subtle.digest('SHA-1', info);
    return toHex(new Uint8Array(digest));
}

// The info hash is the SHA-1 of the raw info bytes: re-encoding would reorder keys of non-canonical torrents.
function findInfoSpan(bytes) {
    const len = bytes.length;
    let pos = 0;
    let infoSpan = null;

    const readStringSpan = () => {
        const start = pos;
        while (pos < len && bytes[pos] >= C_0 && bytes[pos] <= C_9) {
            pos++;
        }
        if (pos >= len || bytes[pos] !== C_COLON) {
            throw new Error('Chaîne sans longueur.');
        }
        const n = Number.parseInt(asciiOf(bytes, start, pos), 10);
        pos++;
        const s = pos;
        pos += n;
        if (pos > len) {
            throw new Error('Chaîne tronquée.');
        }
        return [s, pos];
    };

    const skipValue = (depth) => {
        if (depth > 100) {
            throw new Error('Bencode trop imbriqué.');
        }
        if (pos >= len) {
            throw new Error('Bencode tronqué.');
        }
        const c = bytes[pos];
        if (c === C_INT) {
            pos++;
            while (pos < len && bytes[pos] !== C_END) {
                pos++;
            }
            if (pos >= len) {
                throw new Error('Entier non terminé.');
            }
            pos++;
        } else if (c === C_LIST) {
            pos++;
            while (pos < len && bytes[pos] !== C_END) {
                skipValue(depth + 1);
            }
            if (pos >= len) {
                throw new Error('Liste non terminée.');
            }
            pos++;
        } else if (c === C_DICT) {
            readDict(depth);
        } else if (c >= C_0 && c <= C_9) {
            readStringSpan();
        } else {
            throw new Error('Octet inattendu.');
        }
    };

    const readDict = (depth) => {
        const isRoot = depth === 0;
        pos++;
        while (pos < len && bytes[pos] !== C_END) {
            if (bytes[pos] < C_0 || bytes[pos] > C_9) {
                throw new Error('Clé de dictionnaire non-chaîne.');
            }
            const [ks, ke] = readStringSpan();
            const valStart = pos;
            skipValue(depth + 1);
            if (isRoot && infoSpan === null && ke - ks === 4 && isInfoKey(bytes, ks)) {
                infoSpan = [valStart, pos];
            }
        }
        if (pos >= len) {
            throw new Error('Dictionnaire non terminé.');
        }
        pos++;
    };

    if (len === 0 || bytes[0] !== C_DICT) {
        throw new Error('Racine non-dictionnaire.');
    }
    readDict(0);

    return infoSpan;
}

function isInfoKey(bytes, at) {
    return bytes[at] === 0x69 && bytes[at + 1] === 0x6e
        && bytes[at + 2] === 0x66 && bytes[at + 3] === 0x6f;
}

function asciiOf(bytes, start, end) {
    let s = '';
    for (let i = start; i < end; i++) {
        s += String.fromCharCode(bytes[i]);
    }
    return s;
}

function toHex(u8) {
    let s = '';
    for (let i = 0; i < u8.length; i++) {
        s += u8[i].toString(16).padStart(2, '0');
    }
    return s;
}

export function decode(bytes) {
    let pos = 0;
    const len = bytes.length;

    const readInt = (terminator) => {
        const start = pos;
        while (pos < len && bytes[pos] !== terminator) {
            pos++;
        }
        if (pos >= len) {
            throw new Error('Entier non terminé.');
        }
        const text = latin1(bytes.subarray(start, pos));
        pos++;
        if (!/^-?\d+$/.test(text)) {
            throw new Error('Entier invalide.');
        }

        return Number(text);
    };

    const readString = () => {
        const n = readInt(C_COLON);
        if (n < 0 || pos + n > len) {
            throw new Error('Chaîne tronquée.');
        }
        const s = bytes.subarray(pos, pos + n);
        pos += n;

        return s;
    };

    const readValue = (depth) => {
        if (depth > 100) {
            throw new Error('Bencode trop imbriqué.');
        }
        if (pos >= len) {
            throw new Error('Bencode tronqué.');
        }
        const c = bytes[pos];
        if (c === C_INT) {
            pos++;

            return readInt(C_END);
        }
        if (c === C_LIST) {
            pos++;
            const list = [];
            while (pos < len && bytes[pos] !== C_END) {
                list.push(readValue(depth + 1));
            }
            if (pos >= len) {
                throw new Error('Liste non terminée.');
            }
            pos++;

            return list;
        }
        if (c === C_DICT) {
            pos++;
            const dict = new Map();
            while (pos < len && bytes[pos] !== C_END) {
                if (bytes[pos] < C_0 || bytes[pos] > C_9) {
                    throw new Error('Clé de dictionnaire non-chaîne.');
                }
                dict.set(latin1(readString()), readValue(depth + 1));
            }
            if (pos >= len) {
                throw new Error('Dictionnaire non terminé.');
            }
            pos++;

            return dict;
        }
        if (c >= C_0 && c <= C_9) {
            return readString();
        }
        throw new Error('Octet inattendu.');
    };

    return readValue(0);
}

function latin1(bytes) {
    let s = '';
    for (const b of bytes) {
        s += String.fromCharCode(b);
    }

    return s;
}
