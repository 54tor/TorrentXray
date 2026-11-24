# mkvcheck

Vérificateur d'intégrité de fichiers MKV et de torrents. L'analyse se fait entièrement dans le navigateur (ffmpeg.wasm, mediabunny) ; le serveur ne sert que la page et stocke, 48 h, les rapports que l'utilisateur choisit d'enregistrer.

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

La page est servie sur `http://localhost:8080`, la sonde de santé sur `/healthz`. Le build télécharge les assets épinglés (ffmpeg.wasm, mediabunny, altcha) depuis le registre npm, avec vérification SHA-256. Les fichiers `/static/vendor/` sont mis en cache 24 h (URL non versionnées) : après une montée de version des assets, les navigateurs peuvent garder les anciens fichiers jusqu'à un jour.

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
| `quality` | gofmt, go vet, staticcheck, golangci-lint, tests `-race -cover`, govulncheck, gosec, `go mod tidy`/`verify`, hadolint, gitleaks |

Les outils Go sont lancés par `go run <module>@<version>` ; hadolint et gitleaks par leurs images Docker officielles, étiquetées dans le `Makefile`.

## Confidentialité

- L'analyse des fichiers et des torrents reste dans le navigateur : rien n'est envoyé sans clic sur « Enregistrer ».
- Seuls les constats (noms de fichiers, chemins du torrent, erreurs ffmpeg, user-agent) partent au serveur ; quiconque a le lien les lit pendant 48 h.
- Passé ce délai, le rapport est supprimé ; aucun compte, aucun cookie, aucune ressource distante.
