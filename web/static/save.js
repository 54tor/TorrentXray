import './vendor/altcha/altcha.min.js';

const SHA_WORKER_URL = '/static/vendor/altcha/sha.js';
const REPORTS_URL = '/api/reports';
const PROOF_HEADER = 'X-Altcha';
const SOLVED_ALGORITHMS = ['SHA-256', 'SHA-384', 'SHA-512'];
const COPY_FEEDBACK_MILLISECONDS = 2000;

const ERROR_MESSAGES = {
    invalid_json: 'Le serveur a refusé les constats (format invalide).',
    invalid_request: 'Requête refusée par le serveur.',
    proof_of_work: 'La preuve de travail a été refusée. Réessayez.',
    too_large: 'Le rapport dépasse la taille maximale acceptée (256 Kio).',
    rate_limited: 'Trop de demandes depuis votre adresse. Réessayez dans une minute.',
    storage_full: 'Le stockage des rapports est plein. Réessayez plus tard.',
};
const LABELS = {
    solving: 'Calcul de la preuve de travail…',
    sending: 'Envoi des constats…',
    saved: 'Rapport enregistré.',
    proofFailed: 'Impossible de calculer la preuve de travail.',
    unknownError: "Échec de l'enregistrement :",
    copied: 'Copié',
    copy: 'Copier',
};

for (const algorithm of SOLVED_ALGORITHMS) {
    globalThis.$altcha.algorithms.set(algorithm, () => new Worker(SHA_WORKER_URL));
}

const elements = {
    button: document.getElementById('save-button'),
    status: document.getElementById('save-status'),
    widget: document.getElementById('proof-widget'),
    result: document.getElementById('save-result'),
    permalink: document.getElementById('permalink'),
    expiry: document.getElementById('expiry'),
    markdownLink: document.getElementById('markdown-link'),
    bbcodeLink: document.getElementById('bbcode-link'),
};

const expiryFormat = new Intl.DateTimeFormat('fr-FR', { dateStyle: 'long', timeStyle: 'short' });

export function setupReportSaving() {
    let pendingFindings = null;
    elements.button.addEventListener('click', () => save(pendingFindings));
    for (const copyButton of document.querySelectorAll('[data-copy-target]')) {
        copyButton.addEventListener('click', () => copyLink(copyButton));
    }
    return {
        offer(findings) {
            pendingFindings = findings;
            elements.result.hidden = true;
            elements.status.textContent = '';
            elements.button.disabled = false;
        },
        reset() {
            pendingFindings = null;
            elements.button.disabled = true;
            elements.result.hidden = true;
            elements.status.textContent = '';
        },
    };
}

async function save(findings) {
    if (findings === null) {
        return;
    }
    elements.button.disabled = true;
    try {
        const created = await postFindings(findings, await solveProofOfWork());
        showCreated(created);
    } catch (failure) {
        elements.status.textContent = failure.message;
        elements.button.disabled = false;
    }
}

async function solveProofOfWork() {
    elements.status.textContent = LABELS.solving;
    elements.widget.reset();
    const solved = await elements.widget.verify();
    if (!solved?.payload) {
        throw new Error(LABELS.proofFailed);
    }
    return solved.payload;
}

async function postFindings(findings, proof) {
    elements.status.textContent = LABELS.sending;
    const response = await fetch(REPORTS_URL, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', [PROOF_HEADER]: proof },
        body: JSON.stringify(findings),
    });
    const body = await response.json().catch(() => ({}));
    if (!response.ok) {
        throw new Error(ERROR_MESSAGES[body.error] ?? `${LABELS.unknownError} HTTP ${response.status}`);
    }
    return body;
}

function showCreated(created) {
    const permalink = new URL(created.url, location.href).href;
    setLink(elements.permalink, permalink);
    setLink(elements.markdownLink, `${permalink}.md`);
    setLink(elements.bbcodeLink, `${permalink}.bb`);
    elements.expiry.dateTime = created.expiresAt;
    elements.expiry.textContent = expiryFormat.format(new Date(created.expiresAt));
    elements.result.hidden = false;
    elements.status.textContent = LABELS.saved;
}

function setLink(anchor, url) {
    anchor.href = url;
    anchor.textContent = url;
}

async function copyLink(copyButton) {
    const anchor = document.getElementById(copyButton.dataset.copyTarget);
    await navigator.clipboard.writeText(anchor.href);
    copyButton.textContent = LABELS.copied;
    setTimeout(() => {
        copyButton.textContent = LABELS.copy;
    }, COPY_FEEDBACK_MILLISECONDS);
}
