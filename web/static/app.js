import { collectDroppedEntries, entriesFromFileList, isMediaPath, isTorrentPath } from './lib/dropped-files.js';
import { MAX_MEDIA_FILES } from './lib/fit-findings.js';
import { groupErrorLines, prescanMkv } from './lib/mkv-prescan.js';
import { describeProgress, formatBytes, formatDuration } from './lib/progress-tracker.js';
import { classifyFiles, describePieceRanges, matchLocalFiles, parseTorrentLayout, verifyPieces } from './lib/torrent-pieces.js';
import { setupReportSaving } from './save.js';

const LIBRARY_URL = '/static/vendor/ffmpeg/index.js';
const CORE_URL = '/static/vendor/ffmpeg/ffmpeg-core.js';
const WASM_URL = '/static/vendor/ffmpeg/ffmpeg-core.wasm';
const MEDIABUNNY_URL = '/static/vendor/ffmpeg/mediabunny.min.js';
const VERSIONS_URL = '/static/vendor/ffmpeg/versions.json';
const MOUNT_DIRECTORY = '/input';
const STAGED_FILE_NAME = 'input.mkv';
const PROGRESS_REFRESH_MILLISECONDS = 500;
const STALL_SECONDS = 3;
const MILLISECONDS_PER_SECOND = 1000;
// Emitted by the emscripten runtime when ffmpeg exits, even on a healthy file.
const RUNTIME_EXIT_MESSAGE = 'Aborted()';
const MAX_DISTINCT_REPORT_LINES = 500;
const ANALYSED_STATUSES = ['suspect', 'size_mismatch'];
const PROBLEM_STATUSES = ['suspect', 'size_mismatch', 'missing', 'unverifiable'];
const FFMPEG_ARGUMENTS_BY_MODE = {
    structure: ['-map', '0:v?', '-map', '0:a?', '-c', 'copy'],
    decode: ['-map', '0:v?', '-map', '0:a?'],
};

const LABELS = {
    selectionFiles: 'Fichiers :',
    needFiles: 'Sélectionnez au moins un fichier ou un dossier.',
    tooManyFiles: `Au plus ${MAX_MEDIA_FILES} fichiers média par analyse : retirez des fichiers ou analysez par lots.`,
    noMedia: 'Aucun fichier MKV, MKA ou WebM dans la sélection.',
    torrentCheck: 'Vérification SHA-1 des pièces du .torrent…',
    fileStage: 'Fichier',
    prescan: 'Analyse préliminaire du conteneur…',
    loading: 'Chargement de ffmpeg.wasm…',
    running: 'Analyse ffmpeg en cours…',
    stepTorrent: 'Vérification SHA-1 des pièces du .torrent',
    stepPrescan: 'Analyse préliminaire du conteneur',
    stepFfmpeg: 'Analyse ffmpeg',
    cancelled: 'Analyse annulée.',
    failed: "L'analyse a échoué :",
    resultOk: 'Analyse terminée : aucune erreur détectée.',
    resultErrors: 'Analyse terminée : des problèmes ont été détectés.',
    verdictOk: 'Aucun problème',
    verdictErrors: 'Problèmes détectés',
    unsupported: 'Votre navigateur ne gère pas WebAssembly : cette vérification est impossible ici. Utilisez une version récente de Firefox, Chrome, Edge ou Safari.',
    elapsed: 'écoulé',
    remaining: 'restant',
    packetsRead: 'paquets lus',
    remove: 'Retirer',
};

const elements = {
    dropzone: document.getElementById('dropzone'),
    filesInput: document.getElementById('files-input'),
    folderInput: document.getElementById('folder-input'),
    torrentInput: document.getElementById('torrent-input'),
    selection: document.getElementById('selection'),
    selectionSummary: document.getElementById('selection-summary'),
    selectionList: document.getElementById('selection-list'),
    torrentSlot: document.getElementById('torrent-slot'),
    torrentList: document.getElementById('torrent-list'),
    modeInputs: [...document.querySelectorAll('input[name="mode"]')],
    startButton: document.getElementById('start-button'),
    cancelButton: document.getElementById('cancel-button'),
    progressPanel: document.getElementById('progress-panel'),
    busyStatus: document.getElementById('busy-status'),
    progressBar: document.getElementById('progress-bar'),
    progressDetails: document.getElementById('progress-details'),
    stallHint: document.getElementById('stall-hint'),
    stepList: document.getElementById('step-list'),
    statusBox: document.getElementById('status-box'),
    status: document.getElementById('status'),
    verdict: document.getElementById('verdict'),
};

const state = {
    selectedEntries: [],
    torrentEntry: null,
    steps: [],
    wasCancelled: false,
    ffmpeg: null,
    versionFfmpeg: null,
    startedAt: null,
    stage: { startedAt: 0, lastActivityAt: 0, ratio: null, totalBytes: 0, packetCount: 0, isProgressing: false },
    progressTimer: null,
};

const reportSaving = setupReportSaving();

function selectedMode() {
    return elements.modeInputs.find((modeInput) => modeInput.checked).value;
}

function totalSelectedBytes() {
    return state.selectedEntries.reduce((sum, entry) => sum + entry.file.size, 0);
}

function addEntries(entries) {
    for (const entry of entries) {
        if (isTorrentPath(entry.path)) {
            state.torrentEntry = entry;
            continue;
        }
        const isAlreadySelected = state.selectedEntries.some((selected) => selected.path === entry.path);
        if (!isAlreadySelected) {
            state.selectedEntries.push(entry);
        }
    }
    renderSelection();
}

function removeEntry(kind, entryIndex) {
    if (kind === 'torrent') {
        state.torrentEntry = null;
    } else {
        state.selectedEntries.splice(entryIndex, 1);
    }
    renderSelection();
}

function renderSelection() {
    elements.selection.hidden = state.selectedEntries.length === 0;
    elements.selectionSummary.textContent = `${LABELS.selectionFiles} ${state.selectedEntries.length} (${formatBytes(totalSelectedBytes())})`;
    elements.selectionList.replaceChildren(
        ...state.selectedEntries.map((entry, entryIndex) => buildSelectionRow(entry.path, entry.file.size, 'file', entryIndex)),
    );
    updateFileLimit();
    elements.torrentSlot.hidden = state.torrentEntry === null;
    const torrentRows = state.torrentEntry ? [buildSelectionRow(state.torrentEntry.file.name, state.torrentEntry.file.size, 'torrent', 0)] : [];
    elements.torrentList.replaceChildren(...torrentRows);
}

function updateFileLimit() {
    const mediaCount = state.torrentEntry === null ? state.selectedEntries.filter((entry) => isMediaPath(entry.path)).length : 0;
    const isOverLimit = mediaCount > MAX_MEDIA_FILES;
    elements.startButton.disabled = isOverLimit || !isWebAssemblySupported();
    if (isOverLimit) {
        showFinished(LABELS.tooManyFiles, 'error');
    }
}

function buildSelectionRow(name, size, kind, entryIndex) {
    const row = document.createElement('li');
    const nameElement = document.createElement('span');
    nameElement.className = 'selection-name';
    nameElement.textContent = name;
    const sizeElement = document.createElement('span');
    sizeElement.className = 'muted';
    sizeElement.textContent = formatBytes(size);
    const removeButton = document.createElement('button');
    removeButton.type = 'button';
    removeButton.className = 'remove-button';
    removeButton.textContent = '×';
    removeButton.setAttribute('aria-label', `${LABELS.remove} ${name}`);
    removeButton.addEventListener('click', () => removeEntry(kind, entryIndex));
    row.append(nameElement, sizeElement, removeButton);
    return row;
}

async function start() {
    if (state.selectedEntries.length === 0) {
        showFinished(LABELS.needFiles, 'error');
        return;
    }
    prepareRun();
    try {
        const findings = await runAnalysis();
        showVerdict(findings);
        reportSaving.offer(findings);
    } catch (failure) {
        showFailure(failure);
    } finally {
        clearInterval(state.progressTimer);
        toggleRunning(false);
    }
}

function prepareRun() {
    state.wasCancelled = false;
    state.steps = [];
    state.startedAt = new Date();
    reportSaving.reset();
    renderSteps();
    elements.verdict.hidden = true;
    toggleRunning(true);
    state.progressTimer = setInterval(renderProgressDetails, PROGRESS_REFRESH_MILLISECONDS);
}

async function runAnalysis() {
    if (state.torrentEntry) {
        planStep('torrent', LABELS.stepTorrent);
    }
    const torrent = state.torrentEntry ? await verifyTorrent() : null;
    throwIfCancelled();
    const targets = selectFilesToAnalyse(torrent);
    if (targets.length === 0 && torrent === null) {
        throw new Error(LABELS.noMedia);
    }
    if (targets.length > MAX_MEDIA_FILES) {
        throw new Error(LABELS.tooManyFiles);
    }
    for (const [targetIndex, target] of targets.entries()) {
        planStep(`prescan:${targetIndex}`, `${target.path} — ${LABELS.stepPrescan}`);
        planStep(`ffmpeg:${targetIndex}`, `${target.path} — ${LABELS.stepFfmpeg}`);
    }
    const checks = [];
    for (const [targetIndex, target] of targets.entries()) {
        const stagePrefix = `${LABELS.fileStage} ${targetIndex + 1}/${targets.length} — ${target.path}`;
        checks.push(await analyseFile(target, stagePrefix, targetIndex));
    }
    return buildFindings(torrent?.summary ?? null, checks);
}

async function buildFindings(torrentSummary, checks) {
    const toolVersions = await (await fetch(VERSIONS_URL)).json();
    return {
        mode: selectedMode(),
        startedAt: state.startedAt.toISOString(),
        elapsedSeconds: Math.round((Date.now() - state.startedAt) / MILLISECONDS_PER_SECOND),
        torrent: torrentSummary,
        checks,
        versions: toolVersions,
        userAgent: navigator.userAgent,
    };
}

async function verifyTorrent() {
    beginStage('torrent', LABELS.torrentCheck, totalSelectedBytes());
    const layout = await parseTorrentLayout(new Uint8Array(await state.torrentEntry.file.arrayBuffer()));
    const matches = matchLocalFiles(layout, state.selectedEntries);
    const verification = await verifyPieces(layout, matches, {
        onProgress: (ratio) => setDeterminateProgress(ratio),
        isCancelled: () => state.wasCancelled,
    });
    const classifiedFiles = classifyFiles(layout, matches, verification);
    setStepState('torrent', 'done');
    return { classifiedFiles, summary: torrentSummary(layout, verification, classifiedFiles) };
}

function torrentSummary(layout, verification, classifiedFiles) {
    return {
        name: layout.name,
        infoHash: layout.infoHash,
        pieceLength: layout.pieceLength,
        pieceCount: layout.pieceCount,
        totalSize: layout.totalSize,
        unverifiablePieces: verification.unverifiablePieceIndexes.length,
        badPieceCount: verification.badPieces.length,
        zeroPieceCount: verification.zeroPieceIndexes.length,
        pieceRanges: describePieceRanges(layout, verification),
        files: classifiedFiles.map(({ path, size, status, localPath }) => ({ path, size, status, localPath })),
    };
}

function selectFilesToAnalyse(torrent) {
    if (torrent === null) {
        return state.selectedEntries.filter((entry) => isMediaPath(entry.path));
    }
    return torrent.classifiedFiles
        .filter((classified) => ANALYSED_STATUSES.includes(classified.status) && classified.match.localFile !== null && isMediaPath(classified.path))
        .map((classified) => ({
            path: classified.path,
            file: classified.match.localFile,
            torrentMatch: {
                torrentPath: classified.path,
                torrentSize: classified.size,
                localPath: classified.match.localPath,
                localSize: classified.match.localFile.size,
                matchKind: classified.match.matchKind,
            },
        }));
}

async function analyseFile(target, stagePrefix, targetIndex) {
    const file = target.file;
    beginStage(`prescan:${targetIndex}`, `${stagePrefix} — ${LABELS.prescan}`, file.size);
    const prescan = await prescanMkv(file, {
        libraryUrl: absoluteUrl(MEDIABUNNY_URL),
        onProgress: ({ ratio, packetCount }) => {
            throwIfCancelled();
            state.stage.packetCount = packetCount;
            setDeterminateProgress(ratio);
        },
    });
    throwIfCancelled();
    setStepState(`prescan:${targetIndex}`, 'done');
    beginStage(`ffmpeg:${targetIndex}`, `${stagePrefix} — ${LABELS.loading}`, file.size);
    const ffmpegResult = await runFfmpeg(file, `${stagePrefix} — ${LABELS.running}`);
    setStepState(`ffmpeg:${targetIndex}`, 'done');
    return { file: { name: file.name, size: file.size }, torrentMatch: target.torrentMatch ?? null, prescan, ffmpeg: ffmpegResult };
}

async function runFfmpeg(file, runningMessage) {
    const versionLines = await readFfmpegVersion();
    throwIfCancelled();
    const logLines = [];
    state.ffmpeg = await createFfmpeg((message) => {
        state.stage.lastActivityAt = performance.now();
        if (message !== RUNTIME_EXIT_MESSAGE) {
            logLines.push(message);
        }
    });
    throwIfCancelled();
    await mountFile(file);
    logLines.length = 0;
    startFfmpegStage(runningMessage);
    const exitCode = await state.ffmpeg.exec(['-v', 'error', '-i', `${MOUNT_DIRECTORY}/${STAGED_FILE_NAME}`, ...FFMPEG_ARGUMENTS_BY_MODE[selectedMode()], '-f', 'null', '-']);
    // A fresh instance per file: the wasm runtime is not meant to survive an ffmpeg exit.
    state.ffmpeg.terminate();
    state.ffmpeg = null;
    return { exitCode, threading: 'st', versionLines, totalErrorLines: logLines.length, errors: groupErrorLines(logLines, MAX_DISTINCT_REPORT_LINES) };
}

async function mountFile(file) {
    const stagedFile = new File([file], STAGED_FILE_NAME);
    await state.ffmpeg.createDir(MOUNT_DIRECTORY);
    await state.ffmpeg.mount('WORKERFS', { files: [stagedFile] }, MOUNT_DIRECTORY);
}

function startFfmpegStage(runningMessage) {
    showBusy(runningMessage);
    setIndeterminateProgress();
    state.stage.startedAt = performance.now();
    state.stage.lastActivityAt = state.stage.startedAt;
    state.stage.isProgressing = true;
}

async function createFfmpeg(onLog) {
    const { FFmpeg } = await import(absoluteUrl(LIBRARY_URL));
    const ffmpeg = new FFmpeg();
    ffmpeg.on('log', ({ message }) => onLog(message));
    ffmpeg.on('progress', ({ progress }) => setDeterminateProgress(progress));
    await ffmpeg.load({ coreURL: absoluteUrl(CORE_URL), wasmURL: absoluteUrl(WASM_URL) });
    return ffmpeg;
}

// Separate instance: running a command after -version in the same wasm instance leaves ffmpeg verbose.
async function readFfmpegVersion() {
    const versionLines = [];
    state.versionFfmpeg = await createFfmpeg((message) => versionLines.push(message));
    await state.versionFfmpeg.exec(['-version']);
    state.versionFfmpeg.terminate();
    state.versionFfmpeg = null;
    return versionLines.filter((line) => line.trim() !== '' && line !== RUNTIME_EXIT_MESSAGE);
}

// ffmpeg.wasm resolves relative URLs against a base baked in at build time.
function absoluteUrl(url) {
    return new URL(url, location.href).href;
}

function hasProblems(findings) {
    const torrent = findings.torrent;
    const hasTorrentProblem = torrent !== null
        && (torrent.badPieceCount > 0
            || torrent.zeroPieceCount > 0
            || torrent.unverifiablePieces > 0
            || torrent.files.some((file) => PROBLEM_STATUSES.includes(file.status)));
    return hasTorrentProblem || findings.checks.some(isCheckProblematic);
}

function isCheckProblematic(check) {
    return check.prescan.anomalies.length > 0
        || check.prescan.readError !== null
        || check.ffmpeg.exitCode !== 0
        || check.ffmpeg.totalErrorLines > 0;
}

function showVerdict(findings) {
    const isProblematic = hasProblems(findings);
    showFinished(isProblematic ? LABELS.resultErrors : LABELS.resultOk, isProblematic ? 'error' : 'success');
    elements.verdict.textContent = isProblematic ? LABELS.verdictErrors : LABELS.verdictOk;
    elements.verdict.className = `badge ${isProblematic ? 'badge-danger' : 'badge-success'}`;
    elements.verdict.hidden = false;
}

function showFailure(failure) {
    failRunningStep();
    const message = state.wasCancelled ? LABELS.cancelled : `${LABELS.failed} ${failure?.message ?? failure}`;
    showFinished(message, state.wasCancelled ? null : 'error');
    state.ffmpeg?.terminate();
    state.versionFfmpeg?.terminate();
    state.ffmpeg = null;
    state.versionFfmpeg = null;
}

function cancel() {
    state.wasCancelled = true;
    state.ffmpeg?.terminate();
    state.versionFfmpeg?.terminate();
}

function throwIfCancelled() {
    if (state.wasCancelled) {
        throw new Error('cancelled');
    }
}

function toggleRunning(isRunning) {
    elements.startButton.hidden = isRunning;
    elements.cancelButton.hidden = !isRunning;
    for (const modeInput of elements.modeInputs) {
        modeInput.disabled = isRunning;
    }
    document.querySelector('main').setAttribute('aria-busy', String(isRunning));
    if (isRunning) {
        elements.cancelButton.focus();
    } else {
        elements.startButton.focus();
    }
}

function showBusy(message) {
    elements.progressPanel.hidden = false;
    elements.statusBox.hidden = true;
    elements.busyStatus.textContent = message;
}

function showFinished(message, outcome) {
    elements.progressPanel.hidden = true;
    elements.stallHint.hidden = true;
    elements.statusBox.hidden = false;
    elements.statusBox.classList.remove('status-box--success', 'status-box--error');
    if (outcome) {
        elements.statusBox.classList.add(`status-box--${outcome}`);
    }
    elements.status.textContent = message;
}

function beginStage(stepKey, message, totalBytes) {
    const now = performance.now();
    state.stage = { startedAt: now, lastActivityAt: now, ratio: null, totalBytes, packetCount: 0, isProgressing: false };
    setStepState(stepKey, 'running');
    showBusy(message);
    setIndeterminateProgress();
}

function setIndeterminateProgress() {
    state.stage.ratio = null;
    elements.progressBar.removeAttribute('value');
    renderProgressDetails();
}

function setDeterminateProgress(ratio) {
    state.stage.isProgressing = true;
    if (ratio !== state.stage.ratio) {
        state.stage.lastActivityAt = performance.now();
    }
    state.stage.ratio = ratio;
    elements.progressBar.value = Math.min(1, Math.max(0, ratio));
    renderProgressDetails();
}

function renderProgressDetails() {
    if (!state.startedAt) {
        return;
    }
    const now = performance.now();
    const hasStalled = state.stage.isProgressing && (now - state.stage.lastActivityAt) / MILLISECONDS_PER_SECOND >= STALL_SECONDS;
    elements.stallHint.hidden = !hasStalled;
    const totalElapsed = `${LABELS.elapsed} ${formatDuration((Date.now() - state.startedAt) / MILLISECONDS_PER_SECOND)}`;
    const parts = state.stage.ratio === null ? [totalElapsed] : stageProgressParts(now, hasStalled, totalElapsed);
    elements.progressDetails.textContent = parts.join(' · ');
}

function stageProgressParts(now, hasStalled, totalElapsed) {
    const progress = describeProgress({
        ratio: state.stage.ratio,
        totalBytes: state.stage.totalBytes ?? 0,
        stageElapsedSeconds: (now - state.stage.startedAt) / MILLISECONDS_PER_SECOND,
    });
    const parts = [`${progress.percent} %`];
    if (state.stage.packetCount > 0) {
        parts.push(`${state.stage.packetCount.toLocaleString('fr-FR')} ${LABELS.packetsRead}`);
    }
    if (progress.totalBytes > 0 && state.stage.packetCount === 0) {
        parts.push(`${formatBytes(progress.doneBytes)} / ${formatBytes(progress.totalBytes)}`);
    }
    if (!hasStalled && progress.bytesPerSecond !== null) {
        parts.push(`${formatBytes(progress.bytesPerSecond)}/s`);
    }
    parts.push(totalElapsed);
    if (!hasStalled && progress.remainingSeconds !== null) {
        parts.push(`${LABELS.remaining} ≈ ${formatDuration(progress.remainingSeconds)}`);
    }
    return parts;
}

function planStep(key, label) {
    state.steps.push({ key, label, state: 'pending' });
    renderSteps();
}

function setStepState(key, stepState) {
    const step = state.steps.find((candidate) => candidate.key === key);
    if (step) {
        step.state = stepState;
        renderSteps();
    }
}

function failRunningStep() {
    const runningStep = state.steps.find((candidate) => candidate.state === 'running');
    if (runningStep) {
        runningStep.state = 'error';
        renderSteps();
    }
}

function renderSteps() {
    elements.stepList.replaceChildren(...state.steps.map((step) => {
        const row = document.createElement('li');
        row.dataset.state = step.state;
        row.textContent = step.label;
        return row;
    }));
}

function bindPickers() {
    document.getElementById('pick-files').addEventListener('click', () => elements.filesInput.click());
    document.getElementById('pick-folder').addEventListener('click', () => elements.folderInput.click());
    document.getElementById('pick-torrent').addEventListener('click', () => elements.torrentInput.click());
    document.getElementById('clear-selection').addEventListener('click', () => {
        state.selectedEntries = [];
        renderSelection();
    });
    for (const input of [elements.filesInput, elements.folderInput, elements.torrentInput]) {
        input.addEventListener('change', (changeEvent) => {
            addEntries(entriesFromFileList(changeEvent.target.files));
            changeEvent.target.value = '';
        });
    }
}

function bindDropzone() {
    elements.dropzone.addEventListener('dragover', (dragEvent) => {
        dragEvent.preventDefault();
        elements.dropzone.classList.add('is-dragging');
    });
    elements.dropzone.addEventListener('dragleave', () => elements.dropzone.classList.remove('is-dragging'));
    elements.dropzone.addEventListener('drop', async (dropEvent) => {
        dropEvent.preventDefault();
        elements.dropzone.classList.remove('is-dragging');
        addEntries(await collectDroppedEntries(dropEvent.dataTransfer));
    });
}

function bindRunControls() {
    elements.startButton.addEventListener('click', start);
    elements.cancelButton.addEventListener('click', cancel);
    window.addEventListener('keydown', (keyEvent) => {
        if (keyEvent.key === 'Escape' && !elements.cancelButton.hidden) {
            cancel();
        }
    });
}

function isWebAssemblySupported() {
    return typeof WebAssembly === 'object' && typeof Worker === 'function';
}

function init() {
    bindPickers();
    bindDropzone();
    bindRunControls();
    renderSelection();
    if (!isWebAssemblySupported()) {
        showFinished(LABELS.unsupported, 'error');
    }
}

init();
