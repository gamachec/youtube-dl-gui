# YouTube DL

Petite application Windows **portable** pour télécharger une vidéo YouTube en qualité maximale, sans ligne de commande.
Rien à installer : un seul fichier `.exe`.

## Ce qu'elle fait

- Reconnaît l'adresse d'une vidéo YouTube collée dans la fenêtre et affiche son titre, sa chaîne, sa durée et sa miniature.
- Propose trois formats :
  - **Vidéo + audio** (MP4)
  - **Vidéo seule** (MP4)
  - **Audio seul** (MP3)
- Affiche la progression (pourcentage, vitesse, temps restant) et permet d'**annuler** à tout moment.
- Mémorise le dossier de destination d'une fois sur l'autre et propose d'**ouvrir le dossier** une fois le fichier prêt.
- Se met à jour toute seule : les outils nécessaires (yt-dlp, ffmpeg, deno) sont téléchargés au premier lancement,
  puis yt-dlp est mis à jour à chaque démarrage si une nouvelle version existe.

## Utilisation

1. Décompressez l'archive `youtube-dl-gui-<version>-win64.zip` où vous voulez.
2. Lancez `youtube-dl-gui.exe`. Au premier lancement, une connexion Internet est nécessaire pour récupérer les outils
   (ils sont placés dans un dossier `bin/` à côté de l'application).
3. Collez l'adresse de la vidéo, choisissez le format et le dossier de destination, puis cliquez sur **Télécharger**.

Pour désinstaller, supprimez simplement le dossier.

## Prérequis

- Windows 10 ou 11 (64 bits).
- WebView2 Runtime, déjà présent sur Windows 10/11 à jour ([téléchargement](https://developer.microsoft.com/microsoft-edge/webview2/) sinon).

## Avertissement SmartScreen / antivirus

L'application n'est pas signée : Windows SmartScreen peut afficher un avertissement au premier lancement.
Cliquez sur « Informations complémentaires » puis « Exécuter quand même ».

## Limites

- Une seule vidéo à la fois (pas de playlist ni de file d'attente).
- Les vidéos à âge restreint ou réservées aux membres ne sont pas prises en charge.
- Seul YouTube est officiellement pris en charge.

## Licences

Les outils tiers sont téléchargés depuis leurs sources officielles et ne sont pas redistribués :
yt-dlp (Unlicense), ffmpeg (GPL), deno (MIT).
