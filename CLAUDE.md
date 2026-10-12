# CLAUDE.md — youtube-dl-gui

Guide pour la maintenance et l'évolution du projet (humain ou IA).

## 1. Le projet

Application Windows **portable** (un seul `.exe`, aucun installeur) en **Go + Wails v2** qui encapsule
[yt-dlp](https://github.com/yt-dlp/yt-dlp) pour télécharger une vidéo YouTube en qualité maximale :
**vidéo+audio (MP4)**, **vidéo seule (MP4)** ou **audio seul (MP3)**.

Au démarrage, l'app installe/met à jour ses outils externes dans `bin/` (`yt-dlp`, `ffmpeg`+`ffprobe`, `deno`), puis
permet : analyse d'URL (titre, chaîne, durée, miniature), choix du mode et du dossier de sortie (mémorisé),
téléchargement avec progression/vitesse/ETA/étape, annulation, ouverture du dossier.

Hors périmètre actuel : playlists, file d'attente, sous-titres, choix manuel de qualité, sites autres que YouTube.

## 2. Stack

- Go (voir `go.mod`), Wails v2, WebView2 (présent sur Windows 10/11). Stdlib pour le reste (le strict minimum de
  dépendances).
- Frontend HTML/CSS/JS **vanilla** dans `frontend/`, embarqué via `embed` : pas de Node/npm, `frontend:install` et
  `frontend:build` sont vides dans `wails.json`.
- Textes UI et documentation en français ; code, identifiants et commentaires en anglais.

## 3. Arborescence

```
main.go, app.go        # bootstrap Wails ; struct App = bindings + événements Go→JS
frontend/              # index.html, style.css, main.js
build/windows/         # icône, manifest, info.json (versioninfo) ; build/dist/LISEZMOI.txt (inclus dans le zip)
internal/paths/        # dossiers portables (exe dir, repli %LOCALAPPDATA%\youtube-dl-gui si non inscriptible)
internal/config/       # config.json (dossier de sortie, dernier mode…)
internal/github/       # client minimal API Releases (BaseURL injectable pour httptest)
internal/deps/         # yt-dlp / ffmpeg / deno : versions, téléchargement, checksum, install atomique
internal/ytdlp/        # validation d'URL, métadonnées (-J), construction des args, parsing progression, exécution
internal/procutil/     # exec.Cmd sans console, kill de l'arbre de process (build tags windows / other)
internal/logging/      # slog → logs/app.log
```

Layout à l'exécution (à côté de l'exe) : `config.json`, `bin/` (outils + `versions.json`), `logs/app.log`.
Aucune écriture dans le registre.

## 4. Conventions

- **Erreurs** : `fmt.Errorf("context: %w", err)` ; pas de `panic` hors `main`. Les erreurs remontées à l'UI sont
  traduites en message lisible + détail technique séparé.
- **Process externes** : toujours via `internal/procutil` (pas de console qui flashe, `context.Context` pour
  l'annulation, kill de l'arbre de process).
- **Pas d'état global** : dépendances injectées dans `App`.
- **Logique pure testée** (comparaison de versions, args, parsing de progression, validation d'URL) : tests unitaires
  table-driven. Les tests ne doivent pas coder `.exe` en dur (`Paths.Tool(name)` n'ajoute `.exe` que sous Windows).
- **Code Windows-only** : build tags `//go:build windows` + stub pour les autres OS, pour que `go vet`/`go test`
  marchent sous Linux.
- **Téléchargements réseau** : fichier `*.part` dans le même dossier → SHA-256 si disponible → `os.Rename` atomique.
  Jamais d'écrasement d'un binaire fonctionnel par un fichier partiel.
- **Logs** : `log/slog` vers `logs/app.log`, sortie brute de yt-dlp recopiée.
- `gofmt` et `go vet` doivent passer.

## 5. Développement

Dev sous WSL2, compilation croisée vers Windows ; tests unitaires sous Linux, tests manuels de l'UI sous Windows.
Go n'est pas dans le `PATH` : `export PATH=$PATH:$HOME/.local/go/bin:$HOME/go/bin` (le `Makefile` le fait déjà).
Prérequis : Go et la CLI `wails` (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`).

```bash
make test                                          # go test ./...
make vet                                           # go vet ./...
make build                                         # build/bin/youtube-dl-gui.exe (cross-compilation)
make dist                                          # dist/youtube-dl-gui-<version>-win64.zip (exe + LISEZMOI.txt)
VERSION=1.0.0 make dist                            # forcer la version (sinon git describe), injectée via -X main.version
gofmt -l .                                         # doit être vide
go test -race -count=1 ./...
GOOS=windows GOARCH=amd64 go build -o /dev/null .  # vérifie la compilation Windows
```

`make dev` n'existe pas : le hot reload (`wails dev`) se lance côté Windows.
Ne pas committer sans demande explicite.

## 6. Outils externes (`internal/deps/tools.go`)

| Outil                        | Source (releases GitHub, dernière version)                                                              | Vérification         |
|------------------------------|---------------------------------------------------------------------------------------------------------|----------------------|
| `yt-dlp.exe`                 | `yt-dlp/yt-dlp` → `yt-dlp.exe`                                                                          | `SHA2-256SUMS`       |
| `ffmpeg.exe` + `ffprobe.exe` | `yt-dlp/FFmpeg-Builds` → `ffmpeg-master-latest-win64-gpl.zip` (builds patchées recommandées par yt-dlp) | `checksums.sha256`   |
| `deno.exe`                   | `denoland/deno` → `deno-x86_64-pc-windows-msvc.zip`                                                     | `.sha256sum` associé |

- Deno est requis par yt-dlp pour YouTube (résolution des signatures) : `--js-runtimes deno:<chemin>`.
- **yt-dlp** : vérifié à chaque démarrage, mis à jour si le tag distant est plus récent (comparaison numérique par
  segments de `YYYY.MM.DD[.N]`). **ffmpeg / deno** : installés si absents, mise à jour vérifiée au plus une fois par
  semaine (timestamp dans `versions.json`).
- Hors ligne ou rate-limit GitHub (403/429, 60 req/h non authentifié) : on garde la version locale ; sans version
  locale, erreur bloquante avec « Réessayer ».
- Mise à jour des binaires uniquement pendant le bootstrap, jamais pendant un téléchargement.
- Pour changer un nom d'asset ou ajouter un outil, modifier `tools.go` (et ses tests dans `deps_test.go`).

## 7. Commande yt-dlp (`internal/ytdlp/download_args.go`, `BuildArgs`)

- Métadonnées : `yt-dlp -J --no-playlist --no-warnings <url>`.
- Communs : `--no-playlist --newline --no-colors --no-warnings --ffmpeg-location <bin> --js-runtimes deno:<deno>
  -P <outputDir> -o "%(title)s [%(id)s].%(ext)s" --windows-filenames --trim-filenames 180 --no-mtime
  --progress-template ... --print after_move:FILE|%(filepath)s --no-quiet --progress -- <url>`.
  `--print` implique `--quiet`, d'où `--no-quiet --progress` pour conserver les lignes d'étape parsées.
- Vidéo+audio : `-f "bv*+ba/b" --merge-output-format mp4` +
  `--postprocessor-args "Merger+ffmpeg_o:-c:v copy -c:a aac -b:a 192k"`
  (vidéo copiée sans perte, audio réencodé en AAC car Opus-in-MP4 est mal lu par les lecteurs Windows).
- Vidéo seule : `-f "bv*" --remux-video mp4`.
- Audio seul : `-f "ba/b" -x --audio-format mp3 --audio-quality 0`.
- Choix actuel : résolution maximale (AV1/VP9 dans MP4). Idée d'évolution : case « compatibilité maximale »
  avec `-S "vcodec:h264,acodec:m4a"` (résolution souvent plafonnée à 1080p).
- Encodage : `PYTHONIOENCODING=utf-8` / `--encoding utf-8` pour les titres accentués.

## 8. Architecture UI ↔ backend

- Le frontend appelle les bindings de `App` (`window.go.main.App.*`) : `Bootstrap`, `CheckUpdates`, `Version`,
  `GetConfig`, `ChooseOutputDir`, `SetMode`, `Analyze`, `Download`, `Cancel`, `OpenFolder`.
- Événements Go → JS : `startup:*` (statut, fin du bootstrap) et `download:*` (progress, done, error, cancelled) ;
  les payloads sont définis dans `app.go` et consommés dans `frontend/main.js` — les modifier des deux côtés.
- **Un seul téléchargement à la fois** (mutex dans `App`). Annulation = kill de l'arbre de process + nettoyage des
  `.part`.
- Instance unique via `SingleInstanceLock`. Dossier de sortie introuvable → repli sur le défaut + bandeau
  d'avertissement.
- Fichier déjà présent : yt-dlp l'ignore (skip), traité comme un succès.

## 9. Points de vigilance

- YouTube casse régulièrement yt-dlp : la mise à jour au démarrage et le bouton de vérification manuelle sont la parade.
- L'exe n'est pas signé : SmartScreen/antivirus peuvent l'avertir (documenté dans `LISEZMOI.txt`).
- Vidéos à âge restreint / réservées aux membres : non gérées (message d'erreur clair). Piste :
  `--cookies-from-browser`.
- Recette manuelle avant une release : lancement sur un Windows vierge (dépendances installées seules), mise à jour de
  yt-dlp au lancement suivant, les 3 modes lisibles dans le lecteur par défaut, progression/annulation/messages de fin,
  dossier mémorisé, aucune fenêtre console.
