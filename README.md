# mkvcheck

Vérificateur d'intégrité de fichiers MKV et de torrents. L'analyse se fait entièrement dans le navigateur (ffmpeg.wasm, mediabunny, mediainfo.js) ; le serveur ne sert que la page et stocke, 48 h, les rapports que l'utilisateur choisit d'enregistrer.

> Les images disque (ISO, BDMV, DVD) peuvent produire de faux positifs : un constat n'est pas forcément une corruption.

## Lancer

Image publiée sur GitHub Container Registry à chaque tag `vX.Y.Z` (remplacer `<proprietaire>/<depot>` par le dépôt GitHub, en minuscules) :

```sh
docker pull ghcr.io/<proprietaire>/<depot>:0.1.0
docker run -d --read-only -p 8080:8080 -v mkvcheck-data:/data \
  --cap-drop ALL --security-opt no-new-privileges ghcr.io/<proprietaire>/<depot>:0.1.0
```

Ou en construisant l'image localement :

```sh
docker build -t mkvcheck .
docker run -d --read-only -p 8080:8080 -v mkvcheck-data:/data \
  --cap-drop ALL --security-opt no-new-privileges mkvcheck
```

La page est servie sur `http://localhost:8080`, la sonde de santé sur `/healthz`. Le build télécharge les assets épinglés (ffmpeg.wasm, mediabunny, mediainfo.js, altcha) depuis le registre npm, avec vérification SHA-256. Les fichiers `/static/vendor/` sont mis en cache 24 h (URL non versionnées) : après une montée de version des assets, les navigateurs peuvent garder les anciens fichiers jusqu'à un jour.

Sans Docker : `make build` puis `bin/mkvcheck`.

## Variables d'environnement

| Variable | Défaut | Rôle |
|---|---|---|
| `MKVCHECK_ADDR` | `:8080` | Adresse d'écoute |
| `MKVCHECK_DATA_DIR` | `data` (`/data` dans l'image) | Dossier des rapports |
| `MKVCHECK_HMAC_KEY` | générée au démarrage | Clé HMAC des défis anti-abus, 32 octets minimum ; à fixer pour que les défis survivent à un redémarrage |
| `MKVCHECK_POW_DIFFICULTY` | `200000` | Difficulté de la preuve de travail Altcha |
| `MKVCHECK_RATE_PER_MINUTE` | `20` | Requêtes par minute et par client (un enregistrement en consomme deux : défi et dépôt) |
| `MKVCHECK_BURST` | `10` | Rafale tolérée par client (mêmes requêtes) |
| `MKVCHECK_MAX_STORE_MB` | `512` | Plafond de stockage des rapports |
| `MKVCHECK_IP_HEADER` | vide | En-tête portant l'IP du client derrière un proxy de confiance |

## Cibles make

| Cible | Action |
|---|---|
| `assets` | Installe les assets navigateur dans `web/static/vendor/` |
| `build` | Compile `bin/mkvcheck` |
| `test` | `go test` limité à 2 paquets en parallèle |
| `docker` | Construit l'image `mkvcheck` |
| `quality` | Porte complète : gofumpt, goimports, go vet, staticcheck, Prettier, ESLint, golangci-lint, deadcode, knip, shellcheck, shfmt, actionlint, tests `-race -cover`, fuzz, tests Node, govulncheck, gosec, `go mod tidy`/`verify`, `npm audit`, hadolint, gitleaks |
| `quality-fast` | Formatage et lint seulement (sans tests ni audits) |
| `fuzz` | `FuzzParse` (analyse des constats) pendant 20 s |
| `mutation` | Tests de mutation gremlins sur `internal/findings` (manuel, hors `quality`) |
| `hooks` | Active `.githooks/pre-push` dans `.git/hooks/` |

Les outils Go sont lancés par `go run <module>@<version>`, les outils JS (Prettier, ESLint, knip) sont épinglés dans `package-lock.json` (`npm ci`), et hadolint, gitleaks, shellcheck, shfmt et actionlint tournent dans leurs images Docker étiquetées dans le `Makefile`.

## Qualité

- `make quality` : porte complète, exécutée aussi par la CI avant la publication de l'image.
- `make hooks` : installe le hook `pre-push` versionné, qui lance `make quality` et annule le push en cas d'échec.
- `make fuzz` : fuzzing de l'analyse des constats ; `make mutation` : tests de mutation, manuel.

## Confidentialité

- L'analyse des fichiers et des torrents reste dans le navigateur : rien n'est envoyé sans clic sur « Enregistrer ».
- Seuls les constats (noms de fichiers, chemins du torrent, erreurs ffmpeg, user-agent) partent au serveur ; quiconque a le lien les lit pendant 48 h.
- Passé ce délai, le rapport est supprimé ; aucun compte, aucun cookie, aucune ressource distante.

## Crédits

| Composant | Rôle | Licence |
|---|---|---|
| [ffmpeg.wasm](https://github.com/ffmpegwasm/ffmpeg.wasm) (`@ffmpeg/ffmpeg` 0.12.15) | Pilote ffmpeg dans un worker du navigateur | MIT |
| `@ffmpeg/core` 0.12.10 (build WebAssembly de [FFmpeg](https://ffmpeg.org)) | Décodage et remux des MKV | GPL-2.0-or-later |
| [mediabunny](https://mediabunny.dev) 1.65.2 | Pré-analyse du conteneur | MPL-2.0 |
| [mediainfo.js](https://github.com/buzz/mediainfo.js) 0.3.9 (build WebAssembly de [MediaInfoLib](https://mediaarea.net/en/MediaInfo)) | Fiche technique affichée localement | BSD-2-Clause |
| [Altcha](https://altcha.org) 3.5.0 (widget) et [altcha-lib-go](https://github.com/altcha-org/altcha-lib-go) | Preuve de travail anti-abus | MIT |
| [golang.org/x/time](https://pkg.go.dev/golang.org/x/time) | Limite de débit | BSD-3-Clause |

La licence MIT de ce dépôt ne couvre que son propre code. `ffmpeg-core.wasm` est un composant distinct sous GPL-2.0-or-later, téléchargé au build avec une empreinte SHA-256 épinglée (`scripts/fetch-assets.sh`) ; ses sources sont celles de FFmpeg et du dépôt ffmpeg.wasm. Les fichiers des bibliothèques gardent leurs en-têtes et licences d'origine.

## Licence

MIT, voir [LICENSE](LICENSE).
