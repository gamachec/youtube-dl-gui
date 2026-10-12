// Centralised UI strings (French). Replace this object to translate the UI.
window.i18n = {
  appName: "YouTube DL",
  startup: {
    checking: "Vérification de yt-dlp…",
    working: "Préparation…",
    retry: "Réessayer",
    errorTitle: "Impossible de démarrer",
  },
  url: {
    label: "Adresse de la vidéo YouTube",
    placeholder: "https://www.youtube.com/watch?v=…",
    paste: "Coller",
  },
  card: {
    empty: "Collez une adresse YouTube pour commencer.",
    loading: "Analyse de la vidéo…",
    maxQuality: (h) => `jusqu'à ${h}p`,
    live: "Direct",
  },
  mode: {
    legend: "Format",
    video_audio: { label: "Vidéo + audio", ext: "MP4", icon: "🎬" },
    video_only: { label: "Vidéo seule", ext: "MP4", icon: "🎞️" },
    audio_only: { label: "Audio seul", ext: "MP3", icon: "🎵" },
  },
  output: {
    label: "Dossier de sortie",
    browse: "Parcourir…",
    none: "Aucun dossier choisi",
  },
  actions: {
    download: "Télécharger",
    cancel: "Annuler",
  },
  status: {
    starting: "Démarrage…",
    successTitle: "Téléchargement terminé",
    errorTitle: "Le téléchargement a échoué",
    cancelled: "Téléchargement annulé.",
    openFolder: "Ouvrir le dossier",
    details: "Détail technique",
    remaining: (eta) => `reste ${eta}`,
  },
  footer: {
    version: (v) => `yt-dlp ${v}`,
    versionUnknown: "yt-dlp : version inconnue",
    checkUpdates: "Vérifier les mises à jour",
    checking: "Vérification…",
  },
  errors: {
    generic: "Une erreur inattendue est survenue.",
    unavailable: "Cette fonction n'est pas disponible.",
  },
};
